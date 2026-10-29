package schemagen

import "gopkg.in/yaml.v3"

func syncTransfers(paths *yaml.Node, changed map[string]bool) {
	for _, kind := range []string{"image", "video"} {
		path := "/api/open/v1/" + kind + "-tasks/{task_id}/events"
		op := get(get(paths, path), "get")
		rs := decode(get(op, "responses"))
		r := rs["200"].(map[string]any)
		r["description"] = "SSE task.status events carry the raw task schema below; id is a content hash. Terminal task closes the stream. Later authorization loss emits event:error with {error:string}. Reconnect by reading current task; Last-Event-ID replay is not supported."
		r["x-event-data-schema"] = ref("./components/schemas/native.generated.json#/$defs/get_" + kind + "_taskOutput")
		set(op, "responses", rs)
		changed[path] = true
	}
	part := obj(map[string]any{"part_number": scalar("integer"), "etag": scalar("string"), "checksum": scalar("string"), "size_bytes": scalar("integer")}, "part_number", "etag")
	for _, path := range []string{"/api/open/v1/uploads/{upload_id}/parts/{part_number}", "/api/open/v1/upload-access/{upload_id}/parts/{part_number}"} {
		op := get(get(paths, path), "put")
		rs := decode(get(op, "responses"))
		rs["200"] = response(envelope(part))
		set(op, "responses", rs)
		changed[path] = true
	}
	path := "/api/open/v1/asset-access/{grant}/content"
	methods := map[string]any{}
	for _, method := range []string{"get", "head"} {
		rs := map[string]any{}
		for _, code := range []string{"200", "206", "416"} {
			headers := map[string]any{"Accept-Ranges": map[string]any{"schema": map[string]any{"type": "string", "const": "bytes"}}, "Cache-Control": map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}}, "Content-Length": map[string]any{"schema": scalar("integer")}}
			if code != "200" {
				headers["Content-Range"] = map[string]any{"schema": scalar("string")}
			}
			if code != "416" {
				headers["Content-Disposition"] = map[string]any{"schema": scalar("string"), "description": "attachment for download purpose, inline otherwise"}
			}
			r := map[string]any{"description": "Resource capability content; 206 is a single byte range, 416 is unsatisfiable or invalid range (empty body).", "headers": headers}
			if method == "get" && code != "416" {
				r["content"] = map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}
			}
			rs[code] = r
		}
		for _, code := range []string{"400", "401", "403", "404", "500", "503"} {
			rs[code] = response(ref("./components/error-components.yaml#/components/schemas/ErrorResponse"))
		}
		methods[method] = map[string]any{"operationId": method + "DeveloperAssetContent", "tags": []string{"Developer API"}, "summary": "Read resource capability content", "description": "Use the URL returned by an access operation. Signed resource-only path capability, expires within five minutes; no Bearer header required. Current principal and resource access are revalidated. Never place API or OAuth credentials in URL parameters.", "security": []any{}, "parameters": []any{map[string]any{"name": "grant", "in": "path", "required": true, "schema": scalar("string")}, map[string]any{"name": "Range", "in": "header", "schema": scalar("string")}}, "responses": rs}
	}
	set(paths, path, methods)
	changed[path] = true
}
