package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNativeOptionsQueryPreservesFalseAndRejectsUnknown(t *testing.T) {
	for _, test := range []struct {
		raw   string
		valid bool
	}{
		{`{"watermark":false,"qwen":{"prompt_extend":false,"enable_thinking":false}}`, true},
		{`{"unknown":true}`, false},
		{`{} {}`, false},
		{strings.Repeat("x", 16385), false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/?native_options="+url.QueryEscape(test.raw), nil)
		got, err := parseNativeOptionsQuery(r)
		if test.valid {
			if err != nil || got.Watermark == nil || *got.Watermark || got.Qwen.PromptExtend == nil || *got.Qwen.PromptExtend {
				t.Fatalf("false lost: %#v %v", got, err)
			}
		} else if err == nil {
			t.Fatalf("accepted invalid options %q", test.raw)
		}
	}
}

func TestOpenLayerManifestRoute(t *testing.T) {
	op, extra := openRoute([]string{"image-tasks", "owned-task", "layers"}, http.MethodGet)
	if op != "get_image_task_layers" || extra["id"] != "owned-task" {
		t.Fatalf("route = %s %#v", op, extra)
	}
	op, _ = openRoute([]string{"video-tasks", "owned-task", "layers"}, http.MethodGet)
	if op != "" {
		t.Fatalf("video layer route unexpectedly matched: %s", op)
	}
}
