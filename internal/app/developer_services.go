package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/mcpserver"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	"github.com/fatballfish/pic-gallery/internal/service/assetaccess"
	"github.com/fatballfish/pic-gallery/internal/service/assetgroup"
	"github.com/fatballfish/pic-gallery/internal/service/assets"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	"github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/oauthserver"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/service/uploadaccess"
	"github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func configureDeveloperServices(cfg config.Config, client *repoent.Client, api *handlers.API, auth *authservice.Service, keys *apikeyservice.Service, bills *billing.Service, images *imagetask.Service, refs *assets.Service, media *mediaasset.Service, projects *project.Service, videoModels *videorouting.Service, videos *videotask.Service) error {
	keys.SetUsageReader(bills)
	images.SetAPIKeyUsageManager(keys)
	oauth, err := oauthserver.New(entstore.NewOAuthStore(client), oauthserver.Config{
		PublicAPIURL: cfg.Runtime.PublicAPIURL, Scopes: domainapikey.SupportedScopes(), EnableRegistration: true,
		User: func(ctx context.Context, id int64) (oauthserver.User, error) {
			u, e := entstore.NewAuthStore(client).GetUserByID(ctx, id)
			if e != nil {
				return oauthserver.User{}, e
			}
			return oauthserver.User{ID: u.ID, GroupCode: u.GroupCode, Active: string(u.Status) == "active"}, nil
		},
	})
	if err != nil {
		return fmt.Errorf("configure OAuth: %w", err)
	}
	validate := func(ctx context.Context, p openplatform.Principal) error {
		if p.APIKeyID != "" {
			id, e := strconv.ParseInt(p.APIKeyID, 10, 64)
			if e != nil {
				return errs.New(401, errs.CodeUnauthorized, "invalid API key identity")
			}
			key, e := keys.GetByID(ctx, p.UserID, id)
			if e != nil {
				return e
			}
			if string(key.Status) != "active" || key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now()) {
				return errs.New(401, errs.CodeUnauthorized, "API key unavailable")
			}
			user, userErr := entstore.NewAuthStore(client).GetUserByID(ctx, p.UserID)
			if userErr != nil || string(user.Status) != "active" {
				return errs.New(401, errs.CodeUnauthorized, "account unavailable")
			}
			generation := sha256.Sum256([]byte(key.SecretHash))
			if p.CredentialVersion == "" || subtle.ConstantTimeCompare([]byte(p.CredentialVersion), []byte(hex.EncodeToString(generation[:]))) != 1 {
				return errs.New(401, errs.CodeUnauthorized, "API key credential changed")
			}
			for _, scope := range p.Scopes {
				if !domainapikey.HasScope(key.Scopes, scope) {
					return errs.New(403, errs.CodeForbidden, "API key permission revoked")
				}
			}
			return nil
		}
		return oauth.ValidateGrant(ctx, p.UserID, p.ClientID, p.GrantID, p.Scopes)
	}
	if len(cfg.Auth.AccessTokenSecret) < 32 || strings.Contains(cfg.Auth.AccessTokenSecret, "local-dev") {
		return fmt.Errorf("developer capability signing requires a nonpublic access-token secret of at least 32 bytes")
	}
	accessKey := sha256.Sum256([]byte("developer-asset-access:" + cfg.Auth.AccessTokenSecret))
	access, err := assetaccess.New(media, assetaccess.Options{PublicAPIURL: cfg.Runtime.PublicAPIURL, SigningKey: accessKey[:], ValidatePrincipal: validate})
	if err != nil {
		return fmt.Errorf("configure asset access: %w", err)
	}
	uploadKey := sha256.Sum256([]byte("developer-upload-access:" + cfg.Auth.AccessTokenSecret))
	uploads, err := uploadaccess.New(media, uploadaccess.Options{PublicAPIURL: cfg.Runtime.PublicAPIURL, SigningKey: uploadKey[:], ValidatePrincipal: validate})
	if err != nil {
		return fmt.Errorf("configure upload access: %w", err)
	}
	modelCaps := capabilities.NewService(cfg)
	modelCaps.SetModelRoutingSource(entstore.NewModelAdminStore(client))
	quoteKey := sha256.Sum256([]byte("developer-preflight:" + cfg.Auth.AccessTokenSecret))
	shared := openplatform.New(openplatform.Dependencies{Auth: auth, Billing: bills, Images: images, ImageModels: modelCaps, Videos: videos, VideoModels: videoModels, Assets: media, UploadTarget: uploads.Target, AssetAccess: access.Access, References: refs, Projects: projects, APIKeys: keys, Groups: assetgroup.NewService(entstore.NewAssetGroupStore(client)), QuoteKey: quoteKey[:], ValidatePrincipal: validate})
	actor := func(r *http.Request) (openplatform.Principal, error) {
		p, e := api.GetOpenPrincipal(r)
		if e != nil {
			return p, e
		}
		return p, nil
	}
	native := handlers.NewOpenPlatform(shared, actor)
	native.SetUploadAccess(handlers.NewUploadAccessHandler(uploads))
	oauthHTTP := handlers.NewOAuth(oauth, auth)
	oauthMux := http.NewServeMux()
	oauthMux.HandleFunc("/.well-known/oauth-authorization-server", oauthHTTP.Metadata)
	oauthMux.HandleFunc("/.well-known/oauth-protected-resource", oauthHTTP.ProtectedMetadata)
	oauthMux.HandleFunc("/.well-known/oauth-protected-resource/mcp", oauthHTTP.ProtectedMetadata)
	oauthMux.HandleFunc("/oauth/authorize", oauthHTTP.Authorize)
	oauthMux.HandleFunc("/oauth/token", oauthHTTP.Token)
	oauthMux.HandleFunc("/oauth/revoke", oauthHTTP.Revoke)
	oauthMux.HandleFunc("/oauth/register", oauthHTTP.Register)
	oauthMux.HandleFunc("/api/v1/oauth/consent", oauthHTTP.Consent)
	oauthMux.HandleFunc("/api/v1/oauth/connected-apps", oauthHTTP.ConnectedApps)
	mcp, err := mcpserver.NewHandler(mcpserver.Config{Service: shared, ResourceURL: oauth.Resource(), ResourceMetadataURL: strings.TrimRight(cfg.Runtime.PublicAPIURL, "/") + "/.well-known/oauth-protected-resource/mcp", AllowedOrigins: cfg.HTTP.CORSAllowedOrigins, Validator: mcpserver.ValidateAccessFunc(func(ctx context.Context, token string) (openplatform.Principal, error) {
		p, e := oauth.ValidateAccess(ctx, token)
		if e != nil {
			return openplatform.Principal{}, e
		}
		return openplatform.Principal{UserID: p.UserID, ClientID: p.ClientID, GrantID: p.GrantID, Scopes: p.Scopes, GroupCode: p.GroupCode}, nil
	})})
	if err != nil {
		return fmt.Errorf("configure MCP: %w", err)
	}
	api.SetCompatOperations(shared)
	api.SetDeveloperTransports(native, oauthMux, mcp, handlers.NewAssetAccessGrantHandler(access))
	return nil
}
