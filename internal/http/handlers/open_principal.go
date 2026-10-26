package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// GetOpenPrincipal authenticates a developer key against its current account and
// optionally requires scopes before an adapter invokes a shared operation.
func (a *API) GetOpenPrincipal(r *http.Request, scopes ...string) (openplatform.Principal, *errs.Error) {
	identity, err := a.authenticateOpenBearer(r, scopes...)
	if err != nil {
		return openplatform.Principal{}, err
	}
	key, keyErr := a.apiKeys.GetByID(r.Context(), identity.UserID, identity.APIKeyID)
	if keyErr != nil {
		return openplatform.Principal{}, normalizeAppError(keyErr)
	}
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || subtle.ConstantTimeCompare([]byte(key.SecretHash), []byte(domainapikey.HashSecret(fields[1]))) != 1 {
		return openplatform.Principal{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "API key credential changed")
	}
	generation := sha256.Sum256([]byte(key.SecretHash))
	return openplatform.Principal{UserID: identity.UserID, APIKeyID: strconv.FormatInt(identity.APIKeyID, 10), CredentialVersion: hex.EncodeToString(generation[:]), Scopes: identity.Scopes, GroupCode: identity.GroupCode}, nil
}

func (a *API) authenticateOpenBearer(r *http.Request, scopes ...string) (domainapikey.Identity, *errs.Error) {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || !strings.HasPrefix(fields[1], "sk-") {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing or invalid bearer API key")
	}
	identity, err := a.apiKeys.AuthenticateBearer(r.Context(), fields[1])
	if err != nil {
		return domainapikey.Identity{}, normalizeAppError(err)
	}
	if appErr := a.requireAPIKeyUserActive(identity); appErr != nil {
		return domainapikey.Identity{}, appErr
	}
	// Resolve the current account group rather than stale key creation metadata.
	if a.auth != nil {
		if user, ok := a.auth.GetUserByID(identity.UserID); ok {
			identity.GroupCode = user.GroupCode
		}
	}
	for _, scope := range scopes {
		if !domainapikey.HasScope(identity.Scopes, scope) {
			return domainapikey.Identity{}, errs.New(http.StatusForbidden, errs.CodeForbidden, "api key requires scope: "+scope)
		}
	}
	return identity, nil
}
