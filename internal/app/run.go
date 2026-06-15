package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	apphttp "github.com/fatballfish/pic-gallery/internal/http/router"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	admincallrecordservice "github.com/fatballfish/pic-gallery/internal/service/admincallrecord"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	adminuserservice "github.com/fatballfish/pic-gallery/internal/service/adminuser"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
	redeemservice "github.com/fatballfish/pic-gallery/internal/service/redeem"
	secureconfigservice "github.com/fatballfish/pic-gallery/internal/service/secureconfig"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func seedDefaultAdmin(ctx context.Context, cfg config.AdminConfig, store *entstore.AdminAuthStore) {
	email := strings.TrimSpace(cfg.SeedEmail)
	password := cfg.SeedPassword
	if email == "" || password == "" {
		return
	}
	if _, err := store.GetAdminByEmail(ctx, email); err == nil {
		return
	}
	_, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        email,
		PasswordHash: adminauthservice.HashPassword(password),
		Role:         defaultAdminSeedRole(cfg.SeedRole),
		Status:       "active",
	})
	if err != nil {
		slog.Warn("failed to seed default admin", "err", err)
	}
}

func defaultAdminSeedRole(value string) string {
	role := strings.ToLower(strings.TrimSpace(value))
	switch role {
	case "":
		return domainadminauth.RoleAdmin
	case domainadminauth.RoleAdmin, domainadminauth.RoleSuperAdmin:
		return role
	default:
		slog.Warn("invalid admin.seed_role, falling back to admin", "role", role)
		return domainadminauth.RoleAdmin
	}
}

func Run() error {
	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	if err := validateStorageTopology(cfg); err != nil {
		return err
	}
	if err := authservice.ValidateProductionEmailCodeConfig(cfg.App.Env, cfg.Auth); err != nil {
		return err
	}
	if err := validateSecureConfigEncryptionKey(cfg); err != nil {
		return err
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	client, err := db.Open(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer client.Close()
	if err := client.Schema.Create(context.Background()); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	storageBackend, err := storage.NewBackend(cfg.Storage)
	if err != nil {
		return fmt.Errorf("init storage backend: %w", err)
	}

	redisClient, allowRedisFallback, err := newRedisClient(context.Background(), cfg)
	if err != nil {
		return err
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	var authRedisRuntime authservice.RedisRuntime
	if redisClient != nil {
		authRedisRuntime = authservice.NewRedisRuntime(redisClient, cfg.Redis.KeyPrefix)
	}

	secureConfigSvc := secureconfigservice.NewService(entstore.NewSecureConfigStore(client), cfg.Security.SecureConfigEncryptionKey, cfg.Auth.SMTP, cfg.App.Env)
	authSvc := authservice.NewServiceWithStoreAndRedis(cfg.Auth, cfg.Billing.UserGroupMultipliers, entstore.NewAuthStore(client), authRedisRuntime, allowRedisFallback)
	authSvc.SetSMTPConfigResolver(secureConfigSvc)
	billingStore := entstore.NewBillingStore(client, cfg.Billing.PointsScale)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	assetSvc := assetservice.NewServiceWithStoreAndBackend(cfg.Storage, cfg.GenerationLimits, entstore.NewAssetsStore(client), storageBackend)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, nil, entstore.NewImageTaskStore(client), assetSvc, billingSvc, storageBackend)
	modelAdminStore := entstore.NewModelAdminStore(client)
	taskSvc.SetModelRoutingSource(modelAdminStore)
	adminSvc := adminconfigservice.NewServiceWithStore(cfg, entstore.NewAdminConfigStore(client))
	apiKeySvc, err := newRuntimeAPIKeyService(cfg, entstore.NewAPIKeyStore(client))
	if err != nil {
		return err
	}
	adminStore := entstore.NewAdminAuthStore(client)
	seedDefaultAdmin(context.Background(), cfg.Admin, adminStore)
	adminAuthSvc := adminauthservice.NewService(cfg.Auth, adminStore)
	auditSvc := auditservice.NewService(entstore.NewAuditStore(client))
	adminUserSvc := adminuserservice.NewServiceWithStore(entstore.NewAdminUserStore(client, billingStore), billingSvc)
	redeemSvc := redeemservice.NewServiceWithStore(entstore.NewRedeemAdminStore(client))
	callRecordSvc := admincallrecordservice.NewServiceWithStore(entstore.NewAdminCallRecordStore(client))
	modelAdminSvc := modeladminservice.NewServiceWithStore(modelAdminStore)
	slog.Info("database-backed stores enabled")

	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, assetSvc, taskSvc, adminSvc, billingSvc, apiKeySvc, adminAuthSvc, auditSvc, adminUserSvc, redeemSvc, callRecordSvc, modelAdminSvc)
	api.SetCashierProviderInstanceStore(entstore.NewCashierStoreWithConfigEncryptionKey(client, cfg.Cashier.ProviderConfigEncryptionKey))
	api.SetSecureConfigService(secureConfigSvc)

	srv := &http.Server{
		Addr:              cfg.App.Addr,
		Handler:           apphttp.NewWithAPIAndConfig(api, cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("starting pic-gallery api", "name", cfg.App.Name, "env", cfg.App.Env, "addr", cfg.App.Addr)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func validateSecureConfigEncryptionKey(cfg config.Config) error {
	key := strings.TrimSpace(cfg.Security.SecureConfigEncryptionKey)
	if isProductionEnv(cfg.App.Env) && isWeakAPIKeySigningSecretEncryptionKey(key) {
		return fmt.Errorf("secure config encryption key must be set to a non-development value in %s env", cfg.App.Env)
	}
	return nil
}

func newRuntimeAPIKeyService(cfg config.Config, store apikeyservice.Store) (*apikeyservice.Service, error) {
	signingSecretEncryptionKey := strings.TrimSpace(cfg.APIKey.SigningSecretEncryptionKey)
	if isProductionEnv(cfg.App.Env) && isWeakAPIKeySigningSecretEncryptionKey(signingSecretEncryptionKey) {
		return nil, fmt.Errorf("api key signing secret encryption key must be set to a non-development value in %s env", cfg.App.Env)
	}
	return apikeyservice.NewServiceWithSigningSecretKey(store, signingSecretEncryptionKey), nil
}

func isProductionEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func isWeakAPIKeySigningSecretEncryptionKey(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "secret", "password", "admin", "admin-token-secret", "admin-secret":
		return true
	default:
		return strings.HasPrefix(value, "change-me") || strings.HasPrefix(value, "local-dev") || strings.Contains(value, "example") || strings.Contains(value, "sample") || len(value) < 32
	}
}
