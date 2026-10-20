package secureconfig

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
	"github.com/fatballfish/pic-gallery/internal/service/secretcodec"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	oidcCategory = "oidc"
	oidcKey      = "default"
)

// DefaultOIDCPublicSettings keeps every legacy login path open when the OIDC
// record has never been configured.
func DefaultOIDCPublicSettings() secureconfig.OIDCPublicSettings {
	return secureconfig.OIDCPublicSettings{AllowLocalLogin: true, AllowRegistration: true}
}

func (s *Service) GetOIDCConfig(ctx context.Context) (secureconfig.OIDCConfigView, error) {
	record, ok, err := s.getOIDCRecord(ctx)
	if err != nil {
		return secureconfig.OIDCConfigView{}, err
	}
	if !ok {
		return defaultOIDCConfigView(), nil
	}
	return oidcViewFromPublic(record.PublicValue, secretStatus(record), record.Version, record.UpdatedAt), nil
}

func (s *Service) UpdateOIDCConfig(ctx context.Context, req secureconfig.UpdateOIDCConfigRequest) (secureconfig.OIDCConfigView, error) {
	if s == nil || s.store == nil {
		return secureconfig.OIDCConfigView{}, errs.Internal("secure config store is not available")
	}
	current, exists, err := s.getOIDCRecord(ctx)
	if err != nil {
		return secureconfig.OIDCConfigView{}, err
	}
	if exists && req.Version > 0 && req.Version != current.Version {
		return secureconfig.OIDCConfigView{}, errs.New(409, errs.CodeConflict, "oidc config version conflict")
	}
	public := oidcPublicValue(req)
	secrets, err := s.oidcSecretsForWrite(ctx, current, req)
	if err != nil {
		return secureconfig.OIDCConfigView{}, err
	}
	if err := validateOIDCWrite(req, secrets); err != nil {
		return secureconfig.OIDCConfigView{}, err
	}
	encrypted, err := s.codec.EncryptJSON(secrets)
	if err != nil {
		return secureconfig.OIDCConfigView{}, err
	}
	secretFields := sortedSecretFields(secrets)
	record := Record{
		Category:          oidcCategory,
		Key:               oidcKey,
		PublicValue:       public,
		SecretEncrypted:   encrypted,
		SecretFingerprint: secretcodec.Fingerprint(secrets, secretFields),
		SecretFields:      secretFields,
		Version:           1,
		UpdatedBy:         req.UpdatedBy,
	}
	if exists {
		record.Version = current.Version + 1
	}
	saved, err := s.store.Save(ctx, record)
	if err != nil {
		return secureconfig.OIDCConfigView{}, err
	}
	return oidcViewFromPublic(saved.PublicValue, secretStatus(saved), saved.Version, saved.UpdatedAt), nil
}

// ResolveOIDCSettings returns the login-mode switches. Missing or unreadable
// configuration resolves to the permissive defaults so local login keeps
// working when OIDC has never been set up.
func (s *Service) ResolveOIDCSettings(ctx context.Context) (secureconfig.OIDCPublicSettings, error) {
	settings := DefaultOIDCPublicSettings()
	record, ok, err := s.getOIDCRecord(ctx)
	if err != nil || !ok {
		return settings, err
	}
	if displayName, _ := record.PublicValue["display_name"].(string); strings.TrimSpace(displayName) != "" {
		settings.DisplayName = strings.TrimSpace(displayName)
	}
	if value, exists := record.PublicValue["allow_local_login"]; exists {
		settings.AllowLocalLogin = boolFromAny(value)
	}
	if value, exists := record.PublicValue["allow_registration"]; exists {
		settings.AllowRegistration = boolFromAny(value)
	}
	settings.Enabled = boolFromAny(record.PublicValue["enabled"])
	return settings, nil
}

// LocalAuthPolicy implements auth.LocalAuthPolicyResolver.
func (s *Service) LocalAuthPolicy(ctx context.Context) (bool, bool, error) {
	settings, err := s.ResolveOIDCSettings(ctx)
	if err != nil {
		return true, true, err
	}
	return settings.AllowLocalLogin, settings.AllowRegistration, nil
}

// ResolveOIDCConfig returns the runnable provider configuration (with the
// decrypted client secret) when OIDC login is enabled and fully configured.
func (s *Service) ResolveOIDCConfig(ctx context.Context) (secureconfig.OIDCRuntimeConfig, bool, error) {
	record, ok, err := s.getOIDCRecord(ctx)
	if err != nil || !ok {
		return secureconfig.OIDCRuntimeConfig{}, false, err
	}
	if !boolFromAny(record.PublicValue["enabled"]) {
		return secureconfig.OIDCRuntimeConfig{}, false, nil
	}
	cfg := secureconfig.OIDCRuntimeConfig{
		IssuerURL:   strings.TrimSpace(fmt.Sprint(record.PublicValue["issuer_url"])),
		ClientID:    strings.TrimSpace(fmt.Sprint(record.PublicValue["client_id"])),
		DisplayName: strings.TrimSpace(fmt.Sprint(record.PublicValue["display_name"])),
	}
	if cfg.IssuerURL == "" || cfg.ClientID == "" || len(record.SecretEncrypted) == 0 {
		return secureconfig.OIDCRuntimeConfig{}, false, nil
	}
	secrets, err := s.codec.DecryptJSON(record.SecretEncrypted)
	if err != nil {
		return secureconfig.OIDCRuntimeConfig{}, false, err
	}
	cfg.ClientSecret = secretString(secrets["client_secret"])
	if cfg.ClientSecret == "" {
		return secureconfig.OIDCRuntimeConfig{}, false, nil
	}
	return cfg, true, nil
}

