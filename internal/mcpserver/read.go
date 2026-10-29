package mcpserver

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
)

func readToolSpec() openplatform.OperationSpec {
	return openplatform.OperationSpec{Name: "read_tool_result", Description: "Read a page of a previous result. Use token, RFC6901 path and next_offset. Object/array entries with type=reference must be read using their path. String offsets and limits are UTF-8 bytes. Never repeat a completed write when a snapshot expires.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"token": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "path": map[string]any{"type": "string", "maxLength": 4096}, "offset": map[string]any{"type": "integer", "minimum": 0, "maximum": snapshotMax}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 8192}}, "required": []string{"token"}, "additionalProperties": false}, OutputSchema: boundedOutputSchema, ReadOnly: true, Idempotent: true}
}
func pointerChild(path, key string) string {
	return path + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}
func referenceValue(path string, v any) any {
	typ := "value"
	switch v.(type) {
	case map[string]any:
		typ = "object"
	case []any:
		typ = "array"
	case string:
		typ = "string"
	}
	return map[string]any{"type": "reference", "value_type": typ, "path": path}
}
func readPage(token, path string, offset, limit int, v any) (map[string]any, error) {
	if offset < 0 || limit < 0 {
		return nil, errors.New("negative offset or limit")
	}
	if limit == 0 {
		limit = 8192
	}
	limit = min(limit, 8192)
	if s, ok := v.(string); ok {
		for {
			f, n, done, e := utf8Fragment(s, offset, limit)
			if e != nil {
				return nil, e
			}
			out := snapshotEnvelope(token, "string", path, offset, done, nil, f, &n)
			if fitsResult(envelopeResult(out)) {
				return out, nil
			}
			if limit <= 4 {
				return nil, errors.New("result path exceeds response budget")
			}
			limit = max(4, limit/2)
		}
	}
	var count int
	var keys []string
	switch x := v.(type) {
	case []any:
		count = len(x)
	case map[string]any:
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		count = len(keys)
	default:
		out := snapshotEnvelope(token, "value", path, offset, true, nil, v, nil)
		if !fitsResult(envelopeResult(out)) {
			return nil, errors.New("result path exceeds response budget")
		}
		return out, nil
	}
	if offset > count {
		return nil, errors.New("offset out of range")
	}
	items := []any{}
	end := offset
	for end < count && end-offset < limit {
		var key string
		var value any
		switch x := v.(type) {
		case []any:
			key = strconv.Itoa(end)
			value = x[end]
		case map[string]any:
			key = keys[end]
			value = x[key]
		}
		child := pointerChild(path, key)
		entry := map[string]any{"key": key, "path": child, "value": value}
		if !fitsResult(envelopeResult(map[string]any{"data": entry})) {
			entry["value"] = referenceValue(child, value)
		}
		candidate := append(append([]any(nil), items...), entry)
		next := end + 1
		out := snapshotEnvelope(token, "entries", path, offset, next == count, candidate, nil, &next)
		if !fitsResult(envelopeResult(out)) {
			entry["value"] = referenceValue(child, value)
			out = snapshotEnvelope(token, "entries", path, offset, next == count, candidate, nil, &next)
		}
		if !fitsResult(envelopeResult(out)) {
			if len(items) == 0 {
				return nil, errors.New("object key or result path exceeds response budget; narrow the original query")
			}
			break
		}
		items = candidate
		end++
	}
	return snapshotEnvelope(token, "entries", path, offset, end == count, items, nil, &end), nil
}
