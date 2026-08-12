package video

import (
	"errors"
	"fmt"
)

type ErrorCategory string

const (
	ErrorInvalidRequest  ErrorCategory = "invalid_request"
	ErrorUnauthorized    ErrorCategory = "unauthorized"
	ErrorRateLimited     ErrorCategory = "rate_limited"
	ErrorContentBlocked  ErrorCategory = "content_blocked"
	ErrorUnavailable     ErrorCategory = "unavailable"
	ErrorInvalidResponse ErrorCategory = "invalid_response"
)

type Error struct {
	Provider          string
	Category          ErrorCategory
	Code              string
	Message           string
	HTTPStatus        int
	RequestID         string
	Retryable         bool
	SubmissionUnknown bool
	Cause             error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return fmt.Sprintf("%s video provider: %s", e.Provider, e.Message)
	}
	return fmt.Sprintf("%s video provider error", e.Provider)
}

func (e *Error) Unwrap() error { return e.Cause }

func AsError(err error) (*Error, bool) {
	var target *Error
	return target, errors.As(err, &target)
}

func ClassifyHTTP(provider string, status int, code, message, requestID string) *Error {
	result := &Error{Provider: provider, HTTPStatus: status, Code: code, Message: message, RequestID: requestID}
	switch status {
	case 400, 422:
		result.Category = ErrorInvalidRequest
	case 401, 403:
		result.Category = ErrorUnauthorized
	case 429:
		result.Category = ErrorRateLimited
		result.Retryable = true
	case 500, 502, 503, 504, 529:
		result.Category = ErrorUnavailable
		result.Retryable = true
	default:
		result.Category = ErrorInvalidResponse
	}
	return result
}
