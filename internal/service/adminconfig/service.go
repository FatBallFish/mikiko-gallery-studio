package adminconfig

import (
	"context"
	"sort"
	"strings"
	"sync"
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
	listenersMu sync.RWMutex
	listeners   []func(tabKey string)
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
	if tabKey == "billing_pricing" && !hasConfigOverride(overrides, "cny_per_point", "global") {
		legacyOverrides, legacyErr := s.store.GetByCategory(ctx, "payments")
		if legacyErr != nil {
			return domainadminconfig.Tab{}, legacyErr
		}
		for _, legacy := range legacyOverrides {
			if legacy.ConfigKey != "custom_amount_cny_per_point" || defaultString(legacy.Scope, "global") != "global" {
				continue
			}
			legacy.ConfigCategory = "billing_pricing"
			legacy.ConfigKey = "cny_per_point"
			overrides = append(overrides, legacy)
			break
		}
	}
	tab := domainadminconfig.Tab{
		TabKey:  definition.Key,
		TabName: definition.Name,
		Version: 1,
		Items:   cloneItems(definition.Items),
	}
	itemsByKey := map[string]domainadminconfig.Item{}
	definedKeys := map[string]struct{}{}
	for _, item := range tab.Items {
		key := itemKey(item.ConfigKey, item.Scope)
		itemsByKey[key] = item
		definedKeys[key] = struct{}{}
	}
	for _, override := range overrides {
		key := itemKey(override.ConfigKey, defaultString(override.Scope, "global"))
		defined, ok := itemsByKey[key]
		if !ok {
			continue
		}
		override.ConfigCategory = defaultString(defined.ConfigCategory, override.ConfigCategory)
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

func hasConfigOverride(items []domainadminconfig.Item, configKey, scope string) bool {
	for _, item := range items {
		if item.ConfigKey == configKey && defaultString(item.Scope, "global") == scope {
			return true
		}
	}
	return false
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
		if item.ConfigCategory != "" && !strings.EqualFold(item.ConfigCategory, req.TabKey) && !definitionContainsCategory(current.Items, item.ConfigKey, defaultString(item.Scope, "global"), item.ConfigCategory) {
			return domainadminconfig.Tab{}, errs.BadRequest("config_category does not match tab_key")
		}
		if !definitionContainsItem(current.Items, item.ConfigKey, defaultString(item.Scope, "global")) {
			return domainadminconfig.Tab{}, errs.BadRequest("unknown config item")
		}
	}
	if req.TabKey == "attachment_policy" {
		if err := validateAttachmentPolicyItems(req.Items); err != nil {
			return domainadminconfig.Tab{}, errs.BadRequest(err.Error())
		}
	}

	nextVersion := current.Version + 1
	if err := s.store.SaveByCategory(ctx, req.TabKey, nextVersion, req.UpdatedBy, req.Items); err != nil {
		return domainadminconfig.Tab{}, err
	}
	s.notifyInvalidation(req.TabKey)
	return s.GetTab(ctx, req.TabKey)
}

func (s *Service) RegisterInvalidationListener(listener func(tabKey string)) {
	if listener == nil {
		return
	}
	s.listenersMu.Lock()
	s.listeners = append(s.listeners, listener)
	s.listenersMu.Unlock()
}

func (s *Service) notifyInvalidation(tabKey string) {
	s.listenersMu.RLock()
	listeners := append([]func(string){}, s.listeners...)
	s.listenersMu.RUnlock()
	for _, listener := range listeners {
		listener(tabKey)
	}
}

func (s *Service) findDefinition(tabKey string) (tabDefinition, bool) {
	for _, definition := range s.definitions {
		if definition.Key == tabKey {
			return definition, true
		}
	}
	return tabDefinition{}, false
}

func definitionContainsItem(items []domainadminconfig.Item, configKey, scope string) bool {
	for _, item := range items {
		if item.ConfigKey == configKey && defaultString(item.Scope, "global") == scope {
			return true
		}
	}
	return false
}

func definitionContainsCategory(items []domainadminconfig.Item, configKey, scope, category string) bool {
	for _, item := range items {
		if item.ConfigKey == configKey && defaultString(item.Scope, "global") == scope && strings.EqualFold(item.ConfigCategory, category) {
			return true
		}
	}
	return false
}

func defaultDefinitions(cfg config.Config) []tabDefinition {
	now := time.Now()
	_ = now // reserved for future defaults that need timestamps
	attachmentPolicy := config.ApplyAttachmentPolicyDefaults(cfg.AttachmentPolicy, cfg.GenerationLimits.ReferenceImageMaxMB)

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
			Key:  "attachment_policy",
			Name: "Attachment Policy",
			Items: []domainadminconfig.Item{
				valueItem("attachment_policy", "image_max_mb", attachmentPolicy.ImageMaxMB),
				valueItem("attachment_policy", "video_max_mb", attachmentPolicy.VideoMaxMB),
				valueItem("attachment_policy", "audio_max_mb", attachmentPolicy.AudioMaxMB),
				valueItem("attachment_policy", "document_max_mb", attachmentPolicy.DocumentMaxMB),
				valueItem("attachment_policy", "image_allowed_formats", append([]string(nil), attachmentPolicy.ImageAllowedFormats...)),
				valueItem("attachment_policy", "video_allowed_formats", append([]string(nil), attachmentPolicy.VideoAllowedFormats...)),
				valueItem("attachment_policy", "audio_allowed_formats", append([]string(nil), attachmentPolicy.AudioAllowedFormats...)),
				valueItem("attachment_policy", "document_allowed_formats", append([]string(nil), attachmentPolicy.DocumentAllowedFormats...)),
			},
		},
		{
			Key:  "billing_pricing",
			Name: "Billing & Pricing",
			Items: []domainadminconfig.Item{
				valueItem("billing_pricing", "cny_per_point", cfg.Billing.CNYPerPoint),
				valueItem("billing_pricing", "auto_base_resolution_default_by_group", cloneMap(cfg.Billing.AutoBaseResolutionDefaultByGroup)),
				valueItem("billing_pricing", "task_multipliers", cloneMap(cfg.Billing.TaskMultipliers)),
				valueItem("billing_pricing", "reference_image_extra", map[string]any{
					"first":      cfg.Billing.ReferenceImageExtra.First,
					"additional": cfg.Billing.ReferenceImageExtra.Additional,
				}),
			},
		},
		{
			Key:  "trial_credits",
			Name: "Trial Credits",
			Items: []domainadminconfig.Item{
				valueItem("billing_trial", "signup_trial", map[string]any{
					"enabled":              cfg.Billing.SignupTrial.Enabled,
					"points":               cfg.Billing.SignupTrial.Points,
					"valid_days":           cfg.Billing.SignupTrial.ValidDays,
					"expiry_reminder_days": cfg.Billing.SignupTrial.ExpiryReminderDays,
					"grant_once_per_user":  cfg.Billing.SignupTrial.GrantOncePerUser,
				}),
			},
		},
		{
			Key:  "openai_compat",
			Name: "OpenAI Compatibility",
			Items: []domainadminconfig.Item{
				valueItem("openai_compat", "openai_compat_model_map", cloneMap(cfg.Routing.OpenAICompatModelMap)),
			},
		},
		{
			Key:  "public_gallery",
			Name: "Public Gallery",
			Items: []domainadminconfig.Item{
				valueItem("public_gallery", "publish_request_enabled", false),
				valueItem("public_gallery", "gallery_enabled", true),
			},
		},
		{
			Key:  "moderation",
			Name: "Moderation",
			Items: []domainadminconfig.Item{
				valueItem("moderation", "provider", "openai"),
				valueItem("moderation", "enabled", false),
			},
		},
		{
			Key:  "runtime",
			Name: "Runtime",
			Items: []domainadminconfig.Item{
				valueItem("runtime", "worker_max_concurrent_tasks", cfg.Worker.MaxConcurrentTasks),
			},
		},
		{
			Key:  "payments",
			Name: "Payments",
			Items: []domainadminconfig.Item{
				valueItem("payments", "enabled", false),
				valueItem("payments", "site_base_url", cfg.Cashier.SiteBaseURL),
				valueItem("payments", "order_timeout_seconds", defaultInt(cfg.Cashier.OrderTimeoutSeconds, 1800)),
				valueItem("payments", "max_pending_orders_per_user", defaultInt(cfg.Cashier.MaxPendingOrdersPerUser, 3)),
				valueItem("payments", "providers", []string{"alipay", "wxpay"}),
				valueItem("payments", "custom_amount_enabled", true),
				valueItem("payments", "custom_amount_min_cny", "1.00000"),
				valueItem("payments", "custom_amount_max_cny", "999.00000"),
				valueItem("payments", "visible_methods", []map[string]any{
					{
						"method":               "mock",
						"label":                "Mock 支付",
						"enabled":              true,
						"source_provider_type": "mock",
						"scheduler_strategy":   "round_robin",
						"display_order":        10,
						"description":          "测试环境模拟支付链路",
					},
				}),
				valueItem("payments", "provider_instances", []map[string]any{}),
				valueItem("payments", "scheduler_state", map[string]any{}),
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

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
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

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
