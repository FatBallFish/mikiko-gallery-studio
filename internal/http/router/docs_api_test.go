package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/http/handlers"
)

func TestDocsEndpointsReturnStructuredContract(t *testing.T) {
	handler := NewWithAPI(handlers.NewAPI(adminConfigAPIConfig(), nil, nil))

	openAPIReq := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	openAPIRec := httptest.NewRecorder()
	handler.ServeHTTP(openAPIRec, openAPIReq)
	if openAPIRec.Code != http.StatusOK {
		t.Fatalf("expected openapi json 200, got %d body=%s", openAPIRec.Code, openAPIRec.Body.String())
	}
	var openAPIResp struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(openAPIRec.Body).Decode(&openAPIResp); err != nil {
		t.Fatalf("decode openapi json: %v", err)
	}
	if openAPIResp.OpenAPI == "" || len(openAPIResp.Paths) == 0 {
		t.Fatalf("expected non-empty openapi document, got openapi=%q paths=%d", openAPIResp.OpenAPI, len(openAPIResp.Paths))
	}

	examplesReq := httptest.NewRequest(http.MethodGet, "/docs/examples", nil)
	examplesRec := httptest.NewRecorder()
	handler.ServeHTTP(examplesRec, examplesReq)
	if examplesRec.Code != http.StatusOK {
		t.Fatalf("expected examples 200, got %d body=%s", examplesRec.Code, examplesRec.Body.String())
	}
	var examplesResp struct {
		Data struct {
			Items []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Language string `json:"language"`
				Code     string `json:"code"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(examplesRec.Body).Decode(&examplesResp); err != nil {
		t.Fatalf("decode examples: %v", err)
	}
	if len(examplesResp.Data.Items) == 0 {
		t.Fatalf("expected structured docs examples")
	}
	for _, item := range examplesResp.Data.Items {
		if item.ID == "" || item.Title == "" || item.Language == "" || item.Code == "" {
			t.Fatalf("example item is incomplete: %#v", item)
		}
		if item.Language == "html" || item.Code[0] == '<' {
			t.Fatalf("example item must contain copyable code, got %#v", item)
		}
	}

	errorsReq := httptest.NewRequest(http.MethodGet, "/docs/errors", nil)
	errorsRec := httptest.NewRecorder()
	handler.ServeHTTP(errorsRec, errorsReq)
	if errorsRec.Code != http.StatusOK {
		t.Fatalf("expected errors 200, got %d body=%s", errorsRec.Code, errorsRec.Body.String())
	}
	var errorsResp struct {
		Data struct {
			Items []struct {
				Code       string `json:"code"`
				HTTPStatus int    `json:"http_status"`
				Message    string `json:"message"`
				Retryable  bool   `json:"retryable"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(errorsRec.Body).Decode(&errorsResp); err != nil {
		t.Fatalf("decode errors: %v", err)
	}
	if len(errorsResp.Data.Items) == 0 {
		t.Fatalf("expected structured docs errors")
	}
	errorCodes := map[string]bool{}
	for _, item := range errorsResp.Data.Items {
		if item.Code == "" || item.HTTPStatus == 0 || item.Message == "" {
			t.Fatalf("error item is incomplete: %#v", item)
		}
		errorCodes[item.Code] = true
	}
	for _, code := range []string{
		"MODEL_ROUTE_NOT_FOUND",
		"MODEL_ROUTE_NO_CANDIDATE",
		"ROUTE_MODEL_PRICE_MISSING",
		"PAYMENT_METHOD_UNAVAILABLE",
		"PAYMENT_PROVIDER_UNAVAILABLE",
		"PAYMENT_TOO_MANY_PENDING_ORDERS",
		"PAYMENT_SIGNATURE_INVALID",
		"PAYMENT_AMOUNT_MISMATCH",
	} {
		if !errorCodes[code] {
			t.Fatalf("expected docs errors to include %s, got %#v", code, errorCodes)
		}
	}
}
