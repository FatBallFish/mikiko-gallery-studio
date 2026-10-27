// Package mcpserver adapts the shared application service to the official MCP transport.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Executor interface {
	Execute(context.Context, openplatform.Principal, string, json.RawMessage) (any, error)
}
type AccessValidator interface {
	ValidateAccess(context.Context, string) (openplatform.Principal, error)
}
type ValidateAccessFunc func(context.Context, string) (openplatform.Principal, error)

func (f ValidateAccessFunc) ValidateAccess(ctx context.Context, token string) (openplatform.Principal, error) {
	return f(ctx, token)
}

// Config URLs must come from validated deployment configuration, never request Host.
type Config struct {
	Service             Executor
	Validator           AccessValidator
	ResourceURL         string
	ResourceMetadataURL string
	AllowedOrigins      []string
}

// NewHandler returns the /mcp handler. OAuth metadata routes are wired by the caller.
// The SDK owns protocol negotiation, JSON-RPC, and transport error semantics.
func NewHandler(cfg Config) (http.Handler, error) {
	if cfg.Service == nil || cfg.Validator == nil {
		return nil, errors.New("MCP requires service and access validator")
	}
	resource, err := canonicalURL(cfg.ResourceURL)
	if err != nil {
		return nil, fmt.Errorf("MCP resource: %w", err)
	}
	metadata := cfg.ResourceMetadataURL
	if metadata == "" {
		metadata = resource.Scheme + "://" + resource.Host + "/.well-known/oauth-protected-resource" + resource.Path
	}
	if _, err := canonicalURL(metadata); err != nil {
		return nil, fmt.Errorf("MCP resource metadata: %w", err)
	}
	origins := map[string]bool{resource.Scheme + "://" + resource.Host: true}
	for _, origin := range cfg.AllowedOrigins {
		u, err := canonicalURL(origin)
		if err != nil || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("invalid MCP allowed origin")
		}
		origins[u.Scheme+"://"+u.Host] = true
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "studio", Version: "1.0.0"}, &mcp.ServerOptions{PageSize: 5})
	cache := newSnapshotCache()
	readSpec := readToolSpec()
	readInputSchema, err := resolveSchema(readSpec.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("MCP tool %s input schema: %w", readSpec.Name, err)
	}
	server.AddTool(&mcp.Tool{Name: readSpec.Name, Description: readSpec.Description, InputSchema: readSpec.InputSchema, OutputSchema: boundedOutputSchema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPtr(false)}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		principal, ok := ctx.Value(principalKey{}).(openplatform.Principal)
		if !ok {
			return toolError("unauthorized"), nil
		}
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		var input any
		if json.Unmarshal(args, &input) != nil || readInputSchema.Validate(input) != nil {
			return toolError("invalid tool arguments"), nil
		}
		var in struct {
			Token  string `json:"token"`
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		_ = json.Unmarshal(args, &in)
		if in.Offset < 0 || in.Limit < 0 {
			return toolError("offset and limit must be non-negative"), nil
		}
		data, err := cache.get(in.Token, principal, readSpec.Scope)
		if err != nil {
			return toolError(err.Error()), nil
		}
		root, err := decodeResult(data)
		if err != nil {
			return toolError("snapshot is invalid"), nil
		}
		value, err := jsonPointer(root, in.Path)
		if err != nil {
			return toolError(err.Error()), nil
		}
		env, err := readPage(in.Token, in.Path, in.Offset, in.Limit, value)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return envelopeResult(env), nil
	})
	nativeOutputs := map[string]map[string]any{}
	for _, operation := range openplatform.Specs() {
		nativeOutputs[operation.Name] = map[string]any{"type": "object", "properties": map[string]any{"data": operation.OutputSchema}, "required": []string{"data"}, "additionalProperties": false}
	}
	for _, spec := range Catalogue() {
		if spec.Name == readSpec.Name {
			continue
		}
		inputSchema, err := resolveSchema(spec.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("MCP tool %s input schema: %w", spec.Name, err)
		}
		outputSchema, err := resolveSchema(nativeOutputs[spec.Name])
		if err != nil {
			return nil, fmt.Errorf("MCP tool %s output schema: %w", spec.Name, err)
		}
		server.AddTool(&mcp.Tool{Name: spec.Name, Description: spec.Description, InputSchema: spec.InputSchema, OutputSchema: spec.OutputSchema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: spec.ReadOnly, IdempotentHint: spec.Idempotent, DestructiveHint: boolPtr(spec.Destructive), OpenWorldHint: boolPtr(false)}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			principal, ok := ctx.Value(principalKey{}).(openplatform.Principal)
			if !ok {
				return toolError("unauthorized"), nil
			}
			if !slices.Contains(principal.Scopes, spec.Scope) {
				return scopeError(spec.Scope, metadata), nil
			}
			arguments := req.Params.Arguments
			if len(arguments) == 0 {
				arguments = json.RawMessage(`{}`)
			}
			var input any
			if err := json.Unmarshal(arguments, &input); err != nil {
				return toolError("invalid tool arguments"), nil
			}
			if err := inputSchema.Validate(input); err != nil {
				return toolError("invalid tool arguments: " + err.Error()), nil
			}
			result, err := cfg.Service.Execute(ctx, principal, spec.Name, arguments)
			if err != nil {
				return toolError(err.Error()), nil
			}
			output := map[string]any{"data": result}
			data, err := json.Marshal(output)
			if err != nil {
				if !spec.ReadOnly {
					return envelopeResult(receipt(result, spec.Name)), nil
				}
				return toolError("could not encode operation result"), nil
			}
			var encoded any
			err = json.Unmarshal(data, &encoded)
			if err != nil {
				if !spec.ReadOnly {
					return envelopeResult(receipt(result, spec.Name)), nil
				}
				return toolError("could not encode operation result"), nil
			}
			if err := outputSchema.Validate(encoded); err != nil {
				if !spec.ReadOnly {
					return envelopeResult(receipt(result, spec.Name)), nil
				}
				return toolError("operation returned an invalid result"), nil
			}
			if spec.Name == "list_image_models" || spec.Name == "list_video_models" {
				token, err := cache.put(principal, spec.Scope, result)
				if err != nil {
					return toolError("model catalogue exceeds snapshot capacity; narrow the query"), nil
				}
				summary := modelSummary(result)
				out := map[string]any{"data": summary, "details": map[string]any{"tool": "read_tool_result", "token": token, "path": "", "expires_in_seconds": 120, "instructions": "Read entries at the original model path for full task-specific constraints and prices before preflight."}}
				if r := envelopeResult(out); fitsResult(r) {
					return r, nil
				}
				return boundedResult(principal, out, cache, spec.Scope, true, spec.Name), nil
			}
			return boundedResult(principal, result, cache, spec.Scope, spec.ReadOnly, spec.Name), nil
		})
	}
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, MaxRequestBodyBytes: 1 << 20})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if values := r.Header.Values("Origin"); len(values) > 0 {
			if len(values) != 1 || !origins[values[0]] {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
		}
		fields := strings.Fields(r.Header.Get("Authorization"))
		if len(r.Header.Values("Authorization")) != 1 || len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			challenge(w, metadata, "", false)
			return
		}
		principal, err := cfg.Validator.ValidateAccess(r.Context(), fields[1])
		if err != nil || principal.UserID <= 0 || principal.ClientID == "" || principal.GrantID == "" {
			challenge(w, metadata, "", true)
			return
		}
		// Only validated OAuth grants become principals. Session cookies and API keys do not.
		transport.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)))
	}), nil
}

type principalKey struct{}

func canonicalURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("expected absolute canonical URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return nil, errors.New("HTTPS required except loopback")
	}
	return u, nil
}
func boolPtr(v bool) *bool { return &v }
func challenge(w http.ResponseWriter, metadata, scope string, invalid bool) {
	value := fmt.Sprintf("Bearer resource_metadata=%q", metadata)
	if scope != "" {
		value += fmt.Sprintf(", scope=%q", scope)
	}
	if invalid {
		value += `, error="invalid_token"`
	}
	w.Header().Set("WWW-Authenticate", value)
	http.Error(w, "OAuth bearer access token required", http.StatusUnauthorized)
}
func toolError(message string) *mcp.CallToolResult {
	if len(message) > 4096 {
		message = strings.ToValidUTF8(message[:4096], "") + " (diagnostic shortened)"
	}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}
func scopeError(scope, metadata string) *mcp.CallToolResult {
	result := toolError("insufficient_scope: requires " + scope)
	result.Meta = mcp.Meta{"mcp/www_authenticate": []string{fmt.Sprintf("Bearer error=\"insufficient_scope\", resource_metadata=%q, scope=%q", metadata, scope)}}
	return result
}
