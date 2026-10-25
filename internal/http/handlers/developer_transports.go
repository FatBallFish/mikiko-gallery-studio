package handlers

import "net/http"

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
