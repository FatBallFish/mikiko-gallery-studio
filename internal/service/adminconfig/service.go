package adminconfig

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type tabDefinition struct {
	Key   string
	Name  string
	Items []domainadminconfig.Item
}

type Service struct {
	store       Store
	definitions []tabDefinition
}

func NewService(cfg config.Config) *Service {
	return NewServiceWithStore(cfg, NewMemoryStore())
}

func NewServiceWithStore(cfg config.Config, store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{
		store:       store,
		definitions: defaultDefinitions(cfg),
	}
}

func (s *Service) ListTabs(ctx context.Context) ([]domainadminconfig.Tab, error) {
	tabs := make([]domainadminconfig.Tab, 0, len(s.definitions))
	for _, definition := range s.definitions {
		tab, err := s.GetTab(ctx, definition.Key)
		if err != nil {
			return nil, err
		}
		tabs = append(tabs, tab)
	}
	return tabs, nil
}

func (s *Service) GetTab(ctx context.Context, tabKey string) (domainadminconfig.Tab, error) {
	definition, ok := s.findDefinition(tabKey)
	if !ok {
		return domainadminconfig.Tab{}, errs.New(404, errs.CodeNotFound, "config tab not found")
	}

	overrides, err := s.store.GetByCategory(ctx, tabKey)
	if err != nil {
		return domainadminconfig.Tab{}, err
	}
	tab := domainadminconfig.Tab{
		TabKey:  definition.Key,
		TabName: definition.Name,
		Version: 1,
		Items:   cloneItems(definition.Items),
	}
	itemsByKey := map[string]domainadminconfig.Item{}
	for _, item := range tab.Items {
		itemsByKey[itemKey(item.ConfigKey, item.Scope)] = item
	}
	for _, override := range overrides {
		key := itemKey(override.ConfigKey, defaultString(override.Scope, "global"))
		itemsByKey[key] = cloneItem(override)
		if override.Version > tab.Version {
			tab.Version = override.Version
		}
	}

	tab.Items = tab.Items[:0]
	for _, item := range itemsByKey {
		tab.Items = append(tab.Items, cloneItem(item))
	}
	sort.Slice(tab.Items, func(i, j int) bool {
		if tab.Items[i].Scope == tab.Items[j].Scope {
			return tab.Items[i].ConfigKey < tab.Items[j].ConfigKey
		}
		return tab.Items[i].Scope < tab.Items[j].Scope
	})
	return tab, nil
}

func (s *Service) UpdateTab(ctx context.Context, req domainadminconfig.UpdateTabRequest) (domainadminconfig.Tab, error) {
	current, err := s.GetTab(ctx, req.TabKey)
	if err != nil {
		return domainadminconfig.Tab{}, err
	}
	if req.Version != current.Version {
		return domainadminconfig.Tab{}, errs.New(409, errs.CodeConflict, "config tab version conflict")
	}

	if len(req.Items) == 0 {
		return domainadminconfig.Tab{}, errs.BadRequest("config items are required")
	}

	for _, item := range req.Items {
		if strings.TrimSpace(item.ConfigKey) == "" {
			return domainadminconfig.Tab{}, errs.BadRequest("config_key is required")
		}
		if item.ConfigCategory != "" && !strings.EqualFold(item.ConfigCategory, req.TabKey) {
			return domainadminconfig.Tab{}, errs.BadRequest("config_category does not match tab_key")
		}
	}

	nextVersion := current.Version + 1
	if err := s.store.SaveByCategory(ctx, req.TabKey, nextVersion, req.UpdatedBy, req.Items); err != nil {
		return domainadminconfig.Tab{}, err
	}
	return s.GetTab(ctx, req.TabKey)
}

func (s *Service) findDefinition(tabKey string) (tabDefinition, bool) {
	for _, definition := range s.definitions {
		if definition.Key == tabKey {
			return definition, true
		}
	}
	return tabDefinition{}, false
}

