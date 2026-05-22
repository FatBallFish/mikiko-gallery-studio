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
		"/api/agent/auth/v1/login/password",
		"/api/agent/auth/v1/password/change",
		"/api/agent/auth/v1/password/reset/request",
		"/api/agent/auth/v1/password/reset/confirm",
		"/api/agent/user/v1/account/close",
		"/api/agent/billing/v1/plans",
		"/api/agent/billing/v1/subscription",
		"/api/agent/billing/v1/orders",
		"/api/agent/billing/v1/orders/{order_id}",
		"/api/agent/gallery/v1/images/{image_id}/publish",
		"/api/agent/image/v1/capabilities",
		"/api/agent/image/v1/tasks",
		"/api/open/image/v1/reference-assets/uploads",
		"/api/open/image/v1/tasks",
		"/api/open/image/v1/estimate",
		"/api/open/image/v1/gallery/images",
		"/api/open/image/v1/payments/webhooks/{channel}",
		"/api/ops/admin/v1/config-tabs/{tab_key}",
		"/api/ops/admin/v1/audit-logs",
		"/api/ops/admin/v1/image-reviews",
		"/api/ops/admin/v1/users",
		"/api/ops/admin/v1/users/{user_id}",
		"/api/ops/admin/v1/users/{user_id}/status",
		"/api/ops/admin/v1/users/{user_id}/reset-password",
		"/api/ops/admin/v1/users/{user_id}/limits",
		"/api/ops/admin/v1/users/{user_id}/group",
		"/api/ops/admin/v1/user-groups",
		"/api/ops/admin/v1/users/{user_id}/points-adjustments",
		"/api/ops/admin/v1/redeem-codes",
		"/api/ops/admin/v1/redeem-codes:batch-create",
		"/api/ops/admin/v1/redeem-codes/{code_id}/status",
		"/api/ops/admin/v1/redeem-codes/{code_id}/redemptions",
		"/api/ops/admin/v1/model-providers",
		"/api/ops/admin/v1/provider-models",
		"/api/ops/admin/v1/model-providers/{provider_code}",
		"/api/ops/admin/v1/model-routes",
		"/api/ops/admin/v1/model-routes/{route_id}",
		"/api/ops/admin/v1/metrics/dashboard",
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

func TestOpenAPISpecDocumentsAdminModelRoutingContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Tags []struct {
			Name string `yaml:"name"`
		} `yaml:"tags"`
		Paths map[string]map[string]struct {
			Tags       []string `yaml:"tags"`
			Parameters []struct {
				Name string `yaml:"name"`
				In   string `yaml:"in"`
			} `yaml:"parameters"`
			RequestBody struct {
				Required bool `yaml:"required"`
			} `yaml:"requestBody"`
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	seenTag := false
	for _, tag := range doc.Tags {
		if tag.Name == "Admin Model Routing" {
			seenTag = true
			break
		}
	}
	if !seenTag {
		t.Fatal("expected Admin Model Routing tag")
	}

	for path, method := range map[string]string{
		"/api/ops/admin/v1/model-providers":                 "get",
		"/api/ops/admin/v1/model-providers/{provider_code}": "get",
		"/api/ops/admin/v1/model-routes":                    "get",
		"/api/ops/admin/v1/model-routes/{route_id}":         "get",
	} {
		operation := doc.Paths[path][method]
		if len(operation.Tags) != 1 || operation.Tags[0] != "Admin Model Routing" {
			t.Fatalf("expected %s %s to use Admin Model Routing tag, got %#v", method, path, operation.Tags)
		}
	}

	providerListParams := map[string]bool{}
	for _, param := range doc.Paths["/api/ops/admin/v1/model-providers"]["get"].Parameters {
		providerListParams[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:provider_type", "query:enabled"} {
		if !providerListParams[key] {
			t.Fatalf("expected model provider list parameter %q", key)
		}
	}
	routeListParams := map[string]bool{}
	for _, param := range doc.Paths["/api/ops/admin/v1/model-routes"]["get"].Parameters {
		routeListParams[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:group_code", "query:task_type", "query:provider_code", "query:enabled"} {
		if !routeListParams[key] {
			t.Fatalf("expected model route list parameter %q", key)
		}
	}

	for path, methods := range map[string][]string{
		"/api/ops/admin/v1/model-providers":                 {"post"},
		"/api/ops/admin/v1/model-providers/{provider_code}": {"put"},
		"/api/ops/admin/v1/model-routes":                    {"post"},
		"/api/ops/admin/v1/model-routes/{route_id}":         {"put"},
	} {
		for _, method := range methods {
			if !doc.Paths[path][method].RequestBody.Required {
				t.Fatalf("expected %s %s request body to be required", method, path)
			}
		}
	}
	for path, method := range map[string]string{
		"/api/ops/admin/v1/model-providers/{provider_code}": "delete",
		"/api/ops/admin/v1/model-routes/{route_id}":         "delete",
	} {
		if _, ok := doc.Paths[path][method].Responses["204"]; !ok {
			t.Fatalf("expected %s %s to document 204 response", method, path)
		}
	}

	schemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `yaml:"required"`
				AnyOf    []struct {
					Required []string `yaml:"required"`
				} `yaml:"anyOf"`
				Properties map[string]any
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}
	for _, name := range []string{"AdminModelProvider", "AdminModelProviderWriteRequest", "AdminModelProviderResponse", "AdminModelProviderListResponse", "AdminModelRoute", "AdminModelRouteWriteRequest", "AdminModelRouteResponse", "AdminModelRouteListResponse"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin model routing schema %q", name)
		}
	}
	for schemaName, requiredFields := range map[string][]string{
		"AdminModelProviderWriteRequest": {"provider_code", "provider_type"},
		"AdminModelRouteWriteRequest":    {"group_code", "task_type"},
	} {
		required := map[string]bool{}
		for _, field := range schemasDoc.Components.Schemas[schemaName].Required {
			required[field] = true
		}
		for _, field := range requiredFields {
			if !required[field] {
				t.Fatalf("expected %s to require %q", schemaName, field)
			}
		}
	}
	routeWrite := schemasDoc.Components.Schemas["AdminModelRouteWriteRequest"]
	routeAnyOf := map[string]bool{}
	for _, option := range routeWrite.AnyOf {
		for _, field := range option.Required {
			routeAnyOf[field] = true
		}
	}
	for _, field := range []string{"provider_code", "provider_model_id"} {
		if !routeAnyOf[field] {
			t.Fatalf("expected AdminModelRouteWriteRequest anyOf to include %q", field)
		}
	}
}

