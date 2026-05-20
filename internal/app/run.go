package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	apphttp "github.com/fatballfish/pic-gallery/internal/http/router"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
)

func Run() error {
	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	if err := validateStorageTopology(cfg); err != nil {
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

	authSvc := authservice.NewServiceWithStore(cfg.Auth, cfg.Billing.UserGroupMultipliers, entstore.NewAuthStore(client))
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, entstore.NewBillingStore(client, cfg.Billing.PointsScale))
	assetSvc := assetservice.NewServiceWithStore(cfg.Storage, cfg.GenerationLimits, entstore.NewAssetsStore(client))
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, entstore.NewImageTaskStore(client), assetSvc, billingSvc)
	adminSvc := adminconfigservice.NewServiceWithStore(cfg, entstore.NewAdminConfigStore(client))
	apiKeySvc := apikeyservice.NewService(entstore.NewAPIKeyStore(client))
	slog.Info("database-backed stores enabled")

	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, adminSvc, billingSvc, apiKeySvc)

	srv := &http.Server{
		Addr:              cfg.App.Addr,
		Handler:           apphttp.NewWithAPI(api),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("starting pic-gallery api", "name", cfg.App.Name, "env", cfg.App.Env, "addr", cfg.App.Addr)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
