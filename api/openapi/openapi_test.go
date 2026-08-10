package openapi

import (
	"os"
	"strings"
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
		"/api/agent/cashier/v1/options",
		"/api/agent/cashier/v1/orders",
		"/api/agent/cashier/v1/orders/{order_id}",
		"/api/agent/cashier/v1/orders/{order_id}/mock-pay",
		"/api/agent/gallery/v1/images/{image_id}/publish",
		"/api/agent/image/v1/capabilities",
		"/api/agent/image/v1/tasks",
		"/api/open/image/v1/reference-assets/uploads",
		"/api/open/image/v1/tasks",
		"/api/open/image/v1/estimate",
		"/api/open/image/v1/gallery/images",
		"/api/open/image/v1/payments/webhooks/{channel}",
		"/api/ops/admin/v1/config-tabs/{tab_key}",
		"/api/ops/admin/v1/security/smtp",
		"/api/ops/admin/v1/security/smtp/test",
		"/api/ops/admin/v1/storage-configs",
		"/api/ops/admin/v1/storage-configs:probe",
		"/api/ops/admin/v1/storage-configs/{storage_config_id}",
		"/api/ops/admin/v1/storage-configs/{storage_config_id}:probe",
		"/api/ops/admin/v1/storage-configs/{storage_config_id}:set-default",
		"/api/ops/admin/v1/storage-configs/{storage_config_id}:set-status",
		"/api/ops/admin/v1/admin-users",
		"/api/ops/admin/v1/admin-users/{admin_id}",
		"/api/ops/admin/v1/admin-users/{admin_id}/reset-password",
		"/api/ops/admin/v1/audit-logs",
		"/api/ops/admin/v1/image-reviews",
		"/api/ops/admin/v1/users",
		"/api/ops/admin/v1/users/{user_id}",
		"/api/ops/admin/v1/users/{user_id}/status",
		"/api/ops/admin/v1/users/{user_id}/reset-password",
		"/api/ops/admin/v1/users/{user_id}/limits",
		"/api/ops/admin/v1/users/{user_id}/groups",
		"/api/ops/admin/v1/user-groups",
		"/api/ops/admin/v1/users/{user_id}/points-adjustments",
		"/api/ops/admin/v1/redeem-codes",
		"/api/ops/admin/v1/redeem-codes:batch-create",
		"/api/ops/admin/v1/redeem-codes:export",
		"/api/ops/admin/v1/redeem-codes/{code_id}/status",
		"/api/ops/admin/v1/redeem-codes/{code_id}/redemptions",
		"/api/ops/admin/v1/model-accounts",
		"/api/ops/admin/v1/model-accounts/{account_id}",
		"/api/ops/admin/v1/model-accounts/{account_id}/models",
		"/api/ops/admin/v1/model-accounts/{account_id}/models/{model_id}",
		"/api/ops/admin/v1/route-models",
		"/api/ops/admin/v1/route-models/{route_model_id}",
		"/api/ops/admin/v1/route-models/{route_model_id}/candidates",
		"/api/ops/admin/v1/route-model-prices",
		"/api/ops/admin/v1/metrics/dashboard",
		"/api/ops/admin/v1/readiness",
		"/api/ops/admin/v1/cashier/overview",
		"/api/ops/admin/v1/cashier/plans",
		"/api/ops/admin/v1/cashier/custom-amount-config",
		"/api/ops/admin/v1/cashier/visible-methods",
		"/api/ops/admin/v1/cashier/provider-instances",
		"/api/ops/admin/v1/cashier/orders",
		"/api/ops/admin/v1/cashier/orders/{order_id}/chargeback",
		"/api/ops/admin/v1/cashier/orders/{order_id}/sync",
		"/api/ops/admin/v1/cashier/webhook-events",
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

func TestOpenAPISpecDocumentsBootstrapAndPendingSetupWithoutSecretExamples(t *testing.T) {
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
	for _, path := range []string{
		"/setup",
		"/api/system/v1/bootstrap-status",
		"/api/setup/v1/session",
		"/api/setup/v1/probes/database",
		"/api/setup/v1/probes/redis",
		"/api/setup/v1/probes/storage",
		"/api/setup/v1/apply",
		"/api/setup/v1/progress/{operation_id}",
	} {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("expected setup path %q", path)
		}
	}

	var operations struct {
		Paths map[string]map[string]struct {
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &operations); err != nil {
		t.Fatalf("unmarshal setup response contracts: %v", err)
	}
	expectedResponses := map[string][]string{
		"GET /setup":                                {"200"},
		"GET /api/system/v1/bootstrap-status":       {"200"},
		"POST /api/setup/v1/session":                {"200", "400", "401", "409", "429", "500"},
		"POST /api/setup/v1/probes/database":        {"200", "400", "401"},
		"POST /api/setup/v1/probes/redis":           {"200", "400", "401"},
		"POST /api/setup/v1/probes/storage":         {"200", "400", "401"},
		"POST /api/setup/v1/apply":                  {"202", "400", "401", "404", "408", "409", "500", "504"},
		"GET /api/setup/v1/progress/{operation_id}": {"200", "400", "401", "404", "408", "409", "500", "504"},
	}
	for operation, statuses := range expectedResponses {
		parts := strings.SplitN(operation, " ", 2)
		method, path := strings.ToLower(parts[0]), parts[1]
		responses := operations.Paths[path][method].Responses
		for _, status := range statuses {
			if _, ok := responses[status]; !ok {
				t.Fatalf("%s must document actual handler response %s", operation, status)
			}
		}
	}
	if !strings.Contains(string(content), "SETUP_FIRST_ADMIN_CONFLICT") {
		t.Fatal("setup apply 409 response must document SETUP_FIRST_ADMIN_CONFLICT")
	}
	if !strings.Contains(string(content), "SetupSessionResponse") {
		t.Fatal("setup session success must document its recoverable operation response")
	}

	schemaContent, err := os.ReadFile("components/schemas/setup.yaml")
	if err != nil {
		t.Fatalf("read setup schemas: %v", err)
	}
	var schemas struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					WriteOnly bool `yaml:"writeOnly"`
					Example   any  `yaml:"example"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemas); err != nil {
		t.Fatalf("unmarshal setup schemas: %v", err)
	}
	secretFields := map[string][]string{
		"SetupSessionRequest":       {"token"},
		"SetupDatabaseProbeRequest": {"database_url"},
		"SetupRedisProbeRequest":    {"redis_url"},
		"SetupStorageProbeRequest":  {"access_key_id", "secret_access_key"},
		"SetupApplyRequest":         {"runtime", "admin_password"},
	}
	for schemaName, fields := range secretFields {
		schema, ok := schemas.Components.Schemas[schemaName]
		if !ok {
			t.Fatalf("expected setup schema %q", schemaName)
		}
		for _, field := range fields {
			property, ok := schema.Properties[field]
			if !ok || !property.WriteOnly {
				t.Fatalf("%s.%s must be writeOnly", schemaName, field)
			}
			if property.Example != nil {
				t.Fatalf("%s.%s must not contain a secret example", schemaName, field)
			}
		}
	}
}

func TestOpenAPISpecDocumentsTextModelsAndPromptOptimization(t *testing.T) {
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
	for _, path := range []string{
		"/api/agent/text/v1/prompt-optimizations/estimate",
		"/api/agent/text/v1/prompt-optimizations",
		"/api/ops/admin/v1/text-model-accounts",
		"/api/ops/admin/v1/text-model-accounts/{account_id}",
		"/api/ops/admin/v1/text-model-accounts/{account_id}/models",
		"/api/ops/admin/v1/text-models/{model_id}",
		"/api/ops/admin/v1/text-models/{model_id}:default",
		"/api/ops/admin/v1/text-models/{model_id}:test",
	} {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("expected text-model path %q", path)
		}
	}

	schemaContent, err := os.ReadFile("components/schemas/text.yaml")
	if err != nil {
		t.Fatalf("read text model schemas: %v", err)
	}
	var schemas struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					WriteOnly bool `yaml:"writeOnly"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemas); err != nil {
		t.Fatalf("unmarshal text model schemas: %v", err)
	}
	for _, name := range []string{"TextModelAccount", "TextModel", "TextModelConnectionTest", "PromptOptimizationEstimate", "PromptOptimizationResult"} {
		if _, ok := schemas.Components.Schemas[name]; !ok {
			t.Fatalf("expected text schema %q", name)
		}
	}
	writeRequest := schemas.Components.Schemas["TextModelAccountWriteRequest"]
	if !writeRequest.Properties["secrets"].WriteOnly {
		t.Fatal("TextModelAccountWriteRequest.secrets must be writeOnly")
	}
}

