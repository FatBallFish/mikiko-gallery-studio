package openapi

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPISpecCoversP0Paths(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	expected := []string{
		"/api/agent/auth/v1/email/send-code",
		"/api/agent/image/v1/capabilities",
		"/api/agent/image/v1/tasks",
		"/api/open/image/v1/reference-assets/uploads",
		"/api/open/image/v1/tasks",
		"/api/open/image/v1/estimate",
		"/api/ops/admin/v1/config-tabs/{tab_key}",
		"/v1/images/generations",
		"/v1/images/edits",
		"/v1/models",
	}

	for _, path := range expected {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("expected path %q in OpenAPI spec", path)
		}
	}
}

func TestOpenAPISpecDocumentsOpenImageAuthAndUploadContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Security   []map[string][]string `yaml:"security"`
			Parameters []struct {
				Name string `yaml:"name"`
				In   string `yaml:"in"`
				Ref  string `yaml:"$ref"`
			} `yaml:"parameters"`
		} `yaml:"paths"`
		Components struct {
			SecuritySchemes map[string]struct {
				BearerFormat string `yaml:"bearerFormat"`
			} `yaml:"securitySchemes"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	for path, method := range map[string]string{
		"/api/open/image/v1/reference-assets/uploads": "post",
		"/api/open/image/v1/tasks":                    "post",
		"/api/open/image/v1/estimate":                 "get",
	} {
		operation := doc.Paths[path][method]
		if len(operation.Security) != 1 || operation.Security[0]["accessKeyAuth"] == nil || operation.Security[0]["accessSignature"] == nil {
			t.Fatalf("expected %s %s to require both accessKeyAuth and accessSignature, got %#v", method, path, operation.Security)
		}
	}
	taskParams := map[string]bool{}
	for _, param := range doc.Paths["/api/open/image/v1/tasks"]["post"].Parameters {
		taskParams[param.In+":"+param.Name] = true
		taskParams[param.Ref] = true
	}
	if !taskParams["header:Idempotency-Key"] && !taskParams["./components/parameters/common.yaml#/components/parameters/IdempotencyKey"] {
		t.Fatal("expected open task create to document Idempotency-Key header")
	}
	for path, method := range map[string]string{
		"/v1/images/generations": "post",
		"/v1/images/edits":       "post",
		"/v1/models":             "get",
	} {
		operation := doc.Paths[path][method]
		if len(operation.Security) != 1 || operation.Security[0]["compatBearerAuth"] == nil {
			t.Fatalf("expected %s %s to use compatBearerAuth, got %#v", method, path, operation.Security)
		}
	}
	if doc.Components.SecuritySchemes["compatBearerAuth"].BearerFormat != "sk-*" {
		t.Fatalf("expected compatBearerAuth bearerFormat sk-*, got %#v", doc.Components.SecuritySchemes["compatBearerAuth"])
	}

	schemaContent, err := os.ReadFile("components/schemas/openapi.yaml")
	if err != nil {
		t.Fatalf("read open api schema: %v", err)
	}
	var schemaDoc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemaDoc); err != nil {
		t.Fatalf("unmarshal open api schema: %v", err)
	}
	for _, name := range []string{"UploadSessionRequest", "UploadSessionResponse"} {
		if _, ok := schemaDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected %s schema in open api schema file", name)
		}
	}
}

func TestOpenAPISpecDocumentsLedgerContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Paths map[string]struct {
			Get struct {
				Parameters []struct {
					Name string `yaml:"name"`
				} `yaml:"parameters"`
				Responses map[string]struct {
					Headers map[string]any `yaml:"headers"`
				} `yaml:"responses"`
			} `yaml:"get"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	ledgerPath, ok := doc.Paths["/api/agent/billing/v1/ledger"]
	if !ok {
		t.Fatal("expected ledger path in OpenAPI spec")
	}
	balancePath, ok := doc.Paths["/api/agent/billing/v1/balance"]
	if !ok {
		t.Fatal("expected balance path in OpenAPI spec")
	}
	paramNames := map[string]bool{}
	for _, param := range ledgerPath.Get.Parameters {
		paramNames[param.Name] = true
	}
	for _, name := range []string{"page", "page_size"} {
		if !paramNames[name] {
			t.Fatalf("expected ledger query parameter %q in OpenAPI spec", name)
		}
	}
	if _, ok := ledgerPath.Get.Responses["400"]; !ok {
		t.Fatal("expected ledger path to document 400 response for invalid pagination")
	}
	if _, ok := balancePath.Get.Responses["401"]; !ok {
		t.Fatal("expected balance path to document 401 response")
	}
	if _, ok := ledgerPath.Get.Responses["401"]; !ok {
		t.Fatal("expected ledger path to document 401 response")
	}
	if _, ok := balancePath.Get.Responses["405"]; !ok {
		t.Fatal("expected balance path to document 405 response")
	}
	if _, ok := ledgerPath.Get.Responses["405"]; !ok {
		t.Fatal("expected ledger path to document 405 response")
	}
	if _, ok := balancePath.Get.Responses["200"].Headers["X-Request-Id"]; !ok {
		t.Fatal("expected balance 200 response to document X-Request-Id header")
	}
	if _, ok := ledgerPath.Get.Responses["200"].Headers["X-Request-Id"]; !ok {
		t.Fatal("expected ledger 200 response to document X-Request-Id header")
	}

	schemaContent, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schema: %v", err)
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Pattern string `yaml:"pattern"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal agent schema: %v", err)
	}
	ledgerEntry, ok := schemasDoc.Components.Schemas["PointLedgerEntry"]
	if !ok {
		t.Fatal("expected PointLedgerEntry schema in agent schema file")
	}
	for _, field := range []string{"task_id", "frozen_after", "reason"} {
		if _, ok := ledgerEntry.Properties[field]; !ok {
			t.Fatalf("expected PointLedgerEntry field %q in agent schema", field)
		}
	}
	for _, field := range []string{"change_points", "balance_after", "frozen_after"} {
		if ledgerEntry.Properties[field].Pattern == "" {
			t.Fatalf("expected PointLedgerEntry field %q to constrain decimal precision", field)
		}
	}
	balanceSummary, ok := schemasDoc.Components.Schemas["BalanceSummary"]
	if !ok {
		t.Fatal("expected BalanceSummary schema in agent schema file")
	}
	for _, field := range []string{"available_points", "frozen_points", "user_group_multiplier", "cny_per_point"} {
		if balanceSummary.Properties[field].Pattern == "" {
			t.Fatalf("expected BalanceSummary field %q to constrain decimal precision", field)
		}
	}
	for _, schemaName := range []string{"BalanceResponse", "PointLedgerPageResponse"} {
		schema, ok := schemasDoc.Components.Schemas[schemaName]
		if !ok {
			t.Fatalf("expected %s schema in agent schema file", schemaName)
		}
		if _, ok := schema.Properties["meta"]; !ok {
			t.Fatalf("expected %s schema to document response meta", schemaName)
		}
	}
}
