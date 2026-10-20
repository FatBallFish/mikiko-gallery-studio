package handlers

import (
	"context"
	"net/http"

	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainsecureconfig "github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const featureConfigTab = "site"

type featureFlagSource interface {
	GetTab(context.Context, string) (domainadminconfig.Tab, error)
}

type oidcSettingsSource interface {
	ResolveOIDCSettings(ctx context.Context) (domainsecureconfig.OIDCPublicSettings, error)
}

type featureOIDCLogin struct {
	Enabled     bool   `json:"enabled"`
	DisplayName string `json:"display_name,omitempty"`
}

type featureFlags struct {
	VideoCreation       bool             `json:"video_creation"`
	CreativeCanvas      bool             `json:"creative_canvas"`
	MediaUpload         bool             `json:"media_upload"`
	OIDCLogin           featureOIDCLogin `json:"oidc_login"`
	PasswordLoginEnabled bool            `json:"password_login_enabled"`
	RegistrationEnabled bool            `json:"registration_enabled"`
}

type featureFlagResolver struct {
	source     featureFlagSource
	oidcSource oidcSettingsSource
}

func newFeatureFlagResolver(source featureFlagSource, oidcSource oidcSettingsSource) *featureFlagResolver {
	return &featureFlagResolver{source: source, oidcSource: oidcSource}
}

func (r *featureFlagResolver) Get(ctx context.Context) (featureFlags, error) {
	// Local login stays fully open unless an OIDC policy explicitly narrows it.
	result := featureFlags{PasswordLoginEnabled: true, RegistrationEnabled: true}
	if r == nil || r.source == nil {
		return result, nil
	}
	tab, err := r.source.GetTab(ctx, featureConfigTab)
	if err != nil {
		return featureFlags{}, err
	}
	for _, item := range tab.Items {
		enabled, ok := item.ConfigValue["value"].(bool)
		if !ok || !enabled {
			continue
		}
		switch item.ConfigKey {
		case "video_creation":
			result.VideoCreation = true
		case "creative_canvas":
			result.CreativeCanvas = true
		case "media_upload":
			result.MediaUpload = true
		}
	}
	if r.oidcSource != nil {
		settings, err := r.oidcSource.ResolveOIDCSettings(ctx)
		if err != nil {
			return featureFlags{}, err
		}
		result.OIDCLogin = featureOIDCLogin{Enabled: settings.Enabled, DisplayName: settings.DisplayName}
		result.PasswordLoginEnabled = settings.AllowLocalLogin
		result.RegistrationEnabled = settings.AllowRegistration
	}
	return result, nil
}

func featureDisabled(name string) *errs.Error {
	return errs.New(403, "FEATURE_DISABLED", name+" is not enabled")
}

func featureWriteBlocked(feature, method, action string) bool {
	switch feature {
	case "video_creation":
		return method == http.MethodPost && (action == "estimate" || action == "create")
	case "media_upload":
		return method == http.MethodPost && action == "init"
	case "creative_canvas":
		if method == http.MethodGet || action == "attach-results" {
			return false
		}
		return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
	default:
		return false
	}
}

func (a *API) featureFlags(ctx context.Context) (featureFlags, error) {
	return newFeatureFlagResolver(a.admin, a.secureCfg).Get(ctx)
}

func (a *API) requireFeature(ctx context.Context, name string) *errs.Error {
	flags, err := a.featureFlags(ctx)
	if err != nil {
		return normalizeAppError(err)
	}
	enabled := name == "video_creation" && flags.VideoCreation || name == "creative_canvas" && flags.CreativeCanvas || name == "media_upload" && flags.MediaUpload
	if !enabled {
		return featureDisabled(name)
	}
	return nil
}

func (a *API) HandleAgentFeatures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	flags, err := a.featureFlags(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, flags)
}
