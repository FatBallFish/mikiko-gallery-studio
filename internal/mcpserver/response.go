package mcpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const resultBudget = 40 << 10
const resultPayloadBudget = resultBudget - 2048

var boundedOutputSchema = map[string]any{"type": "object", "properties": map[string]any{
	"data": map[string]any{}, "snapshot_token": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}, "offset": map[string]any{"type": "integer"}, "next_offset": map[string]any{"type": "integer"}, "complete": map[string]any{"type": "boolean"}, "items": map[string]any{}, "value": map[string]any{}, "operation_completed": map[string]any{"type": "boolean"}, "id": map[string]any{}, "status": map[string]any{}, "replayed": map[string]any{}, "query_tool": map[string]any{"type": "string"}, "recovery": map[string]any{"type": "string"}, "details": map[string]any{},
}, "anyOf": []any{map[string]any{"required": []string{"data"}}, map[string]any{"required": []string{"type", "complete"}}, map[string]any{"required": []string{"operation_completed"}}}}

func decodeResult(data []byte) (any, error) {
	var value any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	err := d.Decode(&value)
	return value, err
}
func envelopeResult(v map[string]any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return toolError("could not encode result")
	}
	resolved, err := resolveSchema(boundedOutputSchema)
	var validation any
	if json.Unmarshal(data, &validation) != nil || err != nil || resolved.Validate(validation) != nil {
		return toolError("invalid MCP result envelope")
	}
	return &mcp.CallToolResult{StructuredContent: v, Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}
}
func fitsResult(r *mcp.CallToolResult) bool {
	b, e := json.Marshal(r)
	return e == nil && len(b) < resultPayloadBudget
}
func receipt(value any, operation string) map[string]any {
	out := map[string]any{"operation_completed": true, "recovery": "Do not repeat the write. Query the existing resource by its ID."}
	b, e := json.Marshal(value)
	if e != nil {
		return out
	}
	v, e := decodeResult(b)
	if e != nil {
		return out
	}
	if m, ok := v.(map[string]any); ok {
		if task, ok := m["task"].(map[string]any); ok {
			for _, k := range []string{"id", "status"} {
				if x, ok := task[k]; ok {
					out[k] = x
				}
			}
		}
		for _, k := range []string{"id", "status", "replayed"} {
			if x, ok := m[k]; ok {
				raw, _ := json.Marshal(x)
				if len(raw) < 1024 {
					out[k] = x
				}
			}
		}
	}
	queries := map[string]string{"create_image_task": "get_image_task", "create_video_task": "get_video_task", "cancel_video_task": "get_video_task", "create_upload": "get_upload", "complete_upload": "get_asset", "create_project": "get_project", "update_project": "get_project", "update_asset": "get_asset", "create_group": "list_groups", "update_group": "list_groups"}
	if q := queries[operation]; q != "" {
		out["query_tool"] = q
	}
	return out
}
func boundedResult(p openplatform.Principal, value any, cache *snapshotCache, scope string, readOnly bool, operation string) *mcp.CallToolResult {
	inline := envelopeResult(map[string]any{"data": value})
	if !inline.IsError && fitsResult(inline) {
		return inline
	}
	token, err := cache.put(p, scope, value)
	if err != nil {
		if !readOnly {
			return envelopeResult(receipt(value, operation))
		}
		return toolError("result exceeds snapshot capacity; narrow the original read query")
	}
	out := snapshotEnvelope(token, "json", "", 0, false, nil, nil, nil)
	if !readOnly {
		for k, v := range receipt(value, operation) {
			out[k] = v
		}
	}
	return envelopeResult(out)
}
func snapshotEnvelope(token, typ, path string, offset int, complete bool, items, value any, next *int) map[string]any {
	m := map[string]any{"snapshot_token": token, "type": typ, "path": path, "offset": offset, "complete": complete}
	if items != nil {
		m["items"] = items
	}
	if value != nil {
		m["value"] = value
	}
	if next != nil {
		m["next_offset"] = *next
	}
	return m
}
func utf8Fragment(s string, offset, limit int) (string, int, bool, error) {
	if offset < 0 || offset > len(s) || limit < 1 || (offset < len(s) && !utf8.RuneStart(s[offset])) {
		return "", 0, false, errors.New("invalid UTF-8 byte offset or limit")
	}
	end := offset + min(limit, len(s)-offset)
	for end > offset && end < len(s) && !utf8.RuneStart(s[end]) {
		end--
	}
	if end == offset && offset < len(s) {
		_, n := utf8.DecodeRuneInString(s[offset:])
		end = offset + n
	}
	return s[offset:end], end, end == len(s), nil
}
func modelSummary(value any) any {
	b, e := json.Marshal(value)
	if e != nil {
		return value
	}
	v, e := decodeResult(b)
	if e != nil {
		return value
	}
	root, ok := v.(map[string]any)
	if !ok {
		return value
	}
	out := map[string]any{}
	for _, key := range []string{"model_groups", "groups", "models", "items"} {
		list, ok := root[key].([]any)
		if !ok {
			continue
		}
		items := make([]any, 0, len(list))
		for i, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			s := map[string]any{"details_path": fmt.Sprintf("/%s/%d", key, i)}
			for _, k := range []string{"id", "code", "route_model_code", "name", "task_types", "minimum_points", "effective_multiplier", "version", "config_version", "capability_version"} {
				if x, ok := m[k]; ok {
					s[k] = x
				}
			}
			items = append(items, s)
		}
		out[key] = items
	}
	if len(out) == 0 {
		return value
	}
	return out
}