func TestOpenAPISpecPlacesProgressOnImageTask(t *testing.T) {
	content, err := os.ReadFile("components/schemas/common.yaml")
	if err != nil {
		t.Fatalf("read common schemas: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal common schemas: %v", err)
	}
	imageTask := doc.Components.Schemas["ImageTask"]
	for _, field := range []string{"progress_stage", "progress_message"} {
		if _, ok := imageTask.Properties[field]; !ok {
			t.Fatalf("ImageTask must document %s", field)
		}
	}
	referenceAsset := doc.Components.Schemas["ReferenceAsset"]
	if _, ok := referenceAsset.Properties["progress_stage"]; ok {
		t.Fatal("ReferenceAsset must not contain image task progress fields")
	}
}

func TestOpenAPISpecDocumentsMediaURLExpirations(t *testing.T) {
	content, err := os.ReadFile("components/schemas/common.yaml")
	if err != nil {
		t.Fatalf("read common schemas: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Type   string `yaml:"type"`
					Format string `yaml:"format"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal common schemas: %v", err)
	}
	for _, schemaName := range []string{"ReferenceAsset", "ImageResult", "GalleryImage"} {
		schema := doc.Components.Schemas[schemaName]
		for _, field := range []string{"preview_expires_at", "download_expires_at"} {
			property, ok := schema.Properties[field]
			if !ok || property.Type != "string" || property.Format != "date-time" {
				t.Fatalf("%s.%s must document optional date-time expiry metadata", schemaName, field)
			}
		}
	}
}

func TestOpenAPISpecDocumentsPublicGalleryPromptBoundaryContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `yaml:"name"`
				In   string `yaml:"in"`
			} `yaml:"parameters"`
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	listOperation := doc.Paths["/api/open/image/v1/gallery/images"]["get"]
	listParams := map[string]bool{}
	for _, param := range listOperation.Parameters {
		listParams[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:sort", "query:query", "query:route_model_code", "query:task_type", "query:liked", "query:favorited"} {
		if !listParams[key] {
			t.Fatalf("expected public gallery list parameter %q", key)
		}
	}
	listRef := listOperation.Responses["200"].Content["application/json"].Schema.Ref
	if listRef != "./components/schemas/common.yaml#/components/schemas/PublicGalleryListResponse" {
		t.Fatalf("expected public gallery list to use PublicGalleryListResponse, got %q", listRef)
	}

	detailOperation := doc.Paths["/api/open/image/v1/gallery/images/{image_id}"]["get"]
	detailRef := detailOperation.Responses["200"].Content["application/json"].Schema.Ref
	if detailRef != "./components/schemas/common.yaml#/components/schemas/PublicGalleryDetailResponse" {
		t.Fatalf("expected public gallery detail to use PublicGalleryDetailResponse, got %q", detailRef)
	}
	if _, ok := detailOperation.Responses["401"]; !ok {
		t.Fatal("expected public gallery detail to document 401 login-required response")
	}

	schemaContent, err := os.ReadFile("components/schemas/common.yaml")
	if err != nil {
		t.Fatalf("read common schema: %v", err)
	}
	var schemaDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string `yaml:"required"`
				Properties map[string]struct {
					Ref         string `yaml:"$ref"`
					Type        string `yaml:"type"`
					Nullable    bool   `yaml:"nullable"`
					Description string `yaml:"description"`
					Enum        []any  `yaml:"enum"`
					Items       struct {
						Ref string `yaml:"$ref"`
					} `yaml:"items"`
					Properties map[string]struct {
						Ref   string `yaml:"$ref"`
						Items struct {
							Ref string `yaml:"$ref"`
						} `yaml:"items"`
					} `yaml:"properties"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemaDoc); err != nil {
		t.Fatalf("unmarshal common schema: %v", err)
	}

	listImage, ok := schemaDoc.Components.Schemas["PublicGalleryListImage"]
	if !ok {
		t.Fatal("expected PublicGalleryListImage schema")
	}
	if _, ok := listImage.Properties["prompt_excerpt"]; !ok {
		t.Fatal("expected PublicGalleryListImage.prompt_excerpt")
	}
	prompt := listImage.Properties["prompt"]
	if !prompt.Nullable {
		t.Fatal("expected PublicGalleryListImage.prompt to be nullable")
	}
	if len(prompt.Enum) != 1 || prompt.Enum[0] != nil {
		t.Fatalf("expected PublicGalleryListImage.prompt enum to contain only null, got %#v", prompt.Enum)
	}
	if _, ok := listImage.Properties["comment_count"]; ok {
		t.Fatal("PublicGalleryListImage must not expose comment_count because comments are not a product capability")
	}

	listResponse, ok := schemaDoc.Components.Schemas["PublicGalleryListResponse"]
	if !ok {
		t.Fatal("expected PublicGalleryListResponse schema")
	}
	itemsRef := listResponse.Properties["data"].Properties["items"].Items.Ref
	if itemsRef != "#/components/schemas/PublicGalleryListImage" {
		t.Fatalf("expected PublicGalleryListResponse items to reference PublicGalleryListImage, got %q", itemsRef)
	}
	detailResponse, ok := schemaDoc.Components.Schemas["PublicGalleryDetailResponse"]
	if !ok {
		t.Fatal("expected PublicGalleryDetailResponse schema")
	}
	if detailResponse.Properties["data"].Ref != "#/components/schemas/PublicGalleryDetailImage" {
		t.Fatalf("expected PublicGalleryDetailResponse data to reference PublicGalleryDetailImage, got %q", detailResponse.Properties["data"].Ref)
	}
	detailImage, ok := schemaDoc.Components.Schemas["PublicGalleryDetailImage"]
	if !ok {
		t.Fatal("expected PublicGalleryDetailImage schema")
	}
	if _, ok := detailImage.Properties["comment_count"]; ok {
		t.Fatal("PublicGalleryDetailImage must not expose comment_count because comments are not a product capability")
	}
	galleryPrompt := detailImage.Properties["prompt"]
	if !galleryPrompt.Nullable || galleryPrompt.Description == "" {
		t.Fatalf("expected PublicGalleryDetailImage.prompt to document nullable authenticated detail semantics, got %#v", galleryPrompt)
	}
}

func TestOpenAPISpecDocumentsAdminDashboardOperationsContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	dashboardRef := doc.Paths["/api/ops/admin/v1/metrics/dashboard"]["get"].Responses["200"].Content["application/json"].Schema.Ref
	if dashboardRef != "./components/schemas/admin.yaml#/components/schemas/AdminDashboardResponse" {
		t.Fatalf("expected admin dashboard to use AdminDashboardResponse, got %q", dashboardRef)
	}

	schemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	type schemaNode struct {
		Required             []string              `yaml:"required"`
		Properties           map[string]schemaNode `yaml:"properties"`
		Type                 string                `yaml:"type"`
		Ref                  string                `yaml:"$ref"`
		AdditionalProperties any                   `yaml:"additionalProperties"`
		Items                struct {
			Ref string `yaml:"$ref"`
		} `yaml:"items"`
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]schemaNode `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}

	for _, name := range []string{"AdminDashboardResponse", "AdminDashboard", "AdminDashboardOperations", "AdminMetric", "AdminProviderHealth", "AdminDashboardQueueItem"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin dashboard schema %q", name)
		}
	}
	response := schemasDoc.Components.Schemas["AdminDashboardResponse"]
	if response.Properties["data"].Ref != "#/components/schemas/AdminDashboard" {
		t.Fatalf("expected AdminDashboardResponse.data to reference AdminDashboard, got %#v", response.Properties["data"])
	}
	dashboard := schemasDoc.Components.Schemas["AdminDashboard"]
	for _, field := range []string{"operations", "metrics", "providers", "queue", "audit"} {
		if _, ok := dashboard.Properties[field]; !ok {
			t.Fatalf("expected AdminDashboard to document %q", field)
		}
	}
	if dashboard.Properties["operations"].Ref != "#/components/schemas/AdminDashboardOperations" {
		t.Fatalf("expected AdminDashboard.operations ref, got %#v", dashboard.Properties["operations"])
	}
	if dashboard.Properties["metrics"].Items.Ref != "#/components/schemas/AdminMetric" {
		t.Fatalf("expected AdminDashboard.metrics to reference AdminMetric, got %#v", dashboard.Properties["metrics"])
	}
	if dashboard.Properties["providers"].Items.Ref != "#/components/schemas/AdminProviderHealth" {
		t.Fatalf("expected AdminDashboard.providers to reference AdminProviderHealth, got %#v", dashboard.Properties["providers"])
	}
	if dashboard.Properties["queue"].Items.Ref != "#/components/schemas/AdminDashboardQueueItem" {
		t.Fatalf("expected AdminDashboard.queue to reference AdminDashboardQueueItem, got %#v", dashboard.Properties["queue"])
	}
	if dashboard.Properties["audit"].Items.Ref != "#/components/schemas/AdminAuditLog" {
		t.Fatalf("expected AdminDashboard.audit to reference AdminAuditLog, got %#v", dashboard.Properties["audit"])
	}

	operations := schemasDoc.Components.Schemas["AdminDashboardOperations"]
	required := map[string]bool{}
	for _, field := range operations.Required {
		required[field] = true
	}
	for _, field := range []string{"today_order_count", "payment_success_rate", "failed_webhook_count", "refund_compensation_failed_count", "mock_enabled", "signup_trial_granted_user_count", "trial_expiring_user_count", "preflight_failure_count", "preflight_failures_by_error_code", "public_gallery_list_views", "public_gallery_detail_login_blocks", "enabled_payment_methods", "generated_at"} {
		if !required[field] {
			t.Fatalf("expected AdminDashboardOperations to require %q", field)
		}
		if _, ok := operations.Properties[field]; !ok {
			t.Fatalf("expected AdminDashboardOperations to document %q", field)
		}
	}
	additionalProperties, ok := operations.Properties["preflight_failures_by_error_code"].AdditionalProperties.(map[string]any)
	if !ok || additionalProperties["type"] != "integer" {
		t.Fatalf("expected preflight_failures_by_error_code to be integer map, got %#v", operations.Properties["preflight_failures_by_error_code"])
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
		"/api/ops/admin/v1/model-accounts":                "get",
		"/api/ops/admin/v1/model-accounts/{account_id}":   "put",
		"/api/ops/admin/v1/route-models":                  "get",
		"/api/ops/admin/v1/route-models/{route_model_id}": "put",
	} {
		operation := doc.Paths[path][method]
		if len(operation.Tags) != 1 || operation.Tags[0] != "Admin Model Routing" {
			t.Fatalf("expected %s %s to use Admin Model Routing tag, got %#v", method, path, operation.Tags)
		}
	}

	providerListParams := map[string]bool{}
	for _, param := range doc.Paths["/api/ops/admin/v1/model-accounts"]["get"].Parameters {
		providerListParams[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:adapter_type", "query:auth_type", "query:status"} {
		if !providerListParams[key] {
			t.Fatalf("expected model provider list parameter %q", key)
		}
	}
	routeListParams := map[string]bool{}
	for _, param := range doc.Paths["/api/ops/admin/v1/route-models"]["get"].Parameters {
		routeListParams[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:visibility", "query:enabled"} {
		if !routeListParams[key] {
			t.Fatalf("expected model route list parameter %q", key)
		}
	}

	for path, methods := range map[string][]string{
		"/api/ops/admin/v1/model-accounts":                {"post"},
		"/api/ops/admin/v1/model-accounts/{account_id}":   {"put"},
		"/api/ops/admin/v1/route-models":                  {"post"},
		"/api/ops/admin/v1/route-models/{route_model_id}": {"put"},
	} {
		for _, method := range methods {
			if !doc.Paths[path][method].RequestBody.Required {
				t.Fatalf("expected %s %s request body to be required", method, path)
			}
		}
	}
	for path, method := range map[string]string{
		"/api/ops/admin/v1/model-accounts/{account_id}":   "delete",
		"/api/ops/admin/v1/route-models/{route_model_id}": "delete",
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
	for _, name := range []string{"AdminModelAccount", "AdminModelAccountWriteRequest", "AdminModelAccountResponse", "AdminModelAccountListResponse", "AdminRouteModel", "AdminRouteModelWriteRequest", "AdminRouteModelResponse", "AdminRouteModelListResponse"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin model routing schema %q", name)
		}
	}
	for schemaName, requiredFields := range map[string][]string{
		"AdminModelAccountWriteRequest": {"name", "adapter_type", "auth_type", "base_url"},
		"AdminRouteModelWriteRequest":   {"code", "name", "visibility"},
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
}

func TestOpenAPISpecDocumentsAdminModelAccountCapabilitiesContract(t *testing.T) {
	type schemaNode struct {
		Type       string                `yaml:"type"`
		Ref        string                `yaml:"$ref"`
		Required   []string              `yaml:"required"`
		Properties map[string]schemaNode `yaml:"properties"`
		Items      *schemaNode           `yaml:"items"`
		Default    any                   `yaml:"default"`
		Minimum    *int                  `yaml:"minimum"`
	}
	type operation struct {
		RequestBody struct {
			Content map[string]struct {
				Schema schemaNode `yaml:"schema"`
			} `yaml:"content"`
		} `yaml:"requestBody"`
		Responses map[string]struct {
			Content map[string]struct {
				Schema schemaNode `yaml:"schema"`
			} `yaml:"content"`
		} `yaml:"responses"`
	}

	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]operation `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	const (
		collectionPath = "/api/ops/admin/v1/model-accounts/{account_id}/models"
		detailPath     = collectionPath + "/{model_id}"
		writeRef       = "./components/schemas/admin.yaml#/components/schemas/AdminModelAccountModelWriteRequest"
		responseRef    = "./components/schemas/admin.yaml#/components/schemas/AdminModelAccountModelResponse"
		listRef        = "./components/schemas/admin.yaml#/components/schemas/AdminModelAccountModelListResponse"
	)
	if got := doc.Paths[collectionPath]["get"].Responses["200"].Content["application/json"].Schema.Ref; got != listRef {
		t.Fatalf("expected account model list response ref %q, got %q", listRef, got)
	}
	if got := doc.Paths[collectionPath]["post"].RequestBody.Content["application/json"].Schema.Ref; got != writeRef {
		t.Fatalf("expected account model create request ref %q, got %q", writeRef, got)
	}
	if got := doc.Paths[collectionPath]["post"].Responses["201"].Content["application/json"].Schema.Ref; got != responseRef {
		t.Fatalf("expected account model create response ref %q, got %q", responseRef, got)
	}
	if got := doc.Paths[detailPath]["put"].RequestBody.Content["application/json"].Schema.Ref; got != writeRef {
		t.Fatalf("expected account model update request ref %q, got %q", writeRef, got)
	}
	if got := doc.Paths[detailPath]["put"].Responses["200"].Content["application/json"].Schema.Ref; got != responseRef {
		t.Fatalf("expected account model update response ref %q, got %q", responseRef, got)
	}
	if _, ok := doc.Paths[detailPath]["delete"].Responses["204"]; !ok {
		t.Fatal("expected account model delete to document 204 response")
	}

	schemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]schemaNode `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}

	for _, name := range []string{"AdminModelAccountModel", "AdminModelAccountModelWriteRequest", "AdminModelAccountModelResponse", "AdminModelAccountModelListResponse"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin account model schema %q", name)
		}
	}
	for _, schemaName := range []string{"AdminModelAccountModel", "AdminModelAccountModelWriteRequest"} {
		schema := schemasDoc.Components.Schemas[schemaName]
		for _, field := range []string{"supported_ratios", "max_image_count", "max_reference_image_count"} {
			if _, ok := schema.Properties[field]; !ok {
				t.Fatalf("expected %s to document %q", schemaName, field)
			}
		}
	}

	write := schemasDoc.Components.Schemas["AdminModelAccountModelWriteRequest"]
	if field := write.Properties["supported_ratios"]; field.Type != "array" || field.Items == nil || field.Items.Type != "string" {
		t.Fatalf("expected supported_ratios to be a string array, got %#v", field)
	}
	if field := write.Properties["max_image_count"]; field.Type != "integer" || field.Minimum == nil || *field.Minimum != 1 || field.Default != 1 {
		t.Fatalf("expected max_image_count minimum/default 1, got %#v", field)
	}
	if field := write.Properties["max_reference_image_count"]; field.Type != "integer" || field.Minimum == nil || *field.Minimum != 0 || field.Default != 0 {
		t.Fatalf("expected max_reference_image_count minimum/default 0, got %#v", field)
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

	systemListParams := map[string]bool{}
	for _, param := range doc.Paths["/api/ops/admin/v1/admin-users"]["get"].Parameters {
		systemListParams[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:query", "query:role", "query:status"} {
		if !systemListParams[key] {
			t.Fatalf("expected system admin user list parameter %q", key)
		}
	}
	if !doc.Paths["/api/ops/admin/v1/admin-users"]["post"].RequestBody.Required {
		t.Fatal("expected system admin create body to be required")
	}
	if !doc.Paths["/api/ops/admin/v1/admin-users/{admin_id}"]["put"].RequestBody.Required {
		t.Fatal("expected system admin update body to be required")
	}
	if !doc.Paths["/api/ops/admin/v1/admin-users/{admin_id}/reset-password"]["post"].RequestBody.Required {
		t.Fatal("expected system admin reset-password body to be required")
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
	for _, name := range []string{"SystemAdminUser", "SystemAdminUserListResponse", "SystemAdminUserCreateRequest", "SystemAdminUserUpdateRequest", "SystemAdminPasswordResetRequest", "SystemAdminUserResponse"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected system admin schema %q", name)
		}
	}
	for _, name := range []string{"AdminRedeemCode", "AdminCreateRedeemCodeRequest", "AdminBatchCreateRedeemCodesRequest", "AdminExportRedeemCodesRequest", "AdminExportRedeemCodesResponse", "AdminRedeemCodeResponse", "AdminRedeemCodeListResponse", "AdminRedeemCodeRedemptionsResponse"} {
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

func TestOpenAPISpecDocumentsTrialBalanceBucketAndLedgerContracts(t *testing.T) {
	schemaContent, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schema: %v", err)
	}
	type schemaNode struct {
		Required   []string              `yaml:"required"`
		Properties map[string]schemaNode `yaml:"properties"`
		Type       string                `yaml:"type"`
		Ref        string                `yaml:"$ref"`
		Items      struct {
			Ref string `yaml:"$ref"`
		} `yaml:"items"`
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]schemaNode `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal agent schema: %v", err)
	}

	for _, name := range []string{"SignupGrant", "BalanceBucket", "BalanceSummary", "PointLedgerEntry", "SessionResponse"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected agent schema %q", name)
		}
	}

	sessionData := schemasDoc.Components.Schemas["SessionResponse"].Properties["data"]
	if sessionData.Type != "object" {
		t.Fatalf("expected SessionResponse.data object, got %#v", sessionData)
	}
	if sessionData.Properties["signup_grant"].Ref != "#/components/schemas/SignupGrant" {
		t.Fatalf("expected SessionResponse.data.signup_grant to reference SignupGrant, got %#v", sessionData.Properties["signup_grant"])
	}
	if _, ok := schemasDoc.Components.Schemas["SignupGrant"]; !ok {
		t.Fatal("expected SignupGrant schema")
	}

	balance := schemasDoc.Components.Schemas["BalanceSummary"]
	for _, field := range []string{"available_points", "frozen_points", "trial_points", "subscription_points", "recharge_points", "buckets", "next_expiring_grant"} {
		if _, ok := balance.Properties[field]; !ok {
			t.Fatalf("expected BalanceSummary to document %q", field)
		}
	}
	if balance.Properties["buckets"].Type != "array" || balance.Properties["buckets"].Items.Ref != "#/components/schemas/BalanceBucket" {
		t.Fatalf("expected BalanceSummary.buckets to reference BalanceBucket, got %#v", balance.Properties["buckets"])
	}

	bucket := schemasDoc.Components.Schemas["BalanceBucket"]
	for _, field := range []string{"bucket", "label", "available_points", "frozen_points", "expires_at", "expire_warning"} {
		if _, ok := bucket.Properties[field]; !ok {
			t.Fatalf("expected BalanceBucket to document %q", field)
		}
	}

	signupGrant := schemasDoc.Components.Schemas["SignupGrant"]
	for _, field := range []string{"granted", "balance"} {
		if _, ok := signupGrant.Properties[field]; !ok {
			t.Fatalf("expected SignupGrant to document %q", field)
		}
	}
	if signupGrant.Properties["balance"].Ref != "#/components/schemas/BalanceSummary" {
		t.Fatalf("expected SignupGrant.balance to reference BalanceSummary, got %#v", signupGrant.Properties["balance"])
	}

	ledger := schemasDoc.Components.Schemas["PointLedgerEntry"]
	for _, field := range []string{"balance_bucket", "bucket_type", "source_type", "source_id", "bucket_balance_after", "expires_at"} {
		if _, ok := ledger.Properties[field]; !ok {
			t.Fatalf("expected PointLedgerEntry to document %q", field)
		}
	}
}

