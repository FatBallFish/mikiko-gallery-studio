package openapi

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestRouteContractIsNotParsedDuringPackageInitialization(t *testing.T) {
	source, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	if strings.Contains(string(source), "var routeContract = mustLoadRouteContract(document)") {
		t.Fatal("embedded OpenAPI route contract must load only when the normal router is constructed")
	}
}

func TestAllowsMatchesEmbeddedRouteMethods(t *testing.T) {
	testCases := []struct {
		method string
		path   string
		want   bool
	}{
		{method: http.MethodPost, path: "/api/agent/auth/v1/login/password", want: true},
		{method: http.MethodGet, path: "/api/agent/auth/v1/login/password", want: false},
		{method: http.MethodDelete, path: "/api/ops/admin/v1/users/42", want: true},
		{method: http.MethodPatch, path: "/api/ops/admin/v1/users/42", want: false},
		{method: http.MethodPost, path: "/api/ops/admin/v1/storage-configs/42:set-default", want: true},
		{method: http.MethodGet, path: "/api/ops/admin/v1/storage-configs/42:set-default", want: false},
		{method: http.MethodGet, path: "/api/setup/v1/progress/op-123", want: true},
		{method: http.MethodGet, path: "/api/setup/v1/progress/op-123/extra", want: false},
		{method: http.MethodPost, path: "/api/agent/auth/v1/login/password/", want: false},
		{method: http.MethodDelete, path: "/api/ops/admin/v1/users/42/", want: false},
		{method: http.MethodDelete, path: "/api/ops/admin/v1/users//42", want: false},
		{method: http.MethodGet, path: "/not-registered", want: false},
	}
	for _, testCase := range testCases {
		if got := Allows(testCase.method, testCase.path); got != testCase.want {
			t.Errorf("Allows(%q, %q) = %t, want %t", testCase.method, testCase.path, got, testCase.want)
		}
	}
}

func TestParseRouteContractFailsClosed(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("paths: ["),
		[]byte("openapi: 3.1.0\npaths: {}\n"),
		[]byte("openapi: 3.1.0\npaths:\n  /healthz:\n    parameters: []\n"),
	} {
		contract, err := parseRouteContract(data)
		if err == nil || contract != nil {
			t.Fatalf("parseRouteContract(%q) = (%v, %v), want nil contract and error", data, contract, err)
		}
	}
}
