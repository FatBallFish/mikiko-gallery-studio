package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	auth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	bill "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/service/assets"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type nativeE2EUsers struct{}

func (nativeE2EUsers) GetUserByID(id int64) (auth.User, bool) {
	return auth.User{ID: id, Status: "active", GroupCode: "basic", GroupCodes: []string{"basic"}, GroupMultiplier: "1.00000"}, id == 7 || id == 8
}

type nativeE2ERouting struct{ snapshot modelhub.ModelRoutingSnapshot }

func (r nativeE2ERouting) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return r.snapshot, nil
}

// All credentials, upstream responses, authentication and download signing in this
// test are fixtures. The production REST handler, StreamableHTTP transport,
// shared operations, quote validation, task leases, adapters and storage run for real.
func TestNativeImageRESTAndMCPFakeProviderLifecycle(t *testing.T) {
	for _, transport := range []string{"api_key_rest", "oauth_mcp"} {
		for _, tc := range []struct {
			name, adapter, model string
			op                   nativeimage.Operation
		}{
			{"qwen_generate", nativeimage.AdapterQwen, "qwen-image-3.0", nativeimage.OperationGenerate},
			{"qwen_edit", nativeimage.AdapterQwen, "qwen-image-3.0", nativeimage.OperationEdit},
			{"seedream_generate", nativeimage.AdapterSeedream, "doubao-seedream-5-0-pro-260628", nativeimage.OperationGenerate},
			{"seedream_edit", nativeimage.AdapterSeedream, "doubao-seedream-5-0-pro-260628", nativeimage.OperationEdit},
			{"seedream_layers", nativeimage.AdapterSeedream, "doubao-seedream-5-0-pro-260628", nativeimage.OperationLayerDecomposition},
		} {
			t.Run(transport+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				var pngBytes bytes.Buffer
				if err := png.Encode(&pngBytes, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
					t.Fatal(err)
				}
				encoded := base64.StdEncoding.EncodeToString(pngBytes.Bytes())
				var providerCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					providerCalls.Add(1)
					if r.Header.Get("Authorization") != "Bearer fake-native-key" {
						t.Error("fixture credential missing")
					}
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if payload["model"] != tc.model {
						t.Errorf("model lost: %#v", payload)
					}
					if tc.adapter == nativeimage.AdapterQwen {
						if r.URL.Path != "/api/v1/services/aigc/multimodal-generation/generation" {
							t.Errorf("Qwen path %s", r.URL.Path)
						}
						input, _ := payload["input"].(map[string]any)
						messages, _ := input["messages"].([]any)
						if len(messages) == 0 {
							t.Error("Qwen native messages missing")
						}
						fmt.Fprintf(w, `{"request_id":"fake-qwen","output":{"choices":[{"message":{"content":[{"image":"data:image/png;base64,%s"}]}}]}}`, encoded)
					} else {
						if r.URL.Path != "/api/v3/images/generations" {
							t.Errorf("Seedream path %s", r.URL.Path)
						}
						if tc.op != nativeimage.OperationGenerate && payload["image"] == nil {
							t.Error("edit reference lost")
						}
						data := []map[string]any{{"b64_json": encoded, "size": "1024x1024", "output_format": "png"}}
						if tc.op == nativeimage.OperationLayerDecomposition {
							if payload["layer_decomposition"] != true {
								t.Error("layer operation lost")
							}
							data[0]["z_index"] = 0
							data = append(data, map[string]any{"b64_json": encoded, "size": "1024x1024", "output_format": "png", "z_index": 1, "name": "fixture layer", "bounding_box": map[string]any{"absolute": []int{0, 0, 1024, 1024}, "normalized": []int{0, 0, 999, 999}}})
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
					}
				}))
				defer upstream.Close()
				cfg := config.Config{}
				cfg.Security.SecureConfigEncryptionKey = "fixture-checkpoint-key"
				cfg.GenerationLimits = config.GenerationLimitsConfig{MaxImageCount: 6, ReferenceImageMaxCount: 10, ReferenceImageMaxMB: 20, PromptMaxChars: 10000}
				cfg.Billing = config.BillingConfig{CNYPerPoint: "0.01", PointsScale: 5, UserGroupMultipliers: map[string]string{"basic": "1.00000"}, TaskMultipliers: map[string]string{"text_to_image": "1.00000", "image_edit": "1.00000"}}
				price, ok := nativeimage.PricingForModel(tc.model)
				if !ok {
					t.Fatal("missing pricing")
				}
				// Explicit fixture tariff: official layer base pricing is not confirmed.
				base := 0.3
				price.LayerBaseCNY = &base
				price.LayerPricingConfirmed = true
				inputPrice := fmt.Sprint(price.InputCNY)
				input := imagepricing.Input{FreeImages: price.FreeInputImages, PerImage: &inputPrice}
				outputTiers := make([]imagepricing.Tier, 0, len(price.OutputTiers))
				for _, tier := range price.OutputTiers {
					outputTiers = append(outputTiers, imagepricing.Tier{Name: tier.Name, MaxPixels: int64(tier.MaxPixels), Price: fmt.Sprint(tier.CNY)})
				}
				routePrice := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{
					{Operation: "generate", Mode: "per_image", Currency: "CNY", Version: "fixture-v1", Input: input, OutputTiers: outputTiers},
					{Operation: "edit", Mode: "per_image", Currency: "CNY", Version: "fixture-v1", Input: input, OutputTiers: outputTiers},
				}}
				if len(price.LayerTiers) > 0 {
					layerBase := "0.3"
					layerTiers := make([]imagepricing.Tier, 0, len(price.LayerTiers))
					for _, tier := range price.LayerTiers {
						layerTiers = append(layerTiers, imagepricing.Tier{Name: tier.Name, MaxPixels: int64(tier.MaxPixels), Price: fmt.Sprint(tier.CNY)})
					}
					routePrice.Rules = append(routePrice.Rules, imagepricing.Rule{Operation: "layer_decomposition", Mode: "per_image", Currency: "CNY", Version: "fixture-v1", Input: input, Layers: &imagepricing.Layers{Confirmed: true, BasePrice: &layerBase, Tiers: layerTiers}})
				}
				if err := imagepricing.Validate(routePrice); err != nil {
					t.Fatal(err)
				}
				routing := nativeE2ERouting{modelhub.ModelRoutingSnapshot{Version: "fixture-native-v1", RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "native", Name: "Fake native", Visibility: "public", Enabled: true, ImagePricing: routePrice}}, ProviderModels: []modelhub.ProviderCandidate{{AccountModelID: 11, Provider: tc.adapter, AdapterType: tc.adapter, AuthType: "api_key", BaseURL: upstream.URL, Credentials: map[string]string{"api_key": "fake-native-key"}, ModelCode: tc.model, HealthStatus: "enabled", MaxImageCount: 6, ModelExtra: map[string]any{"native_image": modelhub.NativeImageConfig{Model: tc.model, Pricing: &price, SalePricing: &price}}}}, Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Priority: 1, Weight: 100, Enabled: true}}}}
				billing := billingservice.NewService(cfg.Billing)
				billing.SetModelRoutingSource(routing)
				if _, err := billing.AdminAdjust(ctx, bill.AdjustRequest{UserID: 7, ChangePoints: "10000.00000", Reason: "fixture", IdempotencyKey: "fixture-credit"}); err != nil {
					t.Fatal(err)
				}
				backend := storage.NewLocalBackend(t.TempDir())
				refs := assets.NewServiceWithStoreAndBackend(config.StorageConfig{}, cfg.GenerationLimits, nil, backend)
				images := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, map[string]provider.ImageProvider{}, imagetask.NewMemoryStore(), refs, billing, backend)
				images.SetModelRoutingSource(routing)
				principal := openplatform.Principal{UserID: 7, Scopes: []string{"images:read", "images:write"}}
				if transport == "api_key_rest" {
					principal.APIKeyID = "42"
				} else {
					principal.ClientID = "fixture-client"
					principal.GrantID = "fixture-grant"
				}
				var currentTaskID string
				download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					task, err := images.GetByID(r.Context(), 7, currentTaskID)
					if err != nil {
						http.Error(w, "missing task", 404)
						return
					}
					for _, result := range task.Results {
						if r.URL.Path == "/"+result.MediaAssetID {
							content, err := backend.Get(r.Context(), result.ObjectKey)
							if err != nil {
								http.Error(w, "missing object", 404)
								return
							}
							w.Header().Set("Content-Type", "image/png")
							_, _ = w.Write(content)
							return
						}
					}
					http.NotFound(w, r)
				}))
				defer download.Close()
				shared := openplatform.New(openplatform.Dependencies{Auth: nativeE2EUsers{}, Billing: billing, Images: images, References: refs, QuoteKey: []byte("fixture-quote-secret-at-least-32-bytes"), AssetAccess: func(_ context.Context, p openplatform.Principal, id uuid.UUID, purpose string) (any, error) {
					if p.UserID != 7 || purpose != "download" {
						t.Errorf("wrong access principal/purpose: %#v %s", p, purpose)
					}
					return map[string]any{"url": download.URL + "/" + id.String(), "expires_at": time.Now().UTC().Add(time.Minute), "range_supported": true}, nil
				}})
				var call func(string, map[string]any) map[string]any
				if transport == "oauth_mcp" {
					h, err := NewHandler(Config{ResourceURL: "https://fixture.example/mcp", Validator: ValidateAccessFunc(func(_ context.Context, token string) (openplatform.Principal, error) {
						if token != "fixture-oauth-token" {
							return openplatform.Principal{}, fmt.Errorf("invalid fixture token")
						}
						return principal, nil
					}), Service: shared})
					if err != nil {
						t.Fatal(err)
					}
					srv := httptest.NewServer(h)
					defer srv.Close()
					client := mcp.NewClient(&mcp.Implementation{Name: "native-fixture", Version: "1"}, nil)
					session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: bearerTransport{token: "fixture-oauth-token", next: http.DefaultTransport}}}, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer session.Close()
					call = func(op string, input map[string]any) map[string]any {
						t.Helper()
						result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: op, Arguments: input})
						if err != nil || result.IsError {
							t.Fatalf("%s: %v %#v", op, err, result)
						}
						raw, err := json.Marshal(result.StructuredContent)
						if err != nil {
							t.Fatal(err)
						}
						var out map[string]any
						if err = json.Unmarshal(raw, &out); err != nil {
							t.Fatal(err)
						}
						return out["data"].(map[string]any)
					}
				} else {
					srv := httptest.NewServer(handlers.NewOpenPlatform(shared, func(r *http.Request) (openplatform.Principal, error) {
						if r.Header.Get("Authorization") != "Bearer fixture-api-key" {
							return openplatform.Principal{}, fmt.Errorf("invalid fixture key")
						}
						return principal, nil
					}))
					defer srv.Close()
					call = func(op string, input map[string]any) map[string]any {
						t.Helper()
						method, path := http.MethodPost, "/image-tasks"
						switch op {
						case "preflight_image_task":
							path += "/preflight"
						case "get_image_task":
							method = http.MethodGet
							path += "/" + input["id"].(string)
							input = nil
						case "get_image_task_layers":
							method = http.MethodGet
							path += "/" + input["id"].(string) + "/layers"
							input = nil
						case "get_image_result_access":
							path += "/" + input["task_id"].(string) + "/results/" + input["image_id"].(string) + "/access"
							input = map[string]any{"purpose": "download"}
						}
						raw, _ := json.Marshal(input)
						req, err := http.NewRequestWithContext(ctx, method, srv.URL+"/api/open/v1"+path, bytes.NewReader(raw))
						if err != nil {
							t.Fatal(err)
						}
						req.Header.Set("Authorization", "Bearer fixture-api-key")
						req.Header.Set("Content-Type", "application/json")
						response, err := http.DefaultClient.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						defer response.Body.Close()
						var envelope struct {
							Data map[string]any `json:"data"`
						}
						body, _ := io.ReadAll(response.Body)
						if response.StatusCode >= 300 {
							t.Fatalf("%s HTTP %d: %s", op, response.StatusCode, body)
						}
						if err = json.Unmarshal(body, &envelope); err != nil {
							t.Fatal(err)
						}
						return envelope.Data
					}
				}
				request := map[string]any{"project_id": "fixture-project", "route_model_code": "native", "prompt": "fake provider integration fixture", "operation": tc.op, "size_mode": "pixel", "requested_size": "1024x1024", "output_format": "png", "output_image_count": 1}
				if tc.op == nativeimage.OperationLayerDecomposition {
					delete(request, "requested_size")
					request["size_mode"] = "auto"
				}
				if tc.op != nativeimage.OperationGenerate {
					ref, err := refs.Upload(7, "fixture.png", "image/png", pngBytes.Bytes())
					if err != nil {
						t.Fatal(err)
					}
					request["task_type"] = "image_edit"
					request["reference_asset_ids"] = []string{ref.ID}
				}
				quote := call("preflight_image_task", request)
				if providerCalls.Load() != 0 {
					t.Fatal("preflight called provider")
				}
				request["quote_token"] = quote["quote_token"]
				request["idempotency_key"] = "native-fixture"
				created := call("create_image_task", request)
				currentTaskID, _ = created["id"].(string)
				if currentTaskID == "" {
					t.Fatalf("missing persisted ID: %#v", created)
				}
				claimed, ok, err := images.AcquireNextTask(ctx, "fixture-worker", time.Minute)
				if err != nil || !ok {
					t.Fatalf("claim: %v %v", ok, err)
				}
				result, err := images.ExecuteLeasedTask(ctx, claimed, "fixture-worker", nil)
				if err != nil {
					t.Fatal(err)
				}
				if result.Task.Status != "succeeded" {
					t.Fatalf("worker status %s", result.Task.Status)
				}
				wantPoints, wantBalance := "18.00000", "9982.00000"
				if tc.op == nativeimage.OperationEdit {
					wantPoints, wantBalance = "20.00000", "9980.00000"
				}
				if tc.adapter == nativeimage.AdapterSeedream {
					wantPoints, wantBalance = "30.00000", "9970.00000"
					if tc.op == nativeimage.OperationLayerDecomposition {
						wantPoints, wantBalance = "45.00000", "9955.00000"
					}
				}
				if result.Task.ActualPoints != wantPoints {
					t.Fatalf("settled points=%s want %s", result.Task.ActualPoints, wantPoints)
				}
				replay := call("create_image_task", request)
				balance, err := billing.GetBalance(ctx, 7, "1.00000")
				if err != nil || balance.AvailablePoints != wantBalance || balance.FrozenPoints != "0.00000" {
					t.Fatalf("settled wallet after replay=%+v want available %s and no frozen points; err=%v", balance, wantBalance, err)
				}
				if replay["id"] != currentTaskID || providerCalls.Load() != 1 {
					t.Fatal("idempotent replay submitted again")
				}
				detail := call("get_image_task", map[string]any{"id": currentTaskID})
				if detail["status"] != "succeeded" {
					t.Fatalf("status: %#v", detail)
				}
				if tc.op == nativeimage.OperationLayerDecomposition {
					manifest := call("get_image_task_layers", map[string]any{"id": currentTaskID})
					layers, ok := manifest["layers"].([]any)
					if !ok || len(layers) != 2 {
						t.Fatalf("manifest: %#v", manifest)
					}
					for _, raw := range layers {
						layer := raw.(map[string]any)
						if layer["layer"] == nil || layer["access_operation"] != "get_image_result_access" {
							t.Fatalf("missing layer metadata: %#v", layer)
						}
						call("get_image_result_access", layer["access_input"].(map[string]any))
					}
				}
				for _, output := range result.Task.Results {
					access := call("get_image_result_access", map[string]any{"task_id": currentTaskID, "image_id": output.ID, "purpose": "download"})
					url, _ := access["url"].(string)
					if !strings.HasPrefix(url, download.URL+"/") {
						t.Fatalf("unexpected access URL %q", url)
					}
					response, err := http.Get(url)
					if err != nil {
						t.Fatal(err)
					}
					content, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil || response.StatusCode != 200 || !bytes.Equal(content, pngBytes.Bytes()) {
						t.Fatalf("download mismatch: status %d err %v", response.StatusCode, err)
					}
				}
				if _, err := shared.Execute(ctx, openplatform.Principal{UserID: 8, Scopes: []string{"images:read"}}, "get_image_task_layers", json.RawMessage(fmt.Sprintf(`{"id":%q}`, currentTaskID))); err == nil {
					t.Fatal("foreign task layers exposed")
				}
				t.Log("FAKE provider only; real REST/MCP, preflight, create, leased worker, storage and download verified")
			})
		}
	}
}