func TestOpenAPISpecDocumentsAdminPermissionContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Tags []struct {
			Name string `yaml:"name"`
		} `yaml:"tags"`
		Paths map[string]map[string]struct {
			Tags      []string       `yaml:"tags"`
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	seenTag := false
	for _, tag := range doc.Tags {
		if tag.Name == "Admin Auth" {
			seenTag = true
			break
		}
	}
	if !seenTag {
		t.Fatal("expected Admin Auth tag")
	}
	login := doc.Paths["/api/ops/admin/v1/auth/login"]["post"]
	if len(login.Tags) != 1 || login.Tags[0] != "Admin Auth" {
		t.Fatalf("expected admin login to use Admin Auth tag, got %#v", login.Tags)
	}
	if _, ok := login.Responses["200"]; !ok {
		t.Fatal("expected admin login to document 200 response")
	}

	schemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string `yaml:"required"`
				Enum       []string `yaml:"enum"`
				Properties map[string]struct {
					Ref   string `yaml:"$ref"`
					Type  string `yaml:"type"`
					Items struct {
						Ref string `yaml:"$ref"`
					} `yaml:"items"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}
	for _, name := range []string{"AdminPermission", "AdminRole", "AdminLoginRequest", "AdminLoginResponse", "AdminSession"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin permission schema %q", name)
		}
	}
	permissionEnum := map[string]bool{}
	for _, permission := range schemasDoc.Components.Schemas["AdminPermission"].Enum {
		permissionEnum[permission] = true
	}
	for _, permission := range []string{"read:all", "manage:admins", "manage:users", "manage:billing", "manage:cashier", "manage:models", "manage:reviews", "manage:config", "manage:dangerous_config", "view:audit"} {
		if !permissionEnum[permission] {
			t.Fatalf("expected AdminPermission enum to include %q", permission)
		}
	}
	session := schemasDoc.Components.Schemas["AdminSession"]
	for _, field := range []string{"access_token", "expires_in_seconds", "admin_id", "email", "role", "permissions"} {
		seen := false
		for _, required := range session.Required {
			if required == field {
				seen = true
				break
			}
		}
		if !seen {
			t.Fatalf("expected AdminSession to require %q", field)
		}
	}
	if session.Properties["permissions"].Type != "array" || session.Properties["permissions"].Items.Ref != "#/components/schemas/AdminPermission" {
		t.Fatalf("expected AdminSession.permissions to reference AdminPermission, got %#v", session.Properties["permissions"])
	}
	if session.Properties["role"].Ref != "#/components/schemas/AdminRole" {
		t.Fatalf("expected AdminSession.role to reference AdminRole, got %#v", session.Properties["role"])
	}
}

func TestOpenAPISpecDocumentsAdminCallRecordOperationsContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Tags []struct {
			Name string `yaml:"name"`
		} `yaml:"tags"`
		Paths map[string]map[string]struct {
			Tags       []string              `yaml:"tags"`
			Security   []map[string][]string `yaml:"security"`
			Parameters []struct {
				Name string `yaml:"name"`
				In   string `yaml:"in"`
			} `yaml:"parameters"`
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	seenTag := false
	for _, tag := range doc.Tags {
		if tag.Name == "Admin Call Records" {
			seenTag = true
			break
		}
	}
	if !seenTag {
		t.Fatal("expected Admin Call Records tag")
	}

	operation := doc.Paths["/api/ops/admin/v1/call-records"]["get"]
	if len(operation.Tags) != 1 || operation.Tags[0] != "Admin Call Records" {
		t.Fatalf("expected admin call records to use Admin Call Records tag, got %#v", operation.Tags)
	}
	if len(operation.Security) != 1 || operation.Security[0]["bearerAuth"] == nil {
		t.Fatalf("expected admin call records to require bearer auth, got %#v", operation.Security)
	}
	if _, ok := operation.Responses["200"]; !ok {
		t.Fatal("expected admin call records to document 200 response")
	}

	params := map[string]bool{}
	for _, param := range operation.Parameters {
		params[param.In+":"+param.Name] = true
	}
	for _, key := range []string{"query:page", "query:page_size", "query:status", "query:error_code", "query:source_channel", "query:provider", "query:user_id", "query:task_id", "query:created_from", "query:created_to"} {
		if !params[key] {
			t.Fatalf("expected admin call records parameter %q", key)
		}
	}

	schemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Ref   string `yaml:"$ref"`
					Type  string `yaml:"type"`
					Items struct {
						Ref string `yaml:"$ref"`
					} `yaml:"items"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}
	callRecord, ok := schemasDoc.Components.Schemas["AdminCallRecord"]
	if !ok {
		t.Fatal("expected AdminCallRecord schema")
	}
	for _, field := range []string{"task_id", "user_id", "api_key_id", "source_channel", "task_type", "status", "provider", "account_model_id", "model_account_id", "upstream_model_code", "abstract_model", "quality", "estimated_points", "actual_points", "provider_cost", "gross_margin", "fallback_count", "route_snapshot_version", "error_code", "error_message", "error_detail", "attempt_count", "attempts", "started_at", "finished_at"} {
		if _, ok := callRecord.Properties[field]; !ok {
			t.Fatalf("expected AdminCallRecord to document %q", field)
		}
	}
	if _, ok := schemasDoc.Components.Schemas["AdminCallRecordAttempt"]; !ok {
		t.Fatal("expected AdminCallRecordAttempt schema")
	}
	listResponse := schemasDoc.Components.Schemas["AdminCallRecordListResponse"]
	data := listResponse.Properties["data"]
	if data.Type != "object" {
		t.Fatalf("expected AdminCallRecordListResponse.data object, got %#v", data)
	}
}

