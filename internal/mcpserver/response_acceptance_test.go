package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	video "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These acceptance tests exercise the authenticated SDK/HTTP boundary. In
// particular, the budget applies to the serialized CallToolResult, including
// both content and structuredContent, rather than just the application data.
const acceptanceWireBudget = 40 << 10

func acceptanceSession(t *testing.T, exec executeFunc) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	scopes := []string{"account:read", "models:read", "images:read", "images:write", "videos:read", "videos:write"}
	h, err := NewHandler(Config{
		ResourceURL: "https://acceptance.example/mcp",
		Service:     exec,
		Validator: ValidateAccessFunc(func(_ context.Context, token string) (openplatform.Principal, error) {
			if token != "acceptance-fixture" {
				return openplatform.Principal{}, fmt.Errorf("unexpected fixture token")
			}
			return openplatform.Principal{UserID: 71, ClientID: "acceptance", GrantID: "fixture", Scopes: scopes}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Capture the real HTTP tools/list response, including JSON-RPC/SSE framing.
	var maxListWire atomic.Int64
	t.Cleanup(func() {
		if size := maxListWire.Load(); size > 0 {
			t.Logf("tools/list maximum HTTP wire=%d bytes", size)
		}
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var request struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &request)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if request.Method == "tools/list" {
			for old := maxListWire.Load(); int64(rec.Body.Len()) > old; old = maxListWire.Load() {
				if maxListWire.CompareAndSwap(old, int64(rec.Body.Len())) {
					break
				}
			}
		}
		if request.Method == "tools/list" && rec.Body.Len() >= acceptanceWireBudget {
			t.Errorf("tools/list HTTP wire = %d bytes, want < %d", rec.Body.Len(), acceptanceWireBudget)
		}
		for k, values := range rec.Header() {
			for _, value := range values {
				w.Header().Add(k, value)
			}
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	}))
	t.Cleanup(srv.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "response-acceptance", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: "acceptance-fixture", next: http.DefaultTransport}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return ctx, cs
}

func acceptanceCall(t *testing.T, ctx context.Context, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s transport: %v", name, err)
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("%s marshal: %v", name, err)
	}
	if len(wire) >= acceptanceWireBudget {
		t.Errorf("%s CallToolResult JSON = %d bytes, want < %d (text + structuredContent included)", name, len(wire), acceptanceWireBudget)
	}
	if result.IsError {
		// Bound diagnostic output too: a failed budget test must not print its giant fixture.
		message := string(wire)
		if len(message) > 512 {
			message = message[:512]
		}
		t.Fatalf("%s returned IsError=true: %s", name, message)
	}
	return result
}

func acceptanceEnvelope(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil || env == nil {
		t.Fatalf("missing structured result: %v", err)
	}
	return env
}

func TestResponseAcceptanceToolsListAllCursorPages(t *testing.T) {
	var executions atomic.Int32
	ctx, cs := acceptanceSession(t, executeFunc(func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error) {
		executions.Add(1)
		return nil, fmt.Errorf("tools/list must not execute an operation")
	}))
	expected := map[string]bool{}
	for _, spec := range Catalogue() {
		expected[spec.Name] = true
	}
	expected["read_tool_result"] = true
	seen, cursors := map[string]bool{}, map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := cs.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		wire, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if len(wire) >= acceptanceWireBudget {
			t.Errorf("tools/list page %d JSON = %d, want < %d", pages, len(wire), acceptanceWireBudget)
		}
		if len(page.Tools) == 0 {
			t.Fatal("empty tools page before enumeration completed")
		}
		for _, tool := range page.Tools {
			if seen[tool.Name] {
				t.Errorf("duplicate tool across pages: %s", tool.Name)
			}
			if !expected[tool.Name] {
				t.Errorf("unexpected tool: %s", tool.Name)
			}
			seen[tool.Name] = true
			if tool.InputSchema == nil || tool.OutputSchema == nil {
				t.Errorf("missing schema: %s", tool.Name)
			}
		}
		if page.NextCursor == "" {
			break
		}
		if cursors[page.NextCursor] {
			t.Fatalf("cursor did not progress: %q", page.NextCursor)
		}
		cursors[page.NextCursor] = true
		cursor = page.NextCursor
		if pages > len(expected)+1 {
			t.Fatal("tools/list never completed")
		}
	}
	for name := range expected {
		if !seen[name] {
			t.Errorf("tool omitted from cursor enumeration: %s", name)
		}
	}
	if pages < 2 {
		t.Error("expected bounded tools/list pagination")
	}
	t.Logf("tools/list total tools=%d pages=%d", len(seen), pages)
	if executions.Load() != 0 {
		t.Errorf("tools/list executed service %d times", executions.Load())
	}
}