func defaultDefinitions(cfg config.Config) []tabDefinition {
	now := time.Now()
	_ = now // reserved for future defaults that need timestamps

	return []tabDefinition{
		{
			Key:  "auth_security",
			Name: "Auth & Security",
			Items: []domainadminconfig.Item{
				valueItem("auth_security", "access_token_ttl_sec", int(cfg.Auth.AccessTokenTTL.Seconds())),
				valueItem("auth_security", "refresh_token_ttl_sec", int(cfg.Auth.RefreshTokenTTL.Seconds())),
				valueItem("auth_security", "refresh_cookie_name", cfg.Auth.RefreshCookieName),
			},
		},
		{
			Key:  "generation_limits",
			Name: "Generation Limits",
			Items: []domainadminconfig.Item{
				valueItem("generation_limits", "max_image_count", cfg.GenerationLimits.MaxImageCount),
				valueItem("generation_limits", "reference_image_max_mb", cfg.GenerationLimits.ReferenceImageMaxMB),
				valueItem("generation_limits", "reference_image_max_count", cfg.GenerationLimits.ReferenceImageMaxCount),
				valueItem("generation_limits", "prompt_max_chars", cfg.GenerationLimits.PromptMaxChars),
				valueItem("generation_limits", "negative_prompt_max_chars", cfg.GenerationLimits.NegativePromptMaxChars),
			},
		},
		{
			Key:  "billing_pricing",
			Name: "Billing & Pricing",
			Items: []domainadminconfig.Item{
				valueItem("billing_pricing", "cny_per_point", cfg.Billing.CNYPerPoint),
				valueItem("billing_pricing", "auto_quality_default_by_group", cloneMap(cfg.Billing.AutoQualityDefaultByGroup)),
				valueItem("billing_pricing", "quality_points_by_model", cloneNestedStringMap(cfg.Billing.QualityPointsByModel)),
				valueItem("billing_pricing", "user_group_multipliers", cloneMap(cfg.Billing.UserGroupMultipliers)),
				valueItem("billing_pricing", "task_multipliers", cloneMap(cfg.Billing.TaskMultipliers)),
				valueItem("billing_pricing", "reference_image_extra", map[string]any{
					"first":      cfg.Billing.ReferenceImageExtra.First,
					"additional": cfg.Billing.ReferenceImageExtra.Additional,
				}),
			},
		},
		{
			Key:  "routing_models",
			Name: "Routing & Models",
			Items: []domainadminconfig.Item{
				valueItem("routing_models", "default_provider", cfg.Routing.DefaultProvider),
				valueItem("routing_models", "fallback_providers", slices.Clone(cfg.Routing.FallbackProviders)),
				valueItem("routing_models", "openai_compat_model_map", cloneMap(cfg.Routing.OpenAICompatModelMap)),
				valueItem("routing_models", "provider_model_map", cloneNestedStringMap(cfg.Routing.ProviderModelMap)),
				valueItem("routing_models", "provider_capabilities", providerCapabilitiesValue(cfg.Routing.ProviderCapabilities)),
			},
		},
		{
			Key:  "docs",
			Name: "Developer Docs",
			Items: []domainadminconfig.Item{
				valueItem("docs", "title", cfg.Docs.Title),
				valueItem("docs", "base_path", cfg.Docs.BasePath),
			},
		},
	}
}

func valueItem(category, key string, value any) domainadminconfig.Item {
	return domainadminconfig.Item{
		ConfigCategory: category,
		ConfigKey:      key,
		ConfigValue:    map[string]any{"value": value},
		Scope:          "global",
		Version:        1,
	}
}

func cloneItems(items []domainadminconfig.Item) []domainadminconfig.Item {
	cloned := make([]domainadminconfig.Item, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, cloneItem(item))
	}
	return cloned
}

func cloneMap(input map[string]string) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneNestedStringMap(input any) map[string]any {
	output := map[string]any{}
	switch typed := input.(type) {
	case map[string]map[string]string:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			output[key] = cloneMap(typed[key])
		}
	case map[string]string:
		return cloneMap(typed)
	}
	return output
}

func providerCapabilitiesValue(input map[string]config.ProviderCapabilityConfig) map[string]any {
	output := make(map[string]any, len(input))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item := input[key]
		output[key] = map[string]any{
			"supported_models":          slices.Clone(item.SupportedModels),
			"supported_task_types":      slices.Clone(item.SupportedTaskTypes),
			"supported_qualities":       slices.Clone(item.SupportedQualities),
			"supported_aspect_ratios":   slices.Clone(item.SupportedAspectRatios),
			"max_image_count":           item.MaxImageCount,
			"max_reference_image_count": item.MaxReferenceImageCount,
			"supports_image_input":      item.SupportsImageInput,
			"supports_mask":             item.SupportsMask,
			"priority":                  item.Priority,
		}
	}
	return output
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
