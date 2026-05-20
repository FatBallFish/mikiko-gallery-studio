package provider_test

import (
	"errors"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/provider"
)

func TestClassifyUpstreamError(t *testing.T) {
	cases := []struct {
		name       string
		input      *provider.UpstreamError
		wantAction provider.UpstreamErrorAction
		wantFamily provider.UpstreamErrorFamily
	}{
		{
			name: "openai rate limit retries",
			input: &provider.UpstreamError{
				Provider:   provider.ProviderTypeOpenAI,
				HTTPStatus: 429,
				Code:       "rate_limit_error",
			},
			wantAction: provider.UpstreamErrorActionRetry,
			wantFamily: provider.UpstreamErrorFamilyRateLimited,
		},
		{
			name: "openrouter bad request wraps",
			input: &provider.UpstreamError{
				Provider:   provider.ProviderTypeOpenRouter,
				HTTPStatus: 400,
				Code:       "bad_request",
			},
			wantAction: provider.UpstreamErrorActionWrap,
			wantFamily: provider.UpstreamErrorFamilyBadRequest,
		},
		{
			name: "provider auth stays internal",
			input: &provider.UpstreamError{
				Provider:   provider.ProviderTypeOpenAI,
				HTTPStatus: 401,
				Code:       "invalid_api_key",
			},
			wantAction: provider.UpstreamErrorActionInternal,
			wantFamily: provider.UpstreamErrorFamilyUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider.ClassifyUpstreamError(tc.input)
			if tc.input.Action != tc.wantAction {
				t.Fatalf("expected action %q, got %q", tc.wantAction, tc.input.Action)
			}
			if tc.input.Family != tc.wantFamily {
				t.Fatalf("expected family %q, got %q", tc.wantFamily, tc.input.Family)
			}
		})
	}
}

func TestAsUpstreamError(t *testing.T) {
	original := &provider.UpstreamError{Provider: provider.ProviderTypeOpenRouter, HTTPStatus: 503}
	resolved, ok := provider.AsUpstreamError(original)
	if !ok {
		t.Fatalf("expected AsUpstreamError to match")
	}
	if resolved != original {
		t.Fatalf("expected same error instance")
	}

	if _, ok := provider.AsUpstreamError(errors.New("plain")); ok {
		t.Fatalf("expected plain error not to match")
	}
}
