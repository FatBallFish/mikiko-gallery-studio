package openapi

import (
	"os"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"gopkg.in/yaml.v3"
)

func TestUnifiedDeveloperContractMatchesOperationCatalogue(t *testing.T) {
	raw, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string                `yaml:"operationId"`
			Scopes      []string              `yaml:"x-required-scopes"`
			Security    []map[string][]string `yaml:"security"`
		} `yaml:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	operations := map[string]string{}
	for path, methods := range doc.Paths {
		if strings.HasPrefix(path, "/api/open/image/v1/") && !strings.Contains(path, "/gallery/") && !strings.Contains(path, "/payments/webhooks/") {
			t.Errorf("obsolete signed route retained: %s", path)
		}
		if !strings.HasPrefix(path, "/api/open/v1/") {
			continue
		}
		for method, op := range methods {
			if path == "/api/open/v1/upload-access/{upload_id}/parts/{part_number}" {
				if method != "put" || op.OperationID != "upload_capability_part" || len(op.Security) != 0 {
					t.Errorf("upload capability route must be PUT without developer Bearer")
				}
				continue
			}
			if path == "/api/open/v1/asset-access/{grant}/content" {
				if (method != "get" && method != "head") || len(op.Security) != 0 {
					t.Errorf("asset capability must be GET/HEAD without developer Bearer")
				}
				continue
			}
			if len(op.Security) != 1 {
				t.Errorf("%s must require Bearer", path)
				continue
			}
			if _, ok := op.Security[0]["developerBearerAuth"]; !ok {
				t.Errorf("%s must use developerBearerAuth", path)
			}
			if len(op.Scopes) == 1 {
				operations[op.OperationID] = op.Scopes[0]
			}
		}
	}
	for _, spec := range openplatform.Specs() {
		if operations[spec.Name] != spec.Scope {
			t.Errorf("%s: spec scope %q differs from catalogue %q", spec.Name, operations[spec.Name], spec.Scope)
		}
		if spec.Name == "create_image_task" || spec.Name == "create_video_task" {
			name := "DeveloperImageCreateRequest"
			if spec.Name == "create_video_task" {
				name = "DeveloperVideoCreateRequest"
			}
			properties, _ := spec.InputSchema["properties"].(map[string]any)
			for field := range properties {
				if _, ok := doc.Components.Schemas[name].Properties[field]; !ok {
					t.Errorf("%s missing catalogue field %s", name, field)
				}
			}
		}
	}
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp", "/oauth/authorize", "/oauth/token", "/oauth/revoke", "/oauth/register", "/mcp", "/api/v1/oauth/consent", "/api/v1/oauth/connected-apps", "/api/open/image/v1/gallery/images", "/api/public/v1/videos"} {
		if _, ok := doc.Paths[path]; !ok {
			t.Errorf("missing preserved/OAuth route %s", path)
		}
	}
	for _, obsolete := range []string{"X-Access-Key", "X-Signature", "accessSignature", "accessBodySHA256"} {
		if strings.Contains(string(raw), obsolete) {
			t.Errorf("obsolete developer auth %s", obsolete)
		}
	}
}
