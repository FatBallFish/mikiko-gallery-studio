package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	"github.com/fatballfish/pic-gallery/internal/service/oauthserver"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

// OAuth owns only Studio OAuth routes; browser credentials never become MCP tokens.
type OAuth struct {
	service *oauthserver.Service
	auth    *authservice.Service
}

func NewOAuth(s *oauthserver.Service, a *authservice.Service) *OAuth { return &OAuth{s, a} }
func oauthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func oauthError(w http.ResponseWriter, status int, code string) {
	oauthJSON(w, status, map[string]string{"error": code})
}
func oauthFailure(w http.ResponseWriter, err error, invalid string) {
	var rate *oauthserver.RateLimitError
	switch {
	case errors.As(err, &rate):
		seconds := max(1, int(math.Ceil(float64(rate.RetryAfter)/float64(time.Second))))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		oauthError(w, 429, "temporarily_unavailable")
	case errors.Is(err, oauthserver.ErrRegistrationDisabled):
		oauthError(w, 403, "access_denied")
	case errors.Is(err, oauthserver.ErrInvalidScope):
		oauthError(w, 400, "invalid_scope")
	case errors.Is(err, oauthserver.ErrInvalidTarget):
		oauthError(w, 400, "invalid_target")
	case errors.Is(err, oauthserver.ErrUnsupportedGrant):
		oauthError(w, 400, "unsupported_grant_type")
	case errors.Is(err, oauthserver.ErrAuthorizationExpired):
		oauthError(w, 400, "authorization_request_expired")
	case errors.Is(err, oauthserver.ErrInvalidRequest):
		oauthError(w, 400, "invalid_request")
	case errors.Is(err, oauthserver.ErrInvalid), errors.Is(err, oauthserver.ErrNotFound):
		oauthError(w, 400, invalid)
	default:
		oauthError(w, 503, "temporarily_unavailable")
	}
}
func (h *OAuth) Metadata(w http.ResponseWriter, r *http.Request) {
	oauthJSON(w, 200, h.service.Metadata())
}
func (h *OAuth) ProtectedMetadata(w http.ResponseWriter, r *http.Request) {
	oauthJSON(w, 200, h.service.ProtectedMetadata())
}
func (h *OAuth) Authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		oauthError(w, 405, "invalid_request")
		return
	}
	id, e := h.service.Begin(r.Context(), r.URL.Query())
	if e != nil {
		oauthFailure(w, e, "invalid_request")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, h.service.ConsentURL(id), http.StatusSeeOther)
}
func (h *OAuth) browserUser(r *http.Request) (int64, bool) {
	a := r.Header.Get("Authorization")
	if !strings.HasPrefix(a, "Bearer ") {
		return 0, false
	}
	claims, e := h.auth.ParseAccessToken(strings.TrimPrefix(a, "Bearer "))
	if e != nil {
		return 0, false
	}
	u, ok := h.auth.GetUserByID(claims.UserID)
	return u.ID, ok && u.Status == "active" && u.TokenVersion == claims.TokenVersion
}
func oauthBrowserError(w http.ResponseWriter, r *http.Request) {
	code := "invalid_session"
	if r.Header.Get("Authorization") == "" {
		code = "login_required"
	}
	oauthError(w, http.StatusUnauthorized, code)
}
func (h *OAuth) Consent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	uid, ok := h.browserUser(r)
	if !ok {
		oauthBrowserError(w, r)
		return
	}
	if r.Method == "GET" {
		a, csrf, e := h.service.Consent(r.Context(), r.URL.Query().Get("request_id"), uid)
		if e != nil {
			oauthFailure(w, e, "invalid_request")
			return
		}
		httpx.WriteSuccess(w, r, 200, map[string]any{"request_id": a.ID, "client": a.Client, "resource": a.Resource, "scopes": a.Scopes, "csrf_token": csrf})
		return
	}
	if r.Method != "POST" {
		oauthError(w, 405, "invalid_request")
		return
	}
	var body struct {
		RequestID string `json:"request_id"`
		CSRF      string `json:"csrf_token"`
		Allow     bool   `json:"allow"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	redirect, e := h.service.Decide(r.Context(), body.RequestID, uid, body.CSRF, body.Allow)
	if e != nil {
		oauthFailure(w, e, "invalid_request")
		return
	}
	httpx.WriteSuccess(w, r, 200, map[string]string{"redirect_uri": redirect})
}
func oauthForm(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		oauthError(w, 400, "invalid_request")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if r.ParseForm() != nil || len(r.URL.RawQuery) > 0 {
		oauthError(w, 400, "invalid_request")
		return nil, false
	}
	return r.PostForm, true
}
func (h *OAuth) Token(w http.ResponseWriter, r *http.Request) {
	q, ok := oauthForm(w, r)
	if !ok {
		return
	}
	if r.Header.Get("Authorization") != "" {
		oauthError(w, 401, "invalid_client")
		return
	}
	v, e := h.service.Token(r.Context(), q)
	if e != nil {
		oauthFailure(w, e, "invalid_grant")
		return
	}
	oauthJSON(w, 200, v)
}
func (h *OAuth) Revoke(w http.ResponseWriter, r *http.Request) {
	q, ok := oauthForm(w, r)
	if !ok {
		return
	}
	if q.Get("client_id") == "" || len(q["token"]) != 1 || len(q["client_id"]) != 1 {
		oauthError(w, 400, "invalid_request")
		return
	}
	if e := h.service.Revoke(r.Context(), q.Get("token"), q.Get("client_id")); e != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	oauthJSON(w, 200, map[string]any{})
}
func (h *OAuth) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		oauthError(w, 405, "invalid_request")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var c oauthserver.Client
	decoder := json.NewDecoder(r.Body)
	if decoder.Decode(&c) != nil || decoder.Decode(new(any)) != io.EOF {
		oauthError(w, 400, "invalid_client_metadata")
		return
	}
	c, e := h.service.Register(r.Context(), c)
	if e != nil {
		oauthFailure(w, e, "invalid_client_metadata")
		return
	}
	oauthJSON(w, 201, c)
}
func (h *OAuth) ConnectedApps(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	uid, ok := h.browserUser(r)
	if !ok {
		oauthBrowserError(w, r)
		return
	}
	if r.Method == "GET" {
		grants, e := h.service.Grants(r.Context(), uid)
		if e != nil {
			oauthFailure(w, e, "invalid_request")
			return
		}
		httpx.WriteSuccess(w, r, 200, map[string]any{"items": grants})
		return
	}
	if r.Method != "DELETE" {
		oauthError(w, 405, "invalid_request")
		return
	}
	if e := h.service.Disconnect(r.Context(), uid, r.URL.Query().Get("grant_id")); e != nil {
		oauthFailure(w, e, "invalid_request")
		return
	}
	httpx.WriteSuccess(w, r, 200, map[string]bool{"revoked": true})
}
