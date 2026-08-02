package entstore_test

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminConfigStorePersistsTabOverrides(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:configstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewAdminConfigStore(client)
	svc := adminconfig.NewServiceWithStore(testAdminConfig(), store)

	updated, err := svc.UpdateTab(ctx, domainadminconfig.UpdateTabRequest{
		TabKey:  "billing_pricing",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_pricing",
			ConfigKey:      "cny_per_point",
			ConfigValue:    map[string]any{"value": "0.5000"},
			Scope:          "global",
		}},
	})
	if err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}

	reloaded := adminconfig.NewServiceWithStore(testAdminConfig(), store)
	tab, err := reloaded.GetTab(ctx, "billing_pricing")
	if err != nil {
		t.Fatalf("GetTab: %v", err)
	}
	found := false
	for _, item := range tab.Items {
		if item.ConfigKey == "cny_per_point" {
			if got := item.ConfigValue["value"]; got != "0.5000" {
				t.Fatalf("unexpected persisted value %#v", item.ConfigValue)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected persisted config item")
	}
}

func TestAdminConfigStoreMigratesLegacyPaymentRateToBillingPricing(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:configstore-legacy-rate?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewAdminConfigStore(client)
	if err := store.SaveByCategory(ctx, "payments", 7, 0, []domainadminconfig.Item{{
		ConfigCategory: "payments",
		ConfigKey:      "custom_amount_cny_per_point",
		ConfigValue:    map[string]any{"value": "0.62500"},
		Scope:          "global",
	}}); err != nil {
		t.Fatalf("save legacy payment rate: %v", err)
	}
	svc := adminconfig.NewServiceWithStore(testAdminConfig(), store)
	legacyTab, err := svc.GetTab(ctx, "billing_pricing")
	if err != nil {
		t.Fatalf("get billing pricing from legacy rate: %v", err)
	}
	assertEntConfigValue(t, legacyTab, "cny_per_point", "0.62500")

	updated, err := svc.UpdateTab(ctx, domainadminconfig.UpdateTabRequest{
		TabKey:  "billing_pricing",
		Version: legacyTab.Version,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_pricing",
			ConfigKey:      "cny_per_point",
			ConfigValue:    map[string]any{"value": "0.50000"},
			Scope:          "global",
		}},
	})
	if err != nil {
		t.Fatalf("save new billing pricing rate: %v", err)
	}
	assertEntConfigValue(t, updated, "cny_per_point", "0.50000")

	if err := store.SaveByCategory(ctx, "payments", 9, 0, []domainadminconfig.Item{{
		ConfigCategory: "payments",
		ConfigKey:      "custom_amount_cny_per_point",
		ConfigValue:    map[string]any{"value": "0.75000"},
		Scope:          "global",
	}}); err != nil {
		t.Fatalf("update legacy payment rate: %v", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		reloaded := adminconfig.NewServiceWithStore(testAdminConfig(), store)
		tab, err := reloaded.GetTab(ctx, "billing_pricing")
		if err != nil {
			t.Fatalf("reload billing pricing attempt %d: %v", attempt, err)
		}
		assertEntConfigValue(t, tab, "cny_per_point", "0.50000")
	}
}

func assertEntConfigValue(t *testing.T, tab domainadminconfig.Tab, key string, want any) {
	t.Helper()
	for _, item := range tab.Items {
		if item.ConfigKey == key {
			if got := item.ConfigValue["value"]; got != want {
				t.Fatalf("unexpected %s value: got %#v want %#v", key, got, want)
			}
			return
		}
	}
	t.Fatalf("expected config item %q in tab %#v", key, tab)
}

func testAdminConfig() config.Config {
	return config.Config{
		Auth: config.AuthConfig{
			AccessTokenTTL:    10 * time.Minute,
			RefreshTokenTTL:   2 * time.Hour,
			RefreshCookieName: "pg_refresh",
		},
		Billing: config.BillingConfig{
			CNYPerPoint:                      "0.3125",
			AutoBaseResolutionDefaultByGroup: map[string]string{"basic": "1k"},
			BaseResolutionPointsByModel:      map[string]map[string]string{"basic": {"1k": "2.00000"}},
			UserGroupMultipliers:             map[string]string{"basic": "1.00000"},
			TaskMultipliers:                  map[string]string{"text_to_image": "1.00000"},
			ReferenceImageExtra:              config.ReferenceExtra{First: "0.10000", Additional: "0.05000"},
		},
		GenerationLimits: config.GenerationLimitsConfig{
			MaxImageCount:          5,
			ReferenceImageMaxMB:    20,
			ReferenceImageMaxCount: 4,
			PromptMaxChars:         4000,
			NegativePromptMaxChars: 1000,
		},
		Providers: config.ProvidersConfig{
			OpenAI:     config.ProviderConfig{Enabled: true, BaseURL: "https://api.openai.com"},
			OpenRouter: config.ProviderConfig{Enabled: true, BaseURL: "https://openrouter.ai/api"},
		},
		Routing: config.RoutingConfig{
			DefaultProvider:   "openai",
			FallbackProviders: []string{"openrouter"},
		},
		Docs: config.DocsConfig{
			Title:    "Pic Gallery API Docs",
			BasePath: "/docs",
		},
	}
}
