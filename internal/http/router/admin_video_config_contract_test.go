package router

import (
	"os"
	"strings"
	"testing"
)

func TestAdminVideoConfigurationRoutesAndOpenAPIStayAligned(t *testing.T) {
	routerSource, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	openAPI, err := os.ReadFile("../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/ops/admin/v1/model-account-models/",
		"/api/ops/admin/v1/video-models/",
		"/api/ops/admin/v1/video-routes/",
		"/api/ops/admin/v1/route-models/",
	} {
		if !strings.Contains(string(routerSource), `mux.HandleFunc("`+path+`"`) {
			t.Errorf("router must register %s", path)
		}
	}
	for _, path := range []string{
		"/api/ops/admin/v1/model-account-models/{id}/video-capability:",
		"/api/ops/admin/v1/video-models/{account_model_id}/rate-cards:",
		"/api/ops/admin/v1/video-models/{account_model_id}/rate-cards/{id}:",
		"/api/ops/admin/v1/video-routes/{route_model_id}/quote-simulation:",
		"/api/ops/admin/v1/route-models/{id}/video-config:",
		"/api/ops/admin/v1/route-models/{id}/video-impact:",
		"/api/ops/admin/v1/video-tasks/{task_id}:retry-settlement:",
	} {
		if !strings.Contains(string(openAPI), "  "+path) {
			t.Errorf("OpenAPI must document %s", strings.TrimSuffix(path, ":"))
		}
	}
	for _, obsolete := range []string{
		`mux.HandleFunc("/api/ops/admin/v1/video-pricing-strategies"`,
		`mux.HandleFunc("/api/ops/admin/v1/video-price-rules"`,
	} {
		if strings.Contains(string(routerSource), obsolete) {
			t.Errorf("router must not register obsolete video pricing route %s", obsolete)
		}
	}
}
