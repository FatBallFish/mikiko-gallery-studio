package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Legacy initialization is exercised at the HTTP boundary. Production parsing and
// version selection remain entirely in the SDK, including header defaults.
func TestPreviousProtocolInitialization(t *testing.T) {
	handler, err := NewHandler(Config{ResourceURL: "https://studio.example/mcp", Service: executeFunc(func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error) {
		return map[string]any{}, nil
	}), Validator: ValidateAccessFunc(func(context.Context, string) (openplatform.Principal, error) {
		return openplatform.Principal{UserID: 1, ClientID: "client", GrantID: "grant"}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"} {
		t.Run(version, func(t *testing.T) {
			payload, err := json.Marshal(struct {
				Version string                `json:"jsonrpc"`
				ID      int                   `json:"id"`
				Method  string                `json:"method"`
				Params  *mcp.InitializeParams `json:"params"`
			}{"2.0", 1, "initialize", &mcp.InitializeParams{ProtocolVersion: version, ClientInfo: &mcp.Implementation{Name: "legacy-test", Version: "1"}, Capabilities: &mcp.ClientCapabilities{}}})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "https://studio.example/mcp", bytes.NewReader(payload))
			req.Header.Set("Authorization", "Bearer valid")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != 200 || !strings.Contains(response.Body.String(), `"protocolVersion":"`+version+`"`) {
				t.Fatalf("legacy initialize %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Mcp-Session-Id") != "" {
				t.Fatal("stateless transport created session")
			}
		})
	}
}
