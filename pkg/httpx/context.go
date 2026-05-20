package httpx

import "context"

type contextKey string

const requestIDKey contextKey = "request_id"

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(requestIDKey).(string); ok {
		return value
	}
	return ""
}
