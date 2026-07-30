package mgsctl

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestBuildInfoFormatsStableTextAndJSON(t *testing.T) {
	info := NormalizeBuildInfo(BuildInfo{
		Version:   "v1.2.3",
		Commit:    "0123456789abcdef",
		BuildTime: "2026-07-28T00:00:00Z",
		Dirty:     false,
	})

	text := info.Text()
	for _, required := range []string{
		"mgsctl v1.2.3",
		"commit: 0123456789abcdef",
		"built: 2026-07-28T00:00:00Z",
		"dirty: false",
		"go: " + runtime.Version(),
		"platform: " + runtime.GOOS + "/" + runtime.GOARCH,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("text build info missing %q:\n%s", required, text)
		}
	}

	encoded, err := info.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode build info JSON: %v", err)
	}
	want := map[string]any{
		"version":    "v1.2.3",
		"commit":     "0123456789abcdef",
		"build_time": "2026-07-28T00:00:00Z",
		"dirty":      false,
		"go_version": runtime.Version(),
		"go_os":      runtime.GOOS,
		"go_arch":    runtime.GOARCH,
	}
	for key, value := range want {
		if document[key] != value {
			t.Errorf("JSON field %s = %#v, want %#v", key, document[key], value)
		}
	}
}

func TestBuildInfoNormalizesMissingLinkerValuesAsDevelopmentBuild(t *testing.T) {
	info := NormalizeBuildInfo(BuildInfo{})
	if info.Version != "dev" || info.Commit != "unknown" || info.BuildTime != "unknown" || info.Dirty {
		t.Fatalf("normalized development build = %#v", info)
	}
	if !strings.Contains(info.Text(), "mgsctl dev") {
		t.Fatalf("development build text = %q", info.Text())
	}
}
