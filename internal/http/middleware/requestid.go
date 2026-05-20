package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const requestIDHeader = "X-Request-Id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}

		ctx := httpx.ContextWithRequestID(r.Context(), requestID)
		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "fallback-request-id"
	}
	return hex.EncodeToString(buffer)
}