func TestOpenAPISpecDocumentsAdminUserManagementContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `yaml:"name"`
				In   string `yaml:"in"`
				Ref  string `yaml:"$ref"`
			} `yaml:"parameters"`
			RequestBody struct {
				Required bool `yaml:"required"`
			} `yaml:"requestBody"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	listParams := map[string]bool{}
	for _, param := range doc.Paths["/api/ops/admin/v1/users"]["get"].Parameters {
		listParams[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:query", "query:status"} {
		if !listParams[key] {
			t.Fatalf("expected admin user list parameter %q", key)
		}
	}

	adjust := doc.Paths["/api/ops/admin/v1/users/{user_id}/points-adjustments"]["post"]
	if !adjust.RequestBody.Required {
		t.Fatal("expected admin point adjustment body to be required")
	}
	seenIDKey := false
	for _, param := range adjust.Parameters {
		if param.Ref == "./components/parameters/common.yaml#/components/parameters/RequiredIdempotencyKey" || (param.In == "header" && param.Name == "Idempotency-Key") {
			seenIDKey = true
		}
	}
	if !seenIDKey {
		t.Fatal("expected admin point adjustment to document Idempotency-Key header")
	}

	schemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `yaml:"required"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}
	for _, name := range []string{"AdminUserSummary", "AdminUserListResponse", "AdminUserDetailResponse", "AdminPointAdjustmentRequest", "AdminUpdateUserStatusRequest"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin schema %q", name)
		}
	}
	for _, name := range []string{"AdminRedeemCode", "AdminCreateRedeemCodeRequest", "AdminBatchCreateRedeemCodesRequest", "AdminRedeemCodeResponse", "AdminRedeemCodeListResponse", "AdminRedeemCodeRedemptionsResponse"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin redeem schema %q", name)
		}
	}
	required := map[string]bool{}
	for _, field := range schemasDoc.Components.Schemas["AdminPointAdjustmentRequest"].Required {
		required[field] = true
	}
	for _, field := range []string{"change_points", "reason"} {
		if !required[field] {
			t.Fatalf("expected AdminPointAdjustmentRequest to require %q", field)
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

func TestOpenAPISpecDocumentsNativeTaskAsyncOnlyContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	for _, path := range []string{"/api/agent/image/v1/tasks", "/api/open/image/v1/tasks"} {
		if _, ok := doc.Paths[path]["post"].Responses["400"]; !ok {
			t.Fatalf("expected %s post to document 400 for unsupported sync response_mode", path)
		}
	}

	schemaContent, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schema: %v", err)
	}
	var schemaDoc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemaDoc); err != nil {
		t.Fatalf("unmarshal agent schema: %v", err)
	}
	responseMode := schemaDoc.Components.Schemas["CreateImageTaskRequest"].Properties["response_mode"]
	if len(responseMode.Enum) != 1 || responseMode.Enum[0] != "async" {
		t.Fatalf("expected native CreateImageTaskRequest response_mode enum to be async-only, got %#v", responseMode.Enum)
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
