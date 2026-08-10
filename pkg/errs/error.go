package errs

import "net/http"

type Error struct {
	Code           string         `json:"code"`
	Message        string         `json:"message"`
	Chargeable     bool           `json:"chargeable"`
	NextSuggestion string         `json:"next_suggestion,omitempty"`
	StatusCode     int            `json:"-"`
	Details        map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

func New(statusCode int, code, message string) *Error {
	return &Error{Code: code, Message: message, Chargeable: false, NextSuggestion: defaultSuggestion(code), StatusCode: statusCode}
}

func BadRequest(message string) *Error {
	return New(http.StatusBadRequest, CodeBadRequest, message)
}

func Unauthorized(message string) *Error {
	return New(http.StatusUnauthorized, CodeUnauthorized, message)
}

func Internal(message string) *Error {
	if message == "" {
		message = "internal server error"
	}
	return New(http.StatusInternalServerError, CodeInternal, message)
}

func WithDetails(err *Error, details map[string]any) *Error {
	err.Details = details
	return err
}

func defaultSuggestion(code string) string {
	switch code {
	case CodeRateLimited:
		return "retry after the cooldown window"
	case CodeInsufficientPoints:
		return "redeem or add points before creating a task"
	case CodeUnauthorized, CodeAuthAccessExpired, CodeAuthRefreshExpired:
		return "sign in again and retry"
	case CodeImageReferenceRequired, CodeImageReferenceExceeded, CodeImageReferenceTooLarge, CodeValidationFailed, CodeBadRequest, CodePaymentProviderConfigInvalid, CodeExportTooLarge:
		return "adjust request parameters and retry"
	case CodeTextModelDefaultRequired:
		return "select a default text model and retry"
	case CodeConfigurationInUse:
		return "remove the dependent configuration and retry"
	default:
		return "retry later or contact support if the problem persists"
	}
}
