package handlers

import (
	"net/http"
	"testing"
)

func TestOpenPlatformNativeRoutes(t *testing.T) {
	cases := []struct{ method, path, op string }{
		{http.MethodGet, "me", "get_current_user"}, {http.MethodPost, "image-tasks/preflight", "preflight_image_task"}, {http.MethodPost, "video-tasks", "create_video_task"}, {http.MethodGet, "video-tasks/task", "get_video_task"}, {http.MethodPost, "video-tasks/task/cancel", "cancel_video_task"}, {http.MethodPost, "uploads/upload/parts/2/url", "get_upload_part_url"}, {http.MethodPost, "uploads/upload/complete", "complete_upload"}, {http.MethodPatch, "assets/asset", "update_asset"}, {http.MethodDelete, "groups/group", "delete_group"}, {http.MethodGet, "projects", "list_projects"},
	}
	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			parts := splitOpenTestPath(tt.path)
			op, extra := openRoute(parts, tt.method)
			if op != tt.op {
				t.Fatalf("got %s want %s", op, tt.op)
			}
			if tt.op == "get_upload_part_url" && extra["part_number"] != 2 {
				t.Fatal("part number lost")
			}
		})
	}
	if op, _ := openRoute([]string{"uploads"}, http.MethodGet); op != "" {
		t.Fatal("unsupported upload list routed")
	}
}
func splitOpenTestPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	return append(parts, path[start:])
}