func TestOpenAPISpecDocumentsAdminSecurityConfigContracts(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Tags []struct {
			Name string `yaml:"name"`
		} `yaml:"tags"`
		Paths map[string]map[string]struct {
			Tags        []string              `yaml:"tags"`
			OperationID string                `yaml:"operationId"`
			Security    []map[string][]string `yaml:"security"`
			RequestBody struct {
				Required bool `yaml:"required"`
			} `yaml:"requestBody"`
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	seenTag := false
	for _, tag := range doc.Tags {
		if tag.Name == "Admin Security Config" {
			seenTag = true
			break
		}
	}
	if !seenTag {
		t.Fatal("expected Admin Security Config tag")
	}

	for path, method := range map[string]string{
		"/api/ops/admin/v1/security/smtp":      "get",
		"/api/ops/admin/v1/security/smtp/test": "post",
	} {
		operation := doc.Paths[path][method]
		if len(operation.Tags) != 1 || operation.Tags[0] != "Admin Security Config" {
			t.Fatalf("expected %s %s to use Admin Security Config tag, got %#v", method, path, operation.Tags)
		}
		if len(operation.Security) != 1 || operation.Security[0]["bearerAuth"] == nil {
			t.Fatalf("expected %s %s to require bearer auth, got %#v", method, path, operation.Security)
		}
		if _, ok := operation.Responses["200"]; !ok {
			t.Fatalf("expected %s %s to document 200 response", method, path)
		}
	}
	putSMTP := doc.Paths["/api/ops/admin/v1/security/smtp"]["put"]
	if len(putSMTP.Tags) != 1 || putSMTP.Tags[0] != "Admin Security Config" {
		t.Fatalf("expected put smtp to use Admin Security Config tag, got %#v", putSMTP.Tags)
	}
	if len(putSMTP.Security) != 1 || putSMTP.Security[0]["bearerAuth"] == nil {
		t.Fatalf("expected put smtp to require bearer auth, got %#v", putSMTP.Security)
	}
	if !putSMTP.RequestBody.Required {
		t.Fatal("expected put smtp request body to be required")
	}

	schemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	type schemaProperty struct {
		Ref        string                    `yaml:"$ref"`
		Type       string                    `yaml:"type"`
		Properties map[string]schemaProperty `yaml:"properties"`
		Items      struct {
			Type string `yaml:"type"`
		} `yaml:"items"`
	}
	var schemasDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string                  `yaml:"required"`
				Properties map[string]schemaProperty `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &schemasDoc); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}
	for _, name := range []string{"SecretStatus", "AdminSecuritySMTPConfig", "AdminSecuritySMTPConfigWriteRequest", "AdminSecuritySMTPConfigResponse", "AdminSecuritySMTPTestRequest", "AdminSecuritySMTPTestResponse"} {
		if _, ok := schemasDoc.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin security schema %q", name)
		}
	}
	secretStatus := schemasDoc.Components.Schemas["SecretStatus"]
	for _, field := range []string{"has_secret", "fingerprint", "updated_at", "secret_fields"} {
		if _, ok := secretStatus.Properties[field]; !ok {
			t.Fatalf("expected SecretStatus to document %q", field)
		}
	}
	smtp := schemasDoc.Components.Schemas["AdminSecuritySMTPConfig"]
	if smtp.Properties["secret_status"].Ref != "#/components/schemas/SecretStatus" {
		t.Fatalf("expected smtp secret_status to reference SecretStatus, got %#v", smtp.Properties["secret_status"])
	}
	smtpWrite := schemasDoc.Components.Schemas["AdminSecuritySMTPConfigWriteRequest"]
	if _, ok := smtpWrite.Properties["secrets"]; !ok {
		t.Fatal("expected smtp write request to document secrets")
	}
	if _, ok := smtpWrite.Properties["clear_secrets"]; !ok {
		t.Fatal("expected smtp write request to document clear_secrets")
	}
}

func TestOpenAPISpecDocumentsCashierAndReadinessContracts(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var doc struct {
		Tags []struct {
			Name string `yaml:"name"`
		} `yaml:"tags"`
		Paths map[string]map[string]struct {
			Tags        []string              `yaml:"tags"`
			OperationID string                `yaml:"operationId"`
			Security    []map[string][]string `yaml:"security"`
			Parameters  []struct {
				Name string `yaml:"name"`
				In   string `yaml:"in"`
				Ref  string `yaml:"$ref"`
			} `yaml:"parameters"`
			RequestBody struct {
				Required bool `yaml:"required"`
			} `yaml:"requestBody"`
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	for _, expectedTag := range []string{"Agent Cashier", "Admin Readiness", "Admin Cashier"} {
		seen := false
		for _, tag := range doc.Tags {
			if tag.Name == expectedTag {
				seen = true
				break
			}
		}
		if !seen {
			t.Fatalf("expected %s tag", expectedTag)
		}
	}

	for path, method := range map[string]string{
		"/api/agent/cashier/v1/options":                    "get",
		"/api/agent/cashier/v1/orders":                     "post",
		"/api/agent/cashier/v1/orders/{order_id}":          "get",
		"/api/agent/cashier/v1/orders/{order_id}/mock-pay": "post",
	} {
		operation := doc.Paths[path][method]
		if len(operation.Tags) != 1 || operation.Tags[0] != "Agent Cashier" {
			t.Fatalf("expected %s %s to use Agent Cashier tag, got %#v", method, path, operation.Tags)
		}
		if len(operation.Security) != 1 || operation.Security[0]["bearerAuth"] == nil {
			t.Fatalf("expected %s %s to require bearer auth, got %#v", method, path, operation.Security)
		}
		if _, ok := operation.Responses["200"]; method != "post" && !ok {
			t.Fatalf("expected %s %s to document 200 response", method, path)
		}
	}
	if !doc.Paths["/api/agent/cashier/v1/orders"]["post"].RequestBody.Required {
		t.Fatal("expected cashier order create body to be required")
	}
	seenCashierIDKey := false
	for _, param := range doc.Paths["/api/agent/cashier/v1/orders"]["post"].Parameters {
		if param.Ref == "./components/parameters/common.yaml#/components/parameters/IdempotencyKey" || (param.In == "header" && param.Name == "Idempotency-Key") {
			seenCashierIDKey = true
		}
	}
	if !seenCashierIDKey {
		t.Fatal("expected cashier order create to document Idempotency-Key header")
	}

	readiness := doc.Paths["/api/ops/admin/v1/readiness"]["get"]
	if len(readiness.Tags) != 1 || readiness.Tags[0] != "Admin Readiness" {
		t.Fatalf("expected admin readiness to use Admin Readiness tag, got %#v", readiness.Tags)
	}
	if len(readiness.Security) != 1 || readiness.Security[0]["bearerAuth"] == nil {
		t.Fatalf("expected admin readiness to require bearer auth, got %#v", readiness.Security)
	}

	for path, method := range map[string]string{
		"/api/ops/admin/v1/cashier/overview":                     "get",
		"/api/ops/admin/v1/cashier/plans":                        "post",
		"/api/ops/admin/v1/cashier/custom-amount-config":         "put",
		"/api/ops/admin/v1/cashier/visible-methods":              "put",
		"/api/ops/admin/v1/cashier/provider-instances":           "post",
		"/api/ops/admin/v1/cashier/orders":                       "get",
		"/api/ops/admin/v1/cashier/orders/{order_id}/complete":   "post",
		"/api/ops/admin/v1/cashier/orders/{order_id}/close":      "post",
		"/api/ops/admin/v1/cashier/orders/{order_id}/refund":     "post",
		"/api/ops/admin/v1/cashier/orders/{order_id}/chargeback": "post",
		"/api/ops/admin/v1/cashier/orders/{order_id}/sync":       "post",
		"/api/ops/admin/v1/cashier/webhook-events":               "get",
	} {
		operation := doc.Paths[path][method]
		if len(operation.Tags) != 1 || operation.Tags[0] != "Admin Cashier" {
			t.Fatalf("expected %s %s to use Admin Cashier tag, got %#v", method, path, operation.Tags)
		}
		if len(operation.Security) != 1 || operation.Security[0]["bearerAuth"] == nil {
			t.Fatalf("expected %s %s to require bearer auth, got %#v", method, path, operation.Security)
		}
		if (method == "post" || method == "put") && path != "/api/ops/admin/v1/cashier/orders/{order_id}/sync" {
			if !operation.RequestBody.Required {
				t.Fatalf("expected %s %s request body to be required", method, path)
			}
		}
	}
	orderSync := doc.Paths["/api/ops/admin/v1/cashier/orders/{order_id}/sync"]["post"]
	if orderSync.OperationID != "postAdminCashierOrderSync" {
		t.Fatalf("expected cashier order sync operationId postAdminCashierOrderSync, got %q", orderSync.OperationID)
	}
	seenOrderID := false
	for _, param := range orderSync.Parameters {
		if param.In == "path" && param.Name == "order_id" {
			seenOrderID = true
			break
		}
	}
	if !seenOrderID {
		t.Fatal("expected cashier order sync to document order_id path parameter")
	}
	syncResponseRef := orderSync.Responses["200"].Content["application/json"].Schema.Ref
	if syncResponseRef != "./components/schemas/admin.yaml#/components/schemas/AdminCashierOrderSyncResponse" {
		t.Fatalf("expected cashier order sync 200 response to reference AdminCashierOrderSyncResponse, got %q", syncResponseRef)
	}
	orderChargeback := doc.Paths["/api/ops/admin/v1/cashier/orders/{order_id}/chargeback"]["post"]
	if orderChargeback.OperationID != "postAdminCashierOrderChargeback" {
		t.Fatalf("expected cashier order chargeback operationId postAdminCashierOrderChargeback, got %q", orderChargeback.OperationID)
	}
	seenChargebackIDKey := false
	for _, param := range orderChargeback.Parameters {
		if param.Ref == "./components/parameters/common.yaml#/components/parameters/RequiredIdempotencyKey" || (param.In == "header" && param.Name == "Idempotency-Key") {
			seenChargebackIDKey = true
			break
		}
	}
	if !seenChargebackIDKey {
		t.Fatal("expected cashier order chargeback to document required Idempotency-Key header")
	}
	chargebackResponseRef := orderChargeback.Responses["200"].Content["application/json"].Schema.Ref
	if chargebackResponseRef != "./components/schemas/admin.yaml#/components/schemas/AdminCashierChargebackResponse" {
		t.Fatalf("expected cashier order chargeback 200 response to reference AdminCashierChargebackResponse, got %q", chargebackResponseRef)
	}
	webhookRetry := doc.Paths["/api/ops/admin/v1/cashier/webhook-events/{event_id}/retry"]["post"]
	if len(webhookRetry.Tags) != 1 || webhookRetry.Tags[0] != "Admin Cashier" {
		t.Fatalf("expected webhook retry to use Admin Cashier tag, got %#v", webhookRetry.Tags)
	}
	if len(webhookRetry.Security) != 1 || webhookRetry.Security[0]["bearerAuth"] == nil {
		t.Fatalf("expected webhook retry to require bearer auth, got %#v", webhookRetry.Security)
	}
	seenEventID := false
	for _, param := range webhookRetry.Parameters {
		if param.In == "path" && param.Name == "event_id" {
			seenEventID = true
			break
		}
	}
	if !seenEventID {
		t.Fatal("expected webhook retry to document event_id path parameter")
	}
	if _, ok := webhookRetry.Responses["200"]; !ok {
		t.Fatal("expected webhook retry to document 200 response")
	}

	schemaContent, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schema: %v", err)
	}
	var agentSchemas struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string `yaml:"required"`
				Properties map[string]struct {
					Type string   `yaml:"type"`
					Ref  string   `yaml:"$ref"`
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(schemaContent, &agentSchemas); err != nil {
		t.Fatalf("unmarshal agent schema: %v", err)
	}
	for _, name := range []string{"CashierOptionsResponse", "CashierOrderCreateRequest", "CashierOrderResponse", "PaymentDisplay"} {
		if _, ok := agentSchemas.Components.Schemas[name]; !ok {
			t.Fatalf("expected agent cashier schema %q", name)
		}
	}
	createRequired := map[string]bool{}
	for _, field := range agentSchemas.Components.Schemas["CashierOrderCreateRequest"].Required {
		createRequired[field] = true
	}
	for _, field := range []string{"purchase_type", "visible_method"} {
		if !createRequired[field] {
			t.Fatalf("expected CashierOrderCreateRequest to require %q", field)
		}
	}
	paymentOrder := agentSchemas.Components.Schemas["PaymentOrder"]
	for _, field := range []string{"purchase_type", "visible_method", "provider_type", "provider_instance_id", "payment_display", "ledger_id", "completed_at", "refund_trade_no"} {
		if _, ok := paymentOrder.Properties[field]; !ok {
			t.Fatalf("expected PaymentOrder to document %q", field)
		}
	}
	paymentDisplay := agentSchemas.Components.Schemas["PaymentDisplay"]
	displayTypes := map[string]bool{}
	for _, value := range paymentDisplay.Properties["type"].Enum {
		displayTypes[value] = true
	}
	for _, value := range []string{"qr_code", "redirect", "form_html", "form", "jsapi", "mock", "none"} {
		if !displayTypes[value] {
			t.Fatalf("expected PaymentDisplay.type enum to document %q, got %#v", value, paymentDisplay.Properties["type"].Enum)
		}
	}

	adminSchemaContent, err := os.ReadFile("components/schemas/admin.yaml")
	if err != nil {
		t.Fatalf("read admin schema: %v", err)
	}
	type schemaProperty struct {
		Enum       []string                  `yaml:"enum"`
		Ref        string                    `yaml:"$ref"`
		Type       string                    `yaml:"type"`
		Required   []string                  `yaml:"required"`
		Properties map[string]schemaProperty `yaml:"properties"`
		Items      struct {
			Ref string `yaml:"$ref"`
		} `yaml:"items"`
	}
	var adminSchemas struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string                  `yaml:"required"`
				Properties map[string]schemaProperty `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(adminSchemaContent, &adminSchemas); err != nil {
		t.Fatalf("unmarshal admin schema: %v", err)
	}
	for _, name := range []string{"ReadinessReport", "ReadinessCheck", "AdminCashierOverview", "AdminCashierPlan", "AdminCashierPlanWriteRequest", "AdminCashierCustomAmountConfig", "AdminCashierProviderInstance", "AdminCashierProviderInstanceWriteRequest", "AdminCashierManualCompleteRequest", "AdminCashierRefundRequest", "AdminCashierChargebackRequest", "AdminCashierChargebackResponse", "AdminCashierOrderSyncResult", "AdminCashierOrderSyncResponse", "AdminPaymentWebhookEvent"} {
		if _, ok := adminSchemas.Components.Schemas[name]; !ok {
			t.Fatalf("expected admin cashier/readiness schema %q", name)
		}
	}
	for schemaName, requiredFields := range map[string][]string{
		"AdminCashierPlanWriteRequest":             {"plan_code", "plan_name", "plan_type", "purchase_enabled", "price_cny", "points"},
		"AdminCashierProviderInstanceWriteRequest": {"provider_type", "name", "supported_methods", "config"},
		"AdminCashierManualCompleteRequest":        {"trade_no"},
		"AdminCashierRefundRequest":                {"refund_trade_no"},
		"AdminCashierChargebackRequest":            {"charge_points", "reason"},
		"AdminCashierOrderSyncResult":              {"provider_type", "query_status", "paid", "completed", "synced_at"},
	} {
		required := map[string]bool{}
		for _, field := range adminSchemas.Components.Schemas[schemaName].Required {
			required[field] = true
		}
		for _, field := range requiredFields {
			if !required[field] {
				t.Fatalf("expected %s to require %q", schemaName, field)
			}
		}
	}
	refundRequest := adminSchemas.Components.Schemas["AdminCashierRefundRequest"]
	if _, ok := refundRequest.Properties["refund_amount_cny"]; !ok {
		t.Fatal("expected AdminCashierRefundRequest to document optional refund_amount_cny")
	}
	providerInstance := adminSchemas.Components.Schemas["AdminCashierProviderInstance"]
	if providerInstance.Properties["credentials_status"].Ref != "#/components/schemas/SecretStatus" {
		t.Fatalf("expected AdminCashierProviderInstance.credentials_status to reference SecretStatus, got %#v", providerInstance.Properties["credentials_status"])
	}
	providerWrite := adminSchemas.Components.Schemas["AdminCashierProviderInstanceWriteRequest"]
	for _, field := range []string{"secrets", "clear_secrets"} {
		if _, ok := providerWrite.Properties[field]; !ok {
			t.Fatalf("expected AdminCashierProviderInstanceWriteRequest to document %q", field)
		}
	}
	chargebackData := adminSchemas.Components.Schemas["AdminCashierChargebackResponse"].Properties["data"]
	if chargebackData.Type != "object" {
		t.Fatalf("expected AdminCashierChargebackResponse.data object, got %#v", chargebackData)
	}
	if chargebackData.Properties["order"].Ref != "./agent.yaml#/components/schemas/PaymentOrder" {
		t.Fatalf("expected AdminCashierChargebackResponse.data.order to reference PaymentOrder, got %#v", chargebackData.Properties["order"])
	}
	if chargebackData.Properties["balance"].Ref != "./agent.yaml#/components/schemas/BalanceSummary" {
		t.Fatalf("expected AdminCashierChargebackResponse.data.balance to reference BalanceSummary, got %#v", chargebackData.Properties["balance"])
	}
	orderSyncData := adminSchemas.Components.Schemas["AdminCashierOrderSyncResponse"].Properties["data"]
	if orderSyncData.Type != "object" {
		t.Fatalf("expected AdminCashierOrderSyncResponse.data object, got %#v", orderSyncData)
	}
	if orderSyncData.Properties["order"].Ref != "./agent.yaml#/components/schemas/PaymentOrder" {
		t.Fatalf("expected AdminCashierOrderSyncResponse.data.order to reference PaymentOrder, got %#v", orderSyncData.Properties["order"])
	}
	if orderSyncData.Properties["sync"].Ref != "#/components/schemas/AdminCashierOrderSyncResult" {
		t.Fatalf("expected AdminCashierOrderSyncResponse.data.sync to reference AdminCashierOrderSyncResult, got %#v", orderSyncData.Properties["sync"])
	}
	orderSyncStatusEnum := map[string]bool{}
	for _, value := range adminSchemas.Components.Schemas["AdminCashierOrderSyncResult"].Properties["query_status"].Enum {
		orderSyncStatusEnum[value] = true
	}
	for _, status := range []string{"pending", "paid", "closed", "failed", "refunded"} {
		if !orderSyncStatusEnum[status] {
			t.Fatalf("expected AdminCashierOrderSyncResult.query_status enum to include %q", status)
		}
	}
	for _, schemaName := range []string{"AdminCashierProviderInstance", "AdminCashierProviderInstanceWriteRequest"} {
		enumValues := map[string]bool{}
		for _, value := range adminSchemas.Components.Schemas[schemaName].Properties["provider_type"].Enum {
			enumValues[value] = true
		}
		for _, providerType := range []string{"jeepay_alipay", "jeepay_wxpay"} {
			if !enumValues[providerType] {
				t.Fatalf("expected %s provider_type enum to include %q", schemaName, providerType)
			}
		}
		if enumValues["jeepay_placeholder"] {
			t.Fatalf("expected %s provider_type enum to remove jeepay_placeholder", schemaName)
		}
	}
	webhookStatusEnum := map[string]bool{}
	for _, value := range adminSchemas.Components.Schemas["AdminPaymentWebhookEvent"].Properties["status"].Enum {
		webhookStatusEnum[value] = true
	}
	for _, status := range []string{"received", "verified", "processed", "failed"} {
		if !webhookStatusEnum[status] {
			t.Fatalf("expected AdminPaymentWebhookEvent.status enum to include %q", status)
		}
	}
	webhookListData := adminSchemas.Components.Schemas["AdminPaymentWebhookEventListResponse"].Properties["data"]
	if webhookListData.Type != "object" {
		t.Fatalf("expected AdminPaymentWebhookEventListResponse.data object, got %#v", webhookListData)
	}
	webhookResponse := adminSchemas.Components.Schemas["AdminPaymentWebhookEventResponse"].Properties["data"]
	if webhookResponse.Ref != "#/components/schemas/AdminPaymentWebhookEvent" {
		t.Fatalf("expected AdminPaymentWebhookEventResponse.data to reference AdminPaymentWebhookEvent, got %#v", webhookResponse)
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

func TestOpenAPISpecDocumentsCurrentImageTaskRequestContract(t *testing.T) {
	content, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schema: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string       `yaml:"required"`
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal agent schema: %v", err)
	}
	request := doc.Components.Schemas["CreateImageTaskRequest"]
	for _, field := range []string{
		"size_mode", "base_resolution", "quality", "output_format", "output_compression",
		"moderation", "aspect_ratio", "requested_size", "requested_output_image_count",
	} {
		if _, ok := request.Properties[field]; !ok {
			t.Errorf("CreateImageTaskRequest must document %s", field)
		}
	}
	for _, legacy := range []string{"requested_quality", "resolved_quality_bucket"} {
		if _, ok := request.Properties[legacy]; ok {
			t.Errorf("CreateImageTaskRequest must not expose legacy field %s", legacy)
		}
		for _, required := range request.Required {
			if required == legacy {
				t.Errorf("CreateImageTaskRequest must not require legacy field %s", legacy)
			}
		}
	}

	commonContent, err := os.ReadFile("components/schemas/common.yaml")
	if err != nil {
		t.Fatalf("read common schema: %v", err)
	}
	if err := yaml.Unmarshal(commonContent, &doc); err != nil {
		t.Fatalf("unmarshal common schema: %v", err)
	}
	imageTask := doc.Components.Schemas["ImageTask"]
	for _, field := range []string{"size_mode", "base_resolution", "quality", "output_format", "output_compression", "moderation", "aspect_ratio", "requested_size", "requested_output_image_count"} {
		if _, ok := imageTask.Properties[field]; !ok {
			t.Errorf("ImageTask must document %s", field)
		}
	}
	for _, legacy := range []string{"requested_quality", "resolved_quality_bucket"} {
		if _, ok := imageTask.Properties[legacy]; ok {
			t.Errorf("ImageTask must not expose legacy field %s", legacy)
		}
	}
}

func TestOpenAPISpecDocumentsPromptTemplateAndReferenceNamingContract(t *testing.T) {
	agentContent, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schema: %v", err)
	}
	type property struct {
		Ref         string `yaml:"$ref"`
		Deprecated  bool   `yaml:"deprecated"`
		Description string `yaml:"description"`
		Items       struct {
			Ref string `yaml:"$ref"`
		} `yaml:"items"`
	}
	var agentDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string            `yaml:"required"`
				Properties map[string]property `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(agentContent, &agentDoc); err != nil {
		t.Fatalf("unmarshal agent schema: %v", err)
	}
	for _, schemaName := range []string{"ReferenceAssetRenameRequest", "PromptReferenceBinding", "PromptVariableInput"} {
		if _, ok := agentDoc.Components.Schemas[schemaName]; !ok {
			t.Errorf("agent schema must document %s", schemaName)
		}
	}
	createTask := agentDoc.Components.Schemas["CreateImageTaskRequest"]
	if got := createTask.Properties["reference_bindings"].Items.Ref; got != "#/components/schemas/PromptReferenceBinding" {
		t.Errorf("reference_bindings must use PromptReferenceBinding, got %q", got)
	}
	if got := createTask.Properties["prompt_variables"].Items.Ref; got != "#/components/schemas/PromptVariableInput" {
		t.Errorf("prompt_variables must use PromptVariableInput, got %q", got)
	}
	legacyProject := agentDoc.Components.Schemas["ReferenceAssetsImportFromGalleryRequest"].Properties["project_id"]
	if !legacyProject.Deprecated || !strings.Contains(strings.ToLower(legacyProject.Description), "ignored") {
		t.Errorf("legacy import project_id must be documented as deprecated and ignored: %#v", legacyProject)
	}

	commonContent, err := os.ReadFile("components/schemas/common.yaml")
	if err != nil {
		t.Fatalf("read common schema: %v", err)
	}
	var commonDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string       `yaml:"required"`
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(commonContent, &commonDoc); err != nil {
		t.Fatalf("unmarshal common schema: %v", err)
	}
	referenceAsset := commonDoc.Components.Schemas["ReferenceAsset"]
	if _, ok := referenceAsset.Properties["name"]; !ok || !containsString(referenceAsset.Required, "name") {
		t.Errorf("ReferenceAsset.name must be documented and required")
	}

	rootContent, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var rootDoc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"requestBody"`
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(rootContent, &rootDoc); err != nil {
		t.Fatalf("unmarshal root spec: %v", err)
	}
	patch := rootDoc.Paths["/api/agent/image/v1/reference-assets/{asset_id}"]["patch"]
	if got := patch.RequestBody.Content["application/json"].Schema.Ref; got != "./components/schemas/agent.yaml#/components/schemas/ReferenceAssetRenameRequest" {
		t.Errorf("reference rename PATCH must use ReferenceAssetRenameRequest, got %q", got)
	}
	for _, status := range []string{"200", "400", "404", "409"} {
		if _, ok := patch.Responses[status]; !ok {
			t.Errorf("reference rename PATCH must document response %s", status)
		}
	}
}

func containsString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func TestOpenAPISpecDocumentsRouteModelSelectionContract(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var rootDoc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Ref string `yaml:"$ref"`
			} `yaml:"parameters"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &rootDoc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	for _, path := range []string{"/api/agent/billing/v1/estimate", "/api/open/image/v1/estimate"} {
		parameters := rootDoc.Paths[path]["get"].Parameters
		if len(parameters) < 2 || parameters[0].Ref != "./components/parameters/common.yaml#/components/parameters/RouteModelCode" {
			t.Fatalf("expected %s to prefer RouteModelCode, got %#v", path, parameters)
		}
		found := map[string]bool{}
		for _, parameter := range parameters {
			found[parameter.Ref] = true
		}
		if !found["./components/parameters/common.yaml#/components/parameters/AbstractModel"] {
			t.Fatalf("expected %s to retain deprecated AbstractModel compatibility", path)
		}
		for _, name := range []string{"SizeMode", "AspectRatio"} {
			ref := "./components/parameters/common.yaml#/components/parameters/" + name
			if !found[ref] {
				t.Errorf("expected %s to document %s", path, ref)
			}
		}
	}

	parameterContent, err := os.ReadFile("components/parameters/common.yaml")
	if err != nil {
		t.Fatalf("read common parameters: %v", err)
	}
	var parameterDoc struct {
		Components struct {
			Parameters map[string]struct {
				Name       string `yaml:"name"`
				Required   bool   `yaml:"required"`
				Deprecated bool   `yaml:"deprecated"`
				Schema     struct {
					Type      string   `yaml:"type"`
					MinLength int      `yaml:"minLength"`
					Enum      []string `yaml:"enum"`
				} `yaml:"schema"`
			} `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(parameterContent, &parameterDoc); err != nil {
		t.Fatalf("unmarshal common parameters: %v", err)
	}
	routeParameter := parameterDoc.Components.Parameters["RouteModelCode"]
	if routeParameter.Name != "route_model_code" || !routeParameter.Required || routeParameter.Schema.Type != "string" || routeParameter.Schema.MinLength != 1 {
		t.Fatalf("unexpected RouteModelCode parameter %#v", routeParameter)
	}
	legacyParameter := parameterDoc.Components.Parameters["AbstractModel"]
	if legacyParameter.Required || !legacyParameter.Deprecated {
		t.Fatalf("expected AbstractModel to be optional and deprecated, got %#v", legacyParameter)
	}
	sizeModeParameter := parameterDoc.Components.Parameters["SizeMode"]
	if sizeModeParameter.Name != "size_mode" || sizeModeParameter.Required || sizeModeParameter.Schema.Type != "string" || strings.Join(sizeModeParameter.Schema.Enum, ",") != "ratio,pixel" {
		t.Fatalf("unexpected SizeMode parameter %#v", sizeModeParameter)
	}
	aspectRatioParameter := parameterDoc.Components.Parameters["AspectRatio"]
	if aspectRatioParameter.Name != "aspect_ratio" || aspectRatioParameter.Required || aspectRatioParameter.Schema.Type != "string" {
		t.Fatalf("unexpected AspectRatio parameter %#v", aspectRatioParameter)
	}

	agentContent, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatalf("read agent schema: %v", err)
	}
	var agentDoc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string `yaml:"required"`
				Properties map[string]struct {
					Type       string `yaml:"type"`
					Deprecated bool   `yaml:"deprecated"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(agentContent, &agentDoc); err != nil {
		t.Fatalf("unmarshal agent schema: %v", err)
	}
	createTask := agentDoc.Components.Schemas["CreateImageTaskRequest"]
	required := map[string]bool{}
	for _, field := range createTask.Required {
		required[field] = true
	}
	if !required["route_model_code"] || required["abstract_model"] {
		t.Fatalf("expected route_model_code, not abstract_model, to be required: %#v", createTask.Required)
	}
	if createTask.Properties["route_model_code"].Type != "string" || !createTask.Properties["abstract_model"].Deprecated {
		t.Fatalf("unexpected task model properties %#v", createTask.Properties)
	}

	commonContent, err := os.ReadFile("components/schemas/common.yaml")
	if err != nil {
		t.Fatalf("read common schema: %v", err)
	}
	var commonDoc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Type       string `yaml:"type"`
					Deprecated bool   `yaml:"deprecated"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(commonContent, &commonDoc); err != nil {
		t.Fatalf("unmarshal common schema: %v", err)
	}
	imageTask := commonDoc.Components.Schemas["ImageTask"]
	if imageTask.Properties["route_model_code"].Type != "string" || !imageTask.Properties["abstract_model"].Deprecated {
		t.Fatalf("unexpected ImageTask model properties %#v", imageTask.Properties)
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

func TestOpenAPISpecDocumentsEncryptedClusterEnrollment(t *testing.T) {
	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]struct {
			Get struct {
				Responses map[string]any `yaml:"responses"`
			} `yaml:"get"`
			Post struct {
				Responses map[string]any `yaml:"responses"`
			} `yaml:"post"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/open/cluster/v1/challenges", "/api/open/cluster/v1/join"} {
		responses := doc.Paths[path].Post.Responses
		if _, ok := responses["201"]; !ok {
			t.Fatalf("%s does not document a 201 response: %#v", path, responses)
		}
		if _, ok := responses["501"]; ok {
			t.Fatalf("%s still documents the retired 501 response", path)
		}
	}
	if responses := doc.Paths["/api/ops/admin/v1/cluster/nodes"].Post.Responses; responses != nil {
		t.Fatalf("cluster nodes must not document a POST operation: %#v", responses)
	}
	if _, ok := doc.Paths["/api/ops/admin/v1/cluster/nodes"].Get.Responses["200"]; !ok {
		t.Fatal("cluster nodes does not document a 200 response")
	}

	schemaContent, err := os.ReadFile("components/schemas/cluster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(schemaContent)
	for _, required := range []string{
		"required: [protocol, token_id, node_id, node_public_key, application_version, runtime_schema_version]",
		"required: [protocol, challenge_id, proof]",
		"writeOnly: true",
		"X25519-HKDF-SHA256-XCHACHA20-POLY1305",
		"readOnly: true",
		"effective_health",
		"application_version_drift",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("cluster schema missing %q", required)
		}
	}
}
