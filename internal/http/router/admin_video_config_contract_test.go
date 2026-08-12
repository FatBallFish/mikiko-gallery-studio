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
		"/api/ops/admin/v1/video-pricing-strategies",
		"/api/ops/admin/v1/video-pricing-strategies/",
		"/api/ops/admin/v1/video-price-rules/",
		"/api/ops/admin/v1/route-models/",
	} {
		if !strings.Contains(string(routerSource), `mux.HandleFunc("`+path+`"`) {
			t.Errorf("router must register %s", path)
		}
	}
	for _, path := range []string{
		"/api/ops/admin/v1/model-account-models/{id}/video-capability:",
		"/api/ops/admin/v1/model-account-models/{id}/video-cost-rules:",
		"/api/ops/admin/v1/video-pricing-strategies:",
		"/api/ops/admin/v1/video-pricing-strategies/{id}:",
		"/api/ops/admin/v1/video-pricing-strategies/{id}:simulate:",
		"/api/ops/admin/v1/video-pricing-strategies/{id}:recalculate:",
		"/api/ops/admin/v1/video-price-rules/{id}:",
		"/api/ops/admin/v1/route-models/{id}/video-config:",
		"/api/ops/admin/v1/route-models/{id}/video-impact:",
	} {
		if !strings.Contains(string(openAPI), "  "+path) {
			t.Errorf("OpenAPI must document %s", strings.TrimSuffix(path, ":"))
		}
	}
}