func (s *Service) getOIDCRecord(ctx context.Context) (Record, bool, error) {
	if s == nil || s.store == nil {
		return Record{}, false, nil
	}
	return s.store.Get(ctx, oidcCategory, oidcKey)
}

func (s *Service) oidcSecretsForWrite(ctx context.Context, current Record, req secureconfig.UpdateOIDCConfigRequest) (map[string]any, error) {
	secrets := map[string]any{}
	if len(current.SecretEncrypted) > 0 {
		decoded, err := s.codec.DecryptJSON(current.SecretEncrypted)
		if err != nil {
			return nil, err
		}
		for key, value := range decoded {
			secrets[key] = value
		}
	}
	for _, key := range req.ClearSecrets {
		if strings.EqualFold(strings.TrimSpace(key), "client_secret") {
			delete(secrets, "client_secret")
		}
	}
	if req.Secrets != nil {
		if secret, ok := req.Secrets["client_secret"]; ok {
			secret = strings.TrimSpace(secret)
			if secret == "" {
				return nil, errs.BadRequest("oidc client secret cannot be empty; use clear_secrets to remove it")
			}
			if isMaskedSecretPlaceholder(secret) {
				return nil, errs.New(400, "INVALID_SECRET_PLACEHOLDER", "masked secret placeholder is not allowed")
			}
			secrets["client_secret"] = secret
		}
	}
	return secrets, nil
}

func validateOIDCWrite(req secureconfig.UpdateOIDCConfigRequest, secrets map[string]any) *errs.Error {
	if !req.Enabled {
		if !req.AllowLocalLogin {
			return errs.BadRequest("local login cannot be disabled while oidc login is disabled")
		}
		return nil
	}
	if strings.TrimSpace(req.IssuerURL) == "" || strings.TrimSpace(req.ClientID) == "" {
		return errs.BadRequest("oidc issuer_url and client_id are required when oidc is enabled")
	}
	if err := validateOIDCIssuerURL(req.IssuerURL); err != nil {
		return err
	}
	if secretString(secrets["client_secret"]) == "" {
		return errs.BadRequest("oidc client_secret is required when oidc is enabled")
	}
	return nil
}

// secretString renders a decrypted secret value; a missing entry is an empty
// string (fmt.Sprint of a nil entry would yield "<nil>").
func secretString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func validateOIDCIssuerURL(raw string) *errs.Error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return errs.BadRequest("oidc issuer_url must be a valid absolute url")
	}
	if parsed.Fragment != "" || parsed.RawQuery != "" {
		return errs.BadRequest("oidc issuer_url must not contain a query or fragment")
	}
	https := strings.EqualFold(parsed.Scheme, "https")
	localHTTP := strings.EqualFold(parsed.Scheme, "http") && (strings.EqualFold(parsed.Hostname(), "localhost") || strings.EqualFold(parsed.Hostname(), "127.0.0.1"))
	if !https && !localHTTP {
		return errs.BadRequest("oidc issuer_url must use https (http is only allowed for localhost)")
	}
	return nil
}

func oidcPublicValue(req secureconfig.UpdateOIDCConfigRequest) map[string]any {
	return map[string]any{
		"enabled":            req.Enabled,
		"issuer_url":         strings.TrimSpace(req.IssuerURL),
		"client_id":          strings.TrimSpace(req.ClientID),
		"display_name":       strings.TrimSpace(req.DisplayName),
		"allow_local_login":  req.AllowLocalLogin,
		"allow_registration": req.AllowRegistration,
	}
}

func defaultOIDCConfigView() secureconfig.OIDCConfigView {
	return secureconfig.OIDCConfigView{
		AllowLocalLogin:   true,
		AllowRegistration: true,
	}
}

func oidcViewFromPublic(public map[string]any, status secureconfig.SecretStatus, version int64, updatedAt time.Time) secureconfig.OIDCConfigView {
	view := defaultOIDCConfigView()
	view.Enabled = boolFromAny(public["enabled"])
	view.IssuerURL = strings.TrimSpace(fmt.Sprint(public["issuer_url"]))
	view.ClientID = strings.TrimSpace(fmt.Sprint(public["client_id"]))
	view.DisplayName = strings.TrimSpace(fmt.Sprint(public["display_name"]))
	if value, exists := public["allow_local_login"]; exists {
		view.AllowLocalLogin = boolFromAny(value)
	}
	if value, exists := public["allow_registration"]; exists {
		view.AllowRegistration = boolFromAny(value)
	}
	view.SecretStatus = status
	view.Version = version
	view.UpdatedAt = updatedAt
	return view
}