// Marshal the real DTO before returning a generic executor result. Validate
// fixtures independently against the original shared schema, never Catalogue's
// generic MCP envelope, so missing DTO fields cannot produce misleading greens.
func acceptanceDTO(t *testing.T, operation string, dto any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	for _, spec := range openplatform.Specs() {
		if spec.Name != operation {
			continue
		}
		schema, err := resolveSchema(spec.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(value); err != nil {
			t.Fatalf("%s DTO fixture violates original schema: %v", operation, err)
		}
		return value
	}
	t.Fatalf("missing original operation schema: %s", operation)
	return nil
}

func acceptanceTask(t *testing.T, operation, id, status, prompt string) map[string]any {
	t.Helper()
	return acceptanceDTO(t, operation, image.Task{
		ID: id, ProjectID: "fixture-project", Status: status,
		Operation: nativeimage.OperationGenerate, AbstractModel: "acceptance-model", RouteModelCode: "acceptance-model",
		TaskType: "text_to_image", Prompt: prompt, BaseResolution: "1k", Quality: "standard",
		OutputFormat: "png", OutputImageCount: 1,
		CreatedAt: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
	})
}

func TestResponseAcceptanceTaskResultWireBudget(t *testing.T) {
	for _, size := range []int{24 << 10, 927 << 10} {
		t.Run(fmt.Sprintf("payload_%d", size), func(t *testing.T) {
			fixture := acceptanceTask(t, "get_image_task", "task-large", image.StatusSucceeded, strings.Repeat("x", size))
			var calls atomic.Int32
			ctx, cs := acceptanceSession(t, executeFunc(func(_ context.Context, _ openplatform.Principal, op string, input json.RawMessage) (any, error) {
				calls.Add(1)
				if op != "get_image_task" {
					return nil, fmt.Errorf("unexpected operation %s", op)
				}
				return fixture, nil
			}))
			result := acceptanceCall(t, ctx, cs, "get_image_task", map[string]any{"id": "task-large"})
			env := acceptanceEnvelope(t, result)
			if env["data"] == nil && env["snapshot_token"] == nil {
				t.Error("task detail has neither inline data nor continuation")
			}
			if calls.Load() != 1 {
				t.Errorf("executor called %d times, want 1", calls.Load())
			}
		})
	}
}

func TestResponseAcceptanceModelsDefaultSummary(t *testing.T) {
	// One realistic image model with a huge capabilities section approximates the
	// observed 927 KiB catalogue. The default reply must expose selectable models,
	// not simply move the full catalogue behind an opaque snapshot token.
	marker := "FULL_CAPABILITY_FIXTURE_"
	full := strings.Repeat(marker, (927<<10)/len(marker)+1)
	fixture := acceptanceDTO(t, "list_image_models", capabilities.Response{
		ModelGroups: []modelhub.VisibleRouteModel{{
			ID: 1, Code: "acceptance-model", Name: "Acceptance model", TaskTypes: []string{"text_to_image"},
			Description: full, BaseResolution: []string{"1k"}, Quality: []string{"standard"},
			SizeModes: []string{"pixel"}, AspectRatios: []string{"1:1"}, PixelSizes: []string{"1024x1024"}, OutputFormat: []string{"png"},
			AutoBaseResolutionByTaskType: map[string]string{"text_to_image": "1k"},
			MaxOutputImageCount:          1, EffectiveMultiplier: "1", MinimumPoints: "1",
			CapabilitiesByTaskType: map[string]modelhub.VisibleRouteModelTaskCapability{"text_to_image": {BaseResolution: []string{"1k"}, PixelSizes: []string{"1024x1024"}}},
			Prices:                 []modelhub.VisibleRouteModelPrice{{TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1", ChargedPoints: "1", DisplayPoints: "1"}},
		}},
		RouteModelGroups: []capabilities.RouteModelGroupInfo{{Code: "fixture-group", Name: "Fixture group", RouteModelCodes: []string{"acceptance-model"}}},
	})
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 927<<10 {
		t.Fatal("synthetic model fixture is smaller than 927 KiB")
	}
	var calls atomic.Int32
	ctx, cs := acceptanceSession(t, executeFunc(func(_ context.Context, _ openplatform.Principal, op string, input json.RawMessage) (any, error) {
		calls.Add(1)
		if op != "list_image_models" {
			return nil, fmt.Errorf("unexpected operation %s", op)
		}
		return fixture, nil
	}))
	result := acceptanceCall(t, ctx, cs, "list_image_models", map[string]any{})
	resultWire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("image model original DTO=%d bytes; summary CallToolResult wire=%d bytes", len(raw), len(resultWire))
	env := acceptanceEnvelope(t, result)
	summary, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(summary, []byte(`"acceptance-model"`)) {
		t.Error("default models reply omits selectable model code; want summary, not a full-result snapshot only")
	}
	if bytes.Contains(summary, []byte(marker)) {
		t.Error("default models reply includes full capability payload instead of summary")
	}
	acceptanceModelDetails(t, ctx, cs, env, fixture, "model_groups", "code", "acceptance-model")
	if calls.Load() != 1 {
		t.Errorf("executor called %d times, want 1", calls.Load())
	}
}

// Verify each advertised detail path resolves to the original full DTO model,
// including constraints and prices. Reassembly traverses only the public tool.
func acceptanceModelDetails(t *testing.T, ctx context.Context, cs *mcp.ClientSession, env, fixture map[string]any, key, codeKey, code string) {
	t.Helper()
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatal("default model summary missing data object")
	}
	models, ok := data[key].([]any)
	if !ok || len(models) != 1 {
		t.Fatalf("summary missing complete %s enumeration", key)
	}
	model, ok := models[0].(map[string]any)
	if !ok || model[codeKey] != code {
		t.Fatalf("summary lost selectable %s", codeKey)
	}
	path, ok := model["details_path"].(string)
	if !ok || path != "/"+key+"/0" {
		t.Fatal("summary missing original model details_path")
	}
	details, ok := env["details"].(map[string]any)
	if !ok || details["tool"] != "read_tool_result" {
		t.Fatal("summary missing detail recovery tool")
	}
	token, ok := details["token"].(string)
	if !ok || token == "" {
		t.Fatal("summary missing detail snapshot token")
	}
	full := nativeReadSnapshot(t, ctx, cs, token, path, "object", 0)
	got, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	original, ok := fixture[key].([]any)
	if !ok || len(original) != 1 {
		t.Fatal("invalid model fixture")
	}
	want, err := json.Marshal(original[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("model details_path did not recover full DTO: got %d bytes want %d", len(got), len(want))
	}
}

func TestResponseAcceptanceVideoModelsSummaryAndDetails(t *testing.T) {
	marker := "FULL_VIDEO_CAPABILITY_"
	full := strings.Repeat(marker, (927<<10)/len(marker)+1)
	fixture := acceptanceDTO(t, "list_video_models", videorouting.CapabilityListResponse{
		Groups: []videorouting.CapabilityResponse{{
			RouteModelCode: "acceptance-video", Name: "Acceptance video", Description: full,
			MinimumPoints: "2", ConfigVersion: "config-v1", CapabilityVersion: "cap-v1", MaxOutputCount: 1,
			TaskTypes: []video.TaskType{video.TaskTypeTextToVideo}, Combinations: []video.Combination{},
			InputCapabilitiesByTaskType: map[video.TaskType]map[video.InputRole]video.InputCapability{},
		}},
		RouteModelGroups: []videorouting.RouteModelGroupInfo{{Code: "video-picker", Name: "Video picker", RouteModelCodes: []string{"acceptance-video"}}},
	})
	var calls atomic.Int32
	ctx, cs := acceptanceSession(t, executeFunc(func(_ context.Context, _ openplatform.Principal, op string, input json.RawMessage) (any, error) {
		calls.Add(1)
		if op != "list_video_models" {
			return nil, fmt.Errorf("unexpected operation %s", op)
		}
		return fixture, nil
	}))
	result := acceptanceCall(t, ctx, cs, "list_video_models", map[string]any{})
	resultWire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	originalWire, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(originalWire) < 927<<10 {
		t.Fatal("video fixture smaller than 927 KiB")
	}
	t.Logf("video model original DTO=%d bytes; summary CallToolResult wire=%d bytes", len(originalWire), len(resultWire))
	env := acceptanceEnvelope(t, result)
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(marker)) {
		t.Error("video default summary leaked full model payload")
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatal("video summary missing data")
	}
	groups, ok := data["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatal("video summary omitted groups")
	}
	model, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatal("video summary model is not object")
	}
	for key, want := range map[string]string{"route_model_code": "acceptance-video", "config_version": "config-v1", "capability_version": "cap-v1", "minimum_points": "2"} {
		if model[key] != want {
			t.Errorf("video summary lost %s", key)
		}
	}
	acceptanceModelDetails(t, ctx, cs, env, fixture, "groups", "route_model_code", "acceptance-video")
	if calls.Load() != 1 {
		t.Errorf("detail continuation re-executed service: %d calls", calls.Load())
	}
}

func TestResponseAcceptanceChineseStringFragmentReassembly(t *testing.T) {
	// Quotes, backslashes and HTML-sensitive characters also exercise JSON wire
	// expansion. 8192 bytes splits a Chinese rune unless the reader aligns UTF-8.
	original := strings.Repeat("中文片段\\\"<>&换行\n", 20000)
	fixture := acceptanceTask(t, "get_image_task", "task-chinese", image.StatusSucceeded, original)
	var calls atomic.Int32
	ctx, cs := acceptanceSession(t, executeFunc(func(_ context.Context, _ openplatform.Principal, op string, input json.RawMessage) (any, error) {
		calls.Add(1)
		if op != "get_image_task" {
			return nil, fmt.Errorf("read_tool_result unexpectedly delegated to executor: %s", op)
		}
		return fixture, nil
	}))
	initial := acceptanceEnvelope(t, acceptanceCall(t, ctx, cs, "get_image_task", map[string]any{"id": "task-chinese"}))
	token, ok := initial["snapshot_token"].(string)
	if !ok || token == "" {
		t.Fatal("large task did not return a snapshot_token")
	}
	var reconstructed strings.Builder
	offset := 0
	pages := 0
	maxFragmentWire := 0
	for {
		result := acceptanceCall(t, ctx, cs, "read_tool_result", map[string]any{"token": token, "path": "/prompt", "offset": offset, "limit": 8192})
		wire, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		maxFragmentWire = max(maxFragmentWire, len(wire))
		page := acceptanceEnvelope(t, result)
		fragment, ok := page["value"].(string)
		if !ok {
			t.Fatalf("string page lacks value fragment (type=%v)", page["type"])
		}
		if !utf8.ValidString(fragment) {
			t.Fatal("fragment contains broken UTF-8")
		}
		next, ok := page["next_offset"].(float64)
		if !ok || int(next) != offset+len(fragment) {
			t.Fatalf("offset did not consume exactly fragment UTF-8 bytes: offset=%d fragment=%d next=%v", offset, len(fragment), page["next_offset"])
		}
		if page["path"] != "/prompt" || page["offset"] != float64(offset) {
			t.Fatalf("page does not identify requested path/offset: path=%v offset=%v", page["path"], page["offset"])
		}
		reconstructed.WriteString(fragment)
		pages++
		complete, ok := page["complete"].(bool)
		if !ok {
			t.Fatal("page lacks complete flag")
		}
		if complete {
			break
		}
		if int(next) <= offset {
			t.Fatal("nonterminal fragment made no progress")
		}
		offset = int(next)
		if pages > len(original) {
			t.Fatal("fragment traversal did not terminate")
		}
	}
	t.Logf("Chinese string=%d UTF-8 bytes; fragment pages=%d maximum CallToolResult wire=%d bytes", len(original), pages, maxFragmentWire)
	if reconstructed.String() != original {
		t.Errorf("reassembled string differs: got %d bytes, want %d", reconstructed.Len(), len(original))
	}
	if pages < 2 {
		t.Error("large string unexpectedly returned as one fragment")
	}
	if calls.Load() != 1 {
		t.Errorf("snapshot traversal executed application %d times, want 1", calls.Load())
	}
}

func TestResponseAcceptanceCompletedWriteSurvivesSnapshotCapacity(t *testing.T) {
	// Larger than the documented 8 MiB per-snapshot capacity. Do not depend on
	// private cache functions: success/failure must be correct over tools/call.
	const taskID = "durable-created-task"
	fixture := acceptanceTask(t, "create_image_task", taskID, image.StatusQueued, strings.Repeat("x", 9<<20))
	var calls atomic.Int32
	ctx, cs := acceptanceSession(t, executeFunc(func(_ context.Context, _ openplatform.Principal, op string, input json.RawMessage) (any, error) {
		calls.Add(1)
		if op != "create_image_task" {
			return nil, fmt.Errorf("unexpected operation %s", op)
		}
		return fixture, nil
	}))
	// Check the execution count even when a tool error causes the assertion to fail.
	defer func() {
		if calls.Load() != 1 {
			t.Errorf("completed write executed %d times, want exactly 1", calls.Load())
		}
	}()
	result := acceptanceCall(t, ctx, cs, "create_image_task", map[string]any{"project_id": "fixture-project", "route_model_code": "fixture-model", "idempotency_key": "fixture-create-once"})
	env := acceptanceEnvelope(t, result)
	if env["operation_completed"] != true {
		t.Errorf("successful write lacks operation_completed=true: %v", env["operation_completed"])
	}
	id := env["id"]
	if data, ok := env["data"].(map[string]any); ok && id == nil {
		id = data["id"]
	}
	if id != taskID {
		t.Errorf("successful write lost durable id: got %v, want %s", id, taskID)
	}
}
