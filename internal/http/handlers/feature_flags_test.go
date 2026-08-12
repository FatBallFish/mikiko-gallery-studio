package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
)

func TestFeatureFlagsDefaultClosedAndReadExplicitOverrides(t *testing.T) {
	admin := adminconfigservice.NewService(config.Config{})
	flags := newFeatureFlagResolver(admin)
	projected, err := flags.Get(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if projected.VideoCreation || projected.CreativeCanvas || projected.MediaUpload {
		t.Fatalf("feature defaults must fail closed: %#v", projected)
	}
	tab, err := admin.GetTab(t.Context(), "site")
	if err != nil {
		t.Fatal(err)
	}
	_, err = admin.UpdateTab(t.Context(), domainadminconfig.UpdateTabRequest{TabKey: "site", Version: tab.Version, Items: []domainadminconfig.Item{{ConfigCategory: "features", ConfigKey: "video_creation", ConfigValue: map[string]any{"value": true}, Scope: "global"}}})
	if err != nil {
		t.Fatal(err)
	}
	projected, err = flags.Get(context.Background())
	if err != nil || !projected.VideoCreation || projected.CreativeCanvas || projected.MediaUpload {
		t.Fatalf("feature override=%#v err=%v", projected, err)
	}
}

func TestFeatureGatesAreWiredAtEveryCreationBoundary(t *testing.T) {
	for file, required := range map[string][]string{
		"video.go":         {`requireFeature(r.Context(), "video_creation")`},
		"media_uploads.go": {`requireFeature(r.Context(), "media_upload")`},
		"canvases.go":      {`requireFeature(r.Context(), "creative_canvas")`},
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range required {
			if !strings.Contains(string(source), token) {
				t.Errorf("%s must contain %s", file, token)
			}
		}
	}
}

func TestFeatureGatePolicyPreservesHistoricalReads(t *testing.T) {
	for _, tc := range []struct {
		feature, method, action string
		blocked                 bool
	}{
		{"video_creation", http.MethodPost, "estimate", true}, {"video_creation", http.MethodPost, "create", true}, {"video_creation", http.MethodGet, "list", false}, {"video_creation", http.MethodGet, "detail", false},
		{"media_upload", http.MethodPost, "init", true}, {"media_upload", http.MethodGet, "status", false}, {"media_upload", http.MethodDelete, "abort", false},
		{"creative_canvas", http.MethodPost, "create", true}, {"creative_canvas", http.MethodPatch, "root", true}, {"creative_canvas", http.MethodDelete, "root", true}, {"creative_canvas", http.MethodPut, "document", true}, {"creative_canvas", http.MethodPost, "duplicate", true}, {"creative_canvas", http.MethodPost, "transfer-project", true}, {"creative_canvas", http.MethodPost, "estimate", true}, {"creative_canvas", http.MethodPost, "generate", true}, {"creative_canvas", http.MethodGet, "list", false}, {"creative_canvas", http.MethodGet, "detail", false}, {"creative_canvas", http.MethodGet, "runs", false}, {"creative_canvas", http.MethodPost, "attach-results", false},
	} {
		if got := featureWriteBlocked(tc.feature, tc.method, tc.action); got != tc.blocked {
			t.Errorf("%s %s %s blocked=%v want=%v", tc.feature, tc.method, tc.action, got, tc.blocked)
		}
	}
}

func TestFeatureProjectionHandlerReturnsAllFlags(t *testing.T) {
	admin := adminconfigservice.NewService(config.Config{})
	api := NewAPI(config.Config{}, nil, nil)
	api.admin = admin
	request := httptest.NewRequest(http.MethodGet, "/api/agent/features/v1", nil)
	recorder := httptest.NewRecorder()
	api.HandleAgentFeatures(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"video_creation":false`) || !strings.Contains(recorder.Body.String(), `"creative_canvas":false`) || !strings.Contains(recorder.Body.String(), `"media_upload":false`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
