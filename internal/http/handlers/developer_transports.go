package handlers

import (
	"net/http"
	"strings"

	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

// HandleDeveloperEndpoints projects only explicit trusted runtime configuration.
func (a *API) HandleDeveloperEndpoints(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimRight(a.cfg.Runtime.PublicAPIURL, "/")
	data := map[string]any{
		"canonical_configured": origin != "", "api_origin": origin,
		"rest_base": "", "openai_base": "", "mcp_url": "", "disabled_reason": "CANONICAL_URL_UNCONFIGURED",
		"authorization_endpoint": "", "token_endpoint": "", "registration_endpoint": "",
		"protected_resource_metadata_url": "", "web_url": "",
	}
	if origin != "" {
		data["disabled_reason"] = ""
		for field, suffix := range map[string]string{
			"rest_base": "/api/open/v1", "openai_base": "/v1", "mcp_url": "/mcp",
			"authorization_endpoint": "/oauth/authorize", "token_endpoint": "/oauth/token", "registration_endpoint": "/oauth/register",
			"protected_resource_metadata_url": "/.well-known/oauth-protected-resource/mcp", "web_url": "",
		} {
			data[field] = origin + suffix
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, r, http.StatusOK, data)
}

// SetDeveloperTransports installs independently authenticated developer adapters.
func (a *API) SetDeveloperTransports(open, oauth, mcp, assetAccess http.Handler) {
	a.developerOpen, a.developerOAuth, a.developerMCP, a.developerAssetAccess = open, oauth, mcp, assetAccess
}

func developerTransport(w http.ResponseWriter, r *http.Request, h http.Handler) {
	if h == nil {
		APINotFound(w, r)
		return
	}
	h.ServeHTTP(w, r)
}
func (a *API) HandleDeveloperOpen(w http.ResponseWriter, r *http.Request) {
	developerTransport(w, r, a.developerOpen)
}
func (a *API) HandleDeveloperOAuth(w http.ResponseWriter, r *http.Request) {
	developerTransport(w, r, a.developerOAuth)
}
func (a *API) HandleDeveloperMCP(w http.ResponseWriter, r *http.Request) {
	developerTransport(w, r, a.developerMCP)
}
func (a *API) HandleDeveloperAssetAccess(w http.ResponseWriter, r *http.Request) {
	developerTransport(w, r, a.developerAssetAccess)
}
