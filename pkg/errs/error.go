package errs

import "net/http"

type Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	StatusCode int            `json:"-"`
	Details    map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

func New(statusCode int, code, message string) *Error {
	return &Error{Code: code, Message: message, StatusCode: statusCode}
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
