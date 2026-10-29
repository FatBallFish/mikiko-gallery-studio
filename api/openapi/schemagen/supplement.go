package schemagen

import (
	"bytes"
	"gopkg.in/yaml.v3"
	"sort"
	"strings"
)

func scalar(t string) map[string]any { return map[string]any{"type": t} }
func oauthError(codes ...string) map[string]any {
	return obj(map[string]any{"error": map[string]any{"type": "string", "enum": codes}}, "error")
}
func syncSupplement(raw []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	root := doc.Content[0]
	paths := get(root, "paths")
	changed := map[string]bool{}
	if item := get(paths, "/api/public/v1/videos/{asset_id}/video"); item != nil {
		for _, method := range []string{"get", "head"} {
			set(get(item, method), "operationId", method+"PublicVideoContent")
		}
		changed["/api/public/v1/videos/{asset_id}/video"] = true
	}
	syncTransfers(paths, changed)
	for i := 0; i < len(paths.Content); i += 2 {
		path := paths.Content[i].Value
		if !(strings.HasPrefix(path, "/oauth/") || strings.HasPrefix(path, "/.well-known/oauth-") || strings.HasPrefix(path, "/api/v1/oauth/")) {
			continue
		}
		changed[path] = true
		methods := paths.Content[i+1]
		for j := 0; j < len(methods.Content); j += 2 {
			method := methods.Content[j].Value
			op := methods.Content[j+1]
			rs := decode(get(op, "responses"))
			bad := []string{"invalid_request"}
			switch path {
			case "/oauth/token":
				bad = append(bad, "invalid_grant", "invalid_scope", "invalid_target", "unsupported_grant_type")
			case "/oauth/register":
				bad = append(bad, "invalid_client_metadata")
			case "/oauth/authorize":
				bad = append(bad, "invalid_scope", "invalid_target")
			case "/api/v1/oauth/consent":
				bad = append(bad, "authorization_request_expired")
			}
			rs["400"] = response(oauthError(bad...))
			rs["400"].(map[string]any)["description"] = "Invalid OAuth request; correct input before retrying."
			rs["503"] = map[string]any{"description": "Transient OAuth service failure, or canonical URL is not configured (CANONICAL_URL_UNCONFIGURED). Retry transient failures; configure canonical URL before retrying configuration failures.", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"oneOf": []any{oauthError("temporarily_unavailable"), ref("./components/error-components.yaml#/components/schemas/ErrorResponse")}}}}}
			if strings.HasPrefix(path, "/api/v1/oauth/") {
				rs["401"] = response(oauthError("login_required", "invalid_session"))
				rs["401"].(map[string]any)["description"] = "Missing browser login or invalid/expired browser session. Sign in again."
			}
			if path == "/oauth/token" {
				rs["401"] = response(oauthError("invalid_client"))
				body := decode(get(op, "requestBody"))
				schema := body["content"].(map[string]any)["application/x-www-form-urlencoded"].(map[string]any)["schema"].(map[string]any)
				schema["oneOf"] = []any{map[string]any{"properties": map[string]any{"grant_type": map[string]any{"const": "authorization_code"}}, "required": []string{"code", "redirect_uri", "code_verifier"}}, map[string]any{"properties": map[string]any{"grant_type": map[string]any{"const": "refresh_token"}}, "required": []string{"refresh_token"}}}
				set(op, "requestBody", body)
			}
			if path == "/oauth/register" {
				rs["403"] = response(oauthError("access_denied"))
				rs["429"] = response(oauthError("temporarily_unavailable"))
				rs["429"].(map[string]any)["description"] = "Dynamic registration limit: 10 requests per minute per client IP. Retry after Retry-After seconds."
				rs["429"].(map[string]any)["headers"] = map[string]any{"Retry-After": map[string]any{"schema": map[string]any{"type": "integer", "minimum": 1}, "description": "Seconds until the registration window resets."}}
			}
			if strings.HasPrefix(path, "/.well-known/") {
				props := map[string]any{}
				if path == "/.well-known/oauth-authorization-server" {
					for _, k := range []string{"issuer", "authorization_endpoint", "token_endpoint", "revocation_endpoint", "registration_endpoint"} {
						props[k] = scalar("string")
					}
					for _, k := range []string{"response_types_supported", "grant_types_supported", "token_endpoint_auth_methods_supported", "code_challenge_methods_supported", "scopes_supported"} {
						props[k] = map[string]any{"type": "array", "items": scalar("string")}
					}
					props["client_id_metadata_document_supported"] = scalar("boolean")
				} else {
					props["resource"] = scalar("string")
					for _, k := range []string{"authorization_servers", "scopes_supported", "bearer_methods_supported"} {
						props[k] = map[string]any{"type": "array", "items": scalar("string")}
					}
				}
				required := []string{}
				for k := range props {
					required = append(required, k)
				}
				sort.Strings(required)
				rs["200"] = response(obj(props, required...))
				delete(rs, "400")
			}
			if path == "/oauth/revoke" {
				rs["200"] = response(obj(map[string]any{}))
			}
			if get(op, "operationId") == nil {
				set(op, "operationId", "oauth_"+method+strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(path))
			}
			set(op, "responses", rs)
		}
	}
	// Public download is a browser-authenticated attachment; playback stays public.
	idParam := map[string]any{"name": "asset_id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}}
	for _, action := range []string{"like", "favorite"} {
		path := "/api/public/v1/videos/{asset_id}/" + action
		changed[path] = true
		set(paths, path, map[string]any{"post": map[string]any{"tags": []string{"Public Media"}, "operationId": "setPublicVideo" + strings.ToUpper(action[:1]) + action[1:], "summary": "Set public video " + action + " state", "description": "Idempotently sets the current viewer state for an approved public video.", "security": []any{map[string]any{"bearerAuth": []string{}}}, "parameters": []any{idParam}, "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": obj(map[string]any{"active": scalar("boolean")}, "active")}}}, "responses": map[string]any{"200": response(ref("#/components/schemas/PublicVideoResponse")), "400": response(ref("./components/schemas/common.yaml#/components/schemas/ErrorEnvelope")), "401": response(ref("./components/schemas/common.yaml#/components/schemas/ErrorEnvelope")), "404": response(ref("./components/schemas/common.yaml#/components/schemas/ErrorEnvelope")), "503": response(ref("./components/schemas/common.yaml#/components/schemas/ErrorEnvelope"))}}})
	}
	path := "/api/public/v1/videos/{asset_id}/download"
	changed[path] = true
	download := map[string]any{}
	for _, method := range []string{"get", "head"} {
		rs := map[string]any{}
		for _, code := range []string{"200", "206", "416"} {
			headers := map[string]any{"Accept-Ranges": map[string]any{"schema": map[string]any{"type": "string", "const": "bytes"}}, "Content-Length": map[string]any{"schema": map[string]any{"type": "integer", "minimum": 0}}, "Cache-Control": map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}}}
			if code != "416" {
				headers["Content-Disposition"] = map[string]any{"schema": scalar("string"), "description": "attachment; filename and filename* identify the downloaded video"}
			}
			if code != "200" {
				headers["Content-Range"] = map[string]any{"schema": scalar("string"), "description": "206: bytes start-end/size; 416: bytes */size"}
			}
			r := map[string]any{"description": map[string]string{"200": "Complete attachment", "206": "Single byte range attachment", "416": "Invalid, multiple or unsatisfiable range; empty body"}[code], "headers": headers}
			if method == "get" && code != "416" {
				r["content"] = map[string]any{"video/*": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}
			}
			rs[code] = r
		}
		for _, code := range []string{"401", "404", "503"} {
			rs[code] = response(ref("./components/schemas/common.yaml#/components/schemas/ErrorEnvelope"))
		}
		download[method] = map[string]any{"operationId": method + "PublicVideoDownload", "tags": []string{"Public Media"}, "summary": "Download approved public video", "description": "Browser Bearer required. Rechecks approved visibility; withdrawn videos are inaccessible. Streams attachment bytes without redirect. HEAD returns headers without a body.", "security": []any{map[string]any{"bearerAuth": []string{}}}, "parameters": []any{idParam, map[string]any{"name": "Range", "in": "header", "schema": scalar("string"), "description": "One bytes range start-end, start-, or -suffix. Multiple ranges are rejected."}}, "responses": rs}
	}
	set(paths, path, download)
	// Replace only the selected path blocks to preserve unrelated browser/admin source.
	lines := strings.Split(string(raw), "\n")
	type patch struct {
		start, end int
		text       string
	}
	patches := []patch{}
	added := []string{}
	for i := 0; i < len(paths.Content); i += 2 {
		key, n := paths.Content[i], paths.Content[i+1]
		if !changed[key.Value] {
			continue
		}
		var b bytes.Buffer
		e := yaml.NewEncoder(&b)
		e.SetIndent(2)
		wrapper := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.Value}, n}}
		if err := e.Encode(wrapper); err != nil {
			return nil, err
		}
		_ = e.Close()
		parts := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		for j := range parts {
			if parts[j] != "" {
				parts[j] = "  " + parts[j]
			}
		}
		text := strings.Join(parts, "\n")
		if key.Line == 0 {
			added = append(added, text)
			continue
		}
		end := get(root, "components").Line - 2
		for j := i + 2; j < len(paths.Content); j += 2 {
			if paths.Content[j].Line > 0 {
				end = paths.Content[j].Line - 1
				break
			}
		}
		patches = append(patches, patch{key.Line - 1, end, text})
	}
	if len(added) > 0 {
		at := get(root, "components").Line - 2
		patches = append(patches, patch{at, at, strings.Join(added, "\n")})
	}
	sort.SliceStable(patches, func(i, j int) bool { return patches[i].start > patches[j].start })
	for _, p := range patches {
		lines = append(lines[:p.start], append([]string{p.text}, lines[p.end:]...)...)
	}
	return []byte(strings.Join(lines, "\n")), nil
}
