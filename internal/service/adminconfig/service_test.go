package adminconfig_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	"github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestListTabsReturnsDefaultRuntimeConfig(t *testing.T) {
	svc := adminconfig.NewService(testConfig())

	tabs, err := svc.ListTabs(context.Background())
	if err != nil {
		t.Fatalf("ListTabs: %v", err)
	}
	if len(tabs) == 0 {
		t.Fatalf("expected default tabs")
	}

	var billingTab domainadminconfig.Tab
	for _, item := range tabs {
		if item.TabKey == "billing_pricing" {
			billingTab = item
			break
		}
	}
	if billingTab.TabKey == "" {
		t.Fatalf("expected billing_pricing tab, got %#v", tabs)
	}
	if billingTab.Version != 1 {
		t.Fatalf("expected default version 1, got %d", billingTab.Version)
	}

	found := false
	for _, item := range billingTab.Items {
		if item.ConfigKey == "cny_per_point" {
			if got := item.ConfigValue["value"]; got != "0.3125" {
				t.Fatalf("unexpected cny_per_point value %#v", item.ConfigValue)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cny_per_point item in billing tab")
	}
	assertTabMissing(t, tabs, "routing_models")
	assertTabKeys(t, tabs, "billing_pricing", []string{
		"auto_quality_default_by_group",
		"cny_per_point",
		"reference_image_extra",
		"task_multipliers",
	})
	assertTabKeys(t, tabs, "openai_compat", []string{"openai_compat_model_map"})
}

func TestUpdateTabOverridesConfigAndBumpsVersion(t *testing.T) {
	svc := adminconfig.NewServiceWithStore(testConfig(), adminconfig.NewMemoryStore())

	updated, err := svc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "generation_limits",
		Version: 1,
		Items: []domainadminconfig.Item{
			{
				ConfigCategory: "generation_limits",
				ConfigKey:      "max_image_count",
				ConfigValue:    map[string]any{"value": 3},
				Scope:          "global",
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}

	loaded, err := svc.GetTab(context.Background(), "generation_limits")
	if err != nil {
		t.Fatalf("GetTab: %v", err)
	}
	found := false
	for _, item := range loaded.Items {
		if item.ConfigKey == "max_image_count" {
			if got := item.ConfigValue["value"]; got != float64(3) && got != 3 {
				t.Fatalf("unexpected max_image_count value %#v", item.ConfigValue)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected overridden max_image_count in tab")
	}
}

func TestUpdateTabRejectsVersionConflict(t *testing.T) {
	svc := adminconfig.NewServiceWithStore(testConfig(), adminconfig.NewMemoryStore())

	_, err := svc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "generation_limits",
		Version: 99,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "generation_limits",
			ConfigKey:      "max_image_count",
			ConfigValue:    map[string]any{"value": 4},
			Scope:          "global",
		}},
	})
	if err == nil {
		t.Fatalf("expected version conflict")
	}
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 409 {
		t.Fatalf("expected 409 app error, got %#v", err)
	}
}

func TestAdminConfigIgnoresStaleOverridesAndRejectsUnknownItems(t *testing.T) {
	store := adminconfig.NewMemoryStore()
	svc := adminconfig.NewServiceWithStore(testConfig(), store)

	if _, err := svc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "billing_pricing",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_pricing",
			ConfigKey:      "quality_points_by_model",
			ConfigValue:    map[string]any{"value": map[string]any{"basic": map[string]any{"1k": "1.00000"}}},
			Scope:          "global",
		}},
	}); err == nil {
		t.Fatal("expected stale billing key to be rejected")
	}

	if err := store.SaveByCategory(context.Background(), "billing_pricing", 2, 0, []domainadminconfig.Item{{
		ConfigCategory: "billing_pricing",
		ConfigKey:      "quality_points_by_model",
		ConfigValue:    map[string]any{"value": map[string]any{"basic": map[string]any{"1k": "1.00000"}}},
		Scope:          "global",
	}}); err != nil {
		t.Fatalf("SaveByCategory stale override: %v", err)
	}

	tab, err := svc.GetTab(context.Background(), "billing_pricing")
	if err != nil {
		t.Fatalf("GetTab: %v", err)
	}
	for _, item := range tab.Items {
		if item.ConfigKey == "quality_points_by_model" || item.ConfigKey == "user_group_multipliers" {
			t.Fatalf("stale config key leaked into system settings: %#v", item)
		}
	}
	if tab.Version != 1 {
		t.Fatalf("expected stale overrides to be ignored for version, got %d", tab.Version)
	}
}

func assertTabMissing(t *testing.T, tabs []domainadminconfig.Tab, tabKey string) {
	t.Helper()
	for _, tab := range tabs {
		if tab.TabKey == tabKey {
			t.Fatalf("expected tab %q to be removed", tabKey)
		}
	}
}

func assertTabKeys(t *testing.T, tabs []domainadminconfig.Tab, tabKey string, expected []string) {
	t.Helper()
	for _, tab := range tabs {
		if tab.TabKey != tabKey {
			continue
		}
		got := make([]string, 0, len(tab.Items))
		for _, item := range tab.Items {
			got = append(got, item.ConfigKey)
		}
		slices.Sort(got)
		slices.Sort(expected)
		if !slices.Equal(got, expected) {
			t.Fatalf("unexpected keys for %s: got %#v want %#v", tabKey, got, expected)
		}
		return
	}
	t.Fatalf("expected tab %q", tabKey)
}

func testConfig() config.Config {
	return config.Config{
		Auth: config.AuthConfig{
			AccessTokenTTL:    10 * time.Minute,
			RefreshTokenTTL:   2 * time.Hour,
			RefreshCookieName: "pg_refresh",
		},
		Billing: config.BillingConfig{
			CNYPerPoint: "0.3125",
			QualityPointsByModel: map[string]map[string]string{
				"basic": {"1k": "2.00000", "2k": "4.00000", "4k": "8.00000"},
			},
			AutoQualityDefaultByGroup: map[string]string{"basic": "1k"},
			UserGroupMultipliers:      map[string]string{"basic": "1.00000"},
			TaskMultipliers:           map[string]string{"text_to_image": "1.00000"},
			ReferenceImageExtra:       config.ReferenceExtra{First: "0.10000", Additional: "0.05000"},
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
			DefaultProvider:      "openai",
			FallbackProviders:    []string{"openrouter"},
			OpenAICompatModelMap: map[string]string{"gpt-image-2": "plus"},
		},
		Docs: config.DocsConfig{
			Title:    "Pic Gallery API Docs",
			BasePath: "/docs",
		},
	}
}
