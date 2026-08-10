package router

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEveryLiteralNormalMuxRouteHasPreflightMetadata(t *testing.T) {
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "router.go", source, 0)
	if err != nil {
		t.Fatalf("parse router.go: %v", err)
	}
	registered := make(map[string]struct{})
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		literal, literalOK := call.Args[0].(*ast.BasicLit)
		if !ok || !literalOK || literal.Kind != token.STRING || (selector.Sel.Name != "Handle" && selector.Sel.Name != "HandleFunc") {
			return true
		}
		pattern, unquoteErr := strconv.Unquote(literal.Value)
		if unquoteErr == nil {
			registered[pattern] = struct{}{}
		}
		return true
	})

	var spec struct {
		Paths map[string]any `yaml:"paths"`
	}
	openAPI, err := os.ReadFile("../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	if err := yaml.Unmarshal(openAPI, &spec); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	for pattern := range registered {
		if !normalMuxPatternHasPreflightMetadata(pattern, spec.Paths) {
			t.Errorf("literal normal mux route %q has no exact OpenAPI or explicit supplemental preflight metadata", pattern)
		}
	}
	if normalMuxPatternHasPreflightMetadata("/api/unregistered/v1/items", spec.Paths) {
		t.Fatal("unregistered route unexpectedly has preflight metadata")
	}
}

func TestNormalPreflightFailsClosedWithoutValidatedRouteContract(t *testing.T) {
	policy := normalPreflightPolicy(true, nil)
	if policy("/api/agent/auth/v1/login/password", http.MethodPost) {
		t.Fatal("normal business preflight must fail closed without a validated OpenAPI route contract")
	}
}

func TestImageTaskCreateOpenAPIResponsesDocumentCapabilityChanged(t *testing.T) {
	var spec struct {
		Paths map[string]struct {
			Post struct {
				Responses map[string]struct {
					Description string `yaml:"description"`
				} `yaml:"responses"`
			} `yaml:"post"`
		} `yaml:"paths"`
	}
	openAPI, err := os.ReadFile("../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	if err := yaml.Unmarshal(openAPI, &spec); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	for _, path := range []string{"/api/agent/image/v1/tasks", "/api/open/image/v1/tasks"} {
		response, ok := spec.Paths[path].Post.Responses["409"]
		if !ok || !strings.Contains(response.Description, "capability_changed") {
			t.Errorf("POST %s must document 409 capability_changed, got %#v", path, response)
		}
	}
}

func TestProjectOpenAPIContractDocumentsLifecycleAndScoping(t *testing.T) {
	type operation struct {
		Parameters []struct {
			Name string `yaml:"name"`
		} `yaml:"parameters"`
	}
	type pathItem struct {
		Get    *operation `yaml:"get"`
		Post   *operation `yaml:"post"`
		Patch  *operation `yaml:"patch"`
		Delete *operation `yaml:"delete"`
	}
	var spec struct {
		Paths map[string]pathItem `yaml:"paths"`
	}
	openAPI, err := os.ReadFile("../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	if err := yaml.Unmarshal(openAPI, &spec); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	for path, methods := range map[string][]string{
		"/api/agent/project/v1/projects":                           {"get", "post"},
		"/api/agent/project/v1/projects/{project_id}":              {"patch", "delete"},
		"/api/agent/image/v1/reference-assets:import-from-gallery": {"post"},
		"/api/agent/gallery/v1/images":                             {"get"},
		"/api/agent/gallery/v1/images:batch-publish":               {"post"},
		"/api/agent/gallery/v1/images:batch-group":                 {"post"},
		"/api/agent/gallery/v1/images:batch-delete":                {"post"},
		"/api/agent/gallery/v1/images:batch-transfer-project":      {"post"},
		"/api/agent/gallery/v1/images:batch-download":              {"post"},
		"/api/agent/gallery/v1/export-jobs/{job_id}":               {"get"},
		"/api/agent/gallery/v1/export-jobs/{job_id}/download":      {"get"},
	} {
		for _, method := range methods {
			item := spec.Paths[path]
			present := map[string]*operation{"get": item.Get, "post": item.Post, "patch": item.Patch, "delete": item.Delete}[method] != nil
			if !present {
				t.Errorf("OpenAPI must document %s %s", method, path)
			}
		}
	}
	for _, path := range []string{"/api/agent/image/v1/tasks", "/api/agent/image/v1/history/tasks", "/api/agent/gallery/v1/images"} {
		found := false
		for _, parameter := range spec.Paths[path].Get.Parameters {
			if parameter.Name == "project_id" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GET %s must document project_id filtering", path)
		}
	}
	commonSchema, err := os.ReadFile("../../../api/openapi/components/schemas/common.yaml")
	if err != nil {
		t.Fatalf("read common schemas: %v", err)
	}
	agentSchema, err := os.ReadFile("../../../api/openapi/components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schemas: %v", err)
	}
	for name, source := range map[string]string{"ProjectSnapshot": string(commonSchema), "ProjectResponse": string(agentSchema)} {
		if !strings.Contains(source, "    "+name+":") {
			t.Errorf("OpenAPI schemas must define %s", name)
		}
	}
}

func normalMuxPatternHasPreflightMetadata(pattern string, openAPIPaths map[string]any) bool {
	if separator := strings.IndexByte(pattern, ' '); separator >= 0 {
		pattern = pattern[separator+1:]
	}
	switch pattern {
	case "/", "/setup", "/setup/", "/api/", "/healthz", "/readyz", "/api/system/v1/bootstrap-status", "/metrics":
		return true
	}
	if strings.HasPrefix(pattern, "/debug/pprof/") {
		return true
	}
	for documented := range openAPIPaths {
		if documented == pattern || (strings.HasSuffix(pattern, "/") && strings.HasPrefix(documented, pattern)) {
			return true
		}
	}
	for supplemental := range supplementalNormalExactRoutes {
		if supplemental == pattern || (strings.HasSuffix(pattern, "/") && strings.HasPrefix(supplemental, pattern)) {
			return true
		}
	}
	for supplemental := range supplementalNormalTemplateRoutes {
		if supplemental == pattern || (strings.HasSuffix(pattern, "/") && strings.HasPrefix(supplemental, pattern)) {
			return true
		}
	}
	return false
}
