package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	billing "github.com/fatballfish/pic-gallery/internal/domain/billing"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAsyncTaskToolsDispatchSharedOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	operations := []string{}
	handler, err := NewHandler(Config{ResourceURL: "https://studio.example/mcp", Validator: ValidateAccessFunc(func(context.Context, string) (openplatform.Principal, error) {
		return openplatform.Principal{UserID: 7, ClientID: "client", GrantID: "grant", Scopes: []string{"images:write", "images:read", "videos:write", "videos:read"}}, nil
	}), Service: executeFunc(func(_ context.Context, p openplatform.Principal, op string, input json.RawMessage) (any, error) {
		if p.UserID != 7 || p.ClientID != "client" || p.GrantID != "grant" {
			t.Error("OAuth attribution lost")
		}
		operations = append(operations, op)
		switch op {
		case "preflight_image_task":
			return struct {
				billing.EstimateResult
				QuoteToken string    `json:"quote_token"`
				ExpiresAt  time.Time `json:"expires_at"`
			}{QuoteToken: "quote", ExpiresAt: time.Now().UTC().Add(time.Minute)}, nil
		case "preflight_video_task":
			return videotask.Estimate{QuoteToken: "quote"}, nil
		case "create_video_task":
			return map[string]any{"task": videotask.Task{}, "replayed": false}, nil
		case "get_video_task":
			return videotask.Task{}, nil
		default:
			return image.Task{ID: "persisted-task", Status: "queued"}, nil
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "async-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: bearerTransport{token: "token", next: http.DefaultTransport}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, kind := range []string{"image", "video"} {
		request := map[string]any{"project_id": "project", "route_model_code": "model"}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "preflight_" + kind + "_task", Arguments: request})
		if err != nil || result.IsError {
			t.Fatalf("preflight: %v %#v", err, result)
		}
		count := len(operations)
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "create_" + kind + "_task", Arguments: request})
		if err != nil || !result.IsError || len(operations) != count {
			t.Fatalf("missing idempotency accepted: %v %#v", err, result)
		}
		request["idempotency_key"] = "retry-safe"
		request["quote_token"] = "quote"
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "create_" + kind + "_task", Arguments: request})
		if err != nil || result.IsError {
			t.Fatalf("create: %v %#v", err, result)
		}
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "get_" + kind + "_task", Arguments: map[string]any{"id": "persisted-task"}})
		if err != nil || result.IsError {
			t.Fatalf("status: %v %#v", err, result)
		}
	}
	if len(operations) != 6 {
		t.Fatalf("expected six shared operations, got %v", operations)
	}
}
