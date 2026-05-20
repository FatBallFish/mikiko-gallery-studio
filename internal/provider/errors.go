package provider

import (
	"errors"
	"fmt"
	"strings"
)

type UpstreamErrorAction string

type UpstreamErrorFamily string

const (
	UpstreamErrorActionRetry    UpstreamErrorAction = "retry"
	UpstreamErrorActionWrap     UpstreamErrorAction = "wrap"
	UpstreamErrorActionPass     UpstreamErrorAction = "pass_through"
	UpstreamErrorActionInternal UpstreamErrorAction = "internal"
)

const (
	UpstreamErrorFamilyRateLimited  UpstreamErrorFamily = "rate_limited"
	UpstreamErrorFamilyBadRequest   UpstreamErrorFamily = "bad_request"
	UpstreamErrorFamilyUnauthorized UpstreamErrorFamily = "unauthorized"
	UpstreamErrorFamilyUnavailable  UpstreamErrorFamily = "unavailable"
	UpstreamErrorFamilyBlocked      UpstreamErrorFamily = "content_blocked"
	UpstreamErrorFamilyUnknown      UpstreamErrorFamily = "unknown"
)

type UpstreamError struct {
	Provider   ProviderType
	HTTPStatus int
	Code       string
	Type       string
	Message    string
	RequestID  string
	Action     UpstreamErrorAction
	Family     UpstreamErrorFamily
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := []string{string(e.Provider), fmt.Sprintf("status=%d", e.HTTPStatus)}
	if e.Code != "" {
		parts = append(parts, "code="+e.Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, " ")
}

func ClassifyUpstreamError(err *UpstreamError) {
	if err == nil {
		return
	}

	code := strings.ToLower(err.Code)
	typ := strings.ToLower(err.Type)
	switch {
	case err.HTTPStatus == 429 || code == "rate_limit_error":
		err.Action = UpstreamErrorActionRetry
		err.Family = UpstreamErrorFamilyRateLimited
	case err.HTTPStatus == 500 || err.HTTPStatus == 502 || err.HTTPStatus == 503 || err.HTTPStatus == 504:
		err.Action = UpstreamErrorActionRetry
		err.Family = UpstreamErrorFamilyUnavailable
	case err.HTTPStatus == 401 || err.HTTPStatus == 403:
		err.Action = UpstreamErrorActionInternal
		err.Family = UpstreamErrorFamilyUnauthorized
	case err.HTTPStatus == 400 || err.HTTPStatus == 422 || code == "invalid_request_error" || typ == "invalid_request_error":
		err.Action = UpstreamErrorActionWrap
		err.Family = UpstreamErrorFamilyBadRequest
	case strings.Contains(code, "content") || strings.Contains(code, "safety") || strings.Contains(typ, "content"):
		err.Action = UpstreamErrorActionWrap
		err.Family = UpstreamErrorFamilyBlocked
	default:
		err.Action = UpstreamErrorActionInternal
		err.Family = UpstreamErrorFamilyUnknown
	}
}

func AsUpstreamError(err error) (*UpstreamError, bool) {
	var target *UpstreamError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
