package middleware

import (
	"net/http"
	"os"
	"strings"
)

const defaultAllowedOrigins = "http://localhost:5173,http://127.0.0.1:5173,http://localhost:5174,http://127.0.0.1:5174"

func CORS(next http.Handler) http.Handler {
	allowed := parseAllowedOrigins(os.Getenv("PIC_GALLERY_CORS_ALLOWED_ORIGINS"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			setCORSHeaders(w, r, origin)
		}
		if r.Method == http.MethodOptions && origin != "" {
			if !allowed[origin] {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseAllowedOrigins(raw string) map[string]bool {
	if strings.TrimSpace(raw) == "" {
		raw = defaultAllowedOrigins
	}
	allowed := make(map[string]bool)
	for _, origin := range strings.Split(raw, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = true
		}
	}
	return allowed
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request, origin string) {
	header := w.Header()
	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	if requested := r.Header.Get("Access-Control-Request-Headers"); requested != "" {
		header.Set("Access-Control-Allow-Headers", requested)
	} else {
		header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Access-Key, X-Body-SHA256, X-Signature, X-Timestamp")
	}
	header.Set("Access-Control-Max-Age", "600")
	header.Add("Vary", "Origin")
}
