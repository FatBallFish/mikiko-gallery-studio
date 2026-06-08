package cashier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const paymentsTabKey = "payments"
const defaultCustomAmountCNYPerPoint = "0.31250"

type ConfigStore interface {
	PaymentConfigValue(ctx context.Context, key string) (any, error)
	SavePaymentConfigValue(ctx context.Context, key string, value any, adminID int64) error
	ProductionMode() bool
}

type ProviderInstanceStore interface {
	ProviderInstances(ctx context.Context) ([]domaincashier.ProviderInstance, error)
	CreateProviderInstance(ctx context.Context, req domaincashier.ProviderInstanceWriteRequest) (domaincashier.ProviderInstance, error)
	UpdateProviderInstance(ctx context.Context, instanceID int64, req domaincashier.ProviderInstanceWriteRequest) (domaincashier.ProviderInstance, error)
	DeleteProviderInstance(ctx context.Context, instanceID int64) (domaincashier.ProviderInstance, error)
}

type AdminConfigStore struct {
	admin         *adminconfigservice.Service
	production    bool
	paymentsTab   string
	paymentsScope string
	cnyPerPoint   string
}

func NewAdminConfigStore(admin *adminconfigservice.Service, production bool) *AdminConfigStore {
	return NewAdminConfigStoreWithDefaultCNYPerPoint(admin, production, "")
}

func NewAdminConfigStoreWithDefaultCNYPerPoint(admin *adminconfigservice.Service, production bool, cnyPerPoint string) *AdminConfigStore {
	return &AdminConfigStore{
		admin:         admin,
		production:    production,
		paymentsTab:   paymentsTabKey,
		paymentsScope: "global",
		cnyPerPoint:   defaultString(cnyPerPoint, defaultCustomAmountCNYPerPoint),
	}
}

func (s *AdminConfigStore) PaymentConfigValue(ctx context.Context, key string) (any, error) {
	if s == nil || s.admin == nil {
		return nil, errs.Internal("cashier config store is not available")
	}
	tab, err := s.admin.GetTab(ctx, s.paymentsTab)
	if err != nil {
		return nil, err
	}
	for _, item := range tab.Items {
		if item.ConfigKey == key {
			return item.ConfigValue["value"], nil
		}
	}
	return nil, nil
}

func (s *AdminConfigStore) SavePaymentConfigValue(ctx context.Context, key string, value any, adminID int64) error {
	if s == nil || s.admin == nil {
		return errs.Internal("cashier config store is not available")
	}
	tab, err := s.admin.GetTab(ctx, s.paymentsTab)
	if err != nil {
		return err
	}
	_, err = s.admin.UpdateTab(ctx, domainadminconfig.UpdateTabRequest{
		TabKey:  s.paymentsTab,
		Version: tab.Version,
		Items: []domainadminconfig.Item{{
			ConfigCategory: s.paymentsTab,
			ConfigKey:      key,
			ConfigValue:    map[string]any{"value": value},
			Scope:          s.paymentsScope,
		}},
		UpdatedBy: adminID,
	})
	return err
}

func (s *AdminConfigStore) ProductionMode() bool {
	return s != nil && s.production
}

func (s *AdminConfigStore) DefaultCNYPerPoint() string {
	if s == nil {
		return defaultCustomAmountCNYPerPoint
	}
	return defaultString(s.cnyPerPoint, defaultCustomAmountCNYPerPoint)
}

type ConfigFacade struct {
	store              ConfigStore
	providerInstances  ProviderInstanceStore
	defaultCNYPerPoint string
}

func NewConfigFacade(store ConfigStore) *ConfigFacade {
	defaultCNYPerPoint := defaultCustomAmountCNYPerPoint
	if provider, ok := store.(interface{ DefaultCNYPerPoint() string }); ok {
		defaultCNYPerPoint = defaultString(provider.DefaultCNYPerPoint(), defaultCustomAmountCNYPerPoint)
	}
	return &ConfigFacade{store: store, defaultCNYPerPoint: defaultCNYPerPoint}
}

func (f *ConfigFacade) WithProviderInstanceStore(store ProviderInstanceStore) *ConfigFacade {
	if f == nil {
		return f
	}
	f.providerInstances = store
	return f
}

func (f *ConfigFacade) CustomAmountConfig(ctx context.Context) (domaincashier.CustomAmountConfig, error) {
	cfg := domaincashier.CustomAmountConfig{
		Enabled:      true,
		MinAmountCNY: "1.00000",
		MaxAmountCNY: "999.00000",
		CNYPerPoint:  defaultString(f.defaultCNYPerPoint, defaultCustomAmountCNYPerPoint),
	}
	if enabled, ok, err := f.boolValue(ctx, "custom_amount_enabled"); err != nil {
		return domaincashier.CustomAmountConfig{}, err
	} else if ok {
		cfg.Enabled = enabled
	}
	if value, ok, err := f.stringValue(ctx, "custom_amount_min_cny"); err != nil {
		return domaincashier.CustomAmountConfig{}, err
	} else if ok {
		cfg.MinAmountCNY = value
	}
	if value, ok, err := f.stringValue(ctx, "custom_amount_max_cny"); err != nil {
		return domaincashier.CustomAmountConfig{}, err
	} else if ok {
		cfg.MaxAmountCNY = value
	}
	if value, ok, err := f.stringValue(ctx, "custom_amount_cny_per_point"); err != nil {
		return domaincashier.CustomAmountConfig{}, err
	} else if ok {
		cfg.CNYPerPoint = value
	}
	return NormalizeCustomAmountConfig(cfg)
}

func (f *ConfigFacade) UpdateCustomAmountConfig(ctx context.Context, cfg domaincashier.CustomAmountConfig, adminID int64) (domaincashier.CustomAmountConfig, error) {
	normalized, err := NormalizeCustomAmountConfig(cfg)
	if err != nil {
		return domaincashier.CustomAmountConfig{}, err
	}
	for key, value := range map[string]any{
		"custom_amount_enabled":       normalized.Enabled,
		"custom_amount_min_cny":       normalized.MinAmountCNY,
		"custom_amount_max_cny":       normalized.MaxAmountCNY,
		"custom_amount_cny_per_point": normalized.CNYPerPoint,
	} {
		if err := f.store.SavePaymentConfigValue(ctx, key, value, adminID); err != nil {
			return domaincashier.CustomAmountConfig{}, err
		}
	}
	return f.CustomAmountConfig(ctx)
}

func (f *ConfigFacade) VisibleMethods(ctx context.Context, includeDisabled bool) ([]domaincashier.VisibleMethod, error) {
	raw, err := f.store.PaymentConfigValue(ctx, "visible_methods")
	if err != nil {
		return nil, err
	}
	methods, err := ParseVisibleMethods(raw)
	if err != nil {
		return nil, err
	}
	filtered := make([]domaincashier.VisibleMethod, 0, len(methods))
	for _, method := range methods {
		if !includeDisabled && !method.Enabled {
			continue
		}
		if !includeDisabled && f.store.ProductionMode() && method.SourceProviderType == "mock" {
			continue
		}
		filtered = append(filtered, method)
	}
	return filtered, nil
}

func (f *ConfigFacade) UpdateVisibleMethods(ctx context.Context, methods []domaincashier.VisibleMethod, adminID int64) ([]domaincashier.VisibleMethod, error) {
	normalized, err := NormalizeVisibleMethods(methods)
	if err != nil {
		return nil, err
	}
	if err := f.store.SavePaymentConfigValue(ctx, "visible_methods", VisibleMethodsConfigValue(normalized), adminID); err != nil {
		return nil, err
	}
	return f.VisibleMethods(ctx, true)
}

func (f *ConfigFacade) ProviderInstances(ctx context.Context) ([]domaincashier.ProviderInstance, error) {
	if f.providerInstances != nil {
		instances, err := f.providerInstances.ProviderInstances(ctx)
		if err != nil {
			return nil, err
		}
		instances = appendDefaultMockProviderInstance(instances, !f.store.ProductionMode(), time.Now().UTC())
		sortProviderInstances(instances)
		return instances, nil
	}
	raw, err := f.store.PaymentConfigValue(ctx, "provider_instances")
	if err != nil {
		return nil, err
	}
	instances, err := ParseProviderInstances(raw, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	instances = appendDefaultMockProviderInstance(instances, !f.store.ProductionMode(), time.Now().UTC())
	sortProviderInstances(instances)
	return instances, nil
}

func (f *ConfigFacade) CreateProviderInstance(ctx context.Context, req domaincashier.ProviderInstanceWriteRequest, adminID int64) (domaincashier.ProviderInstance, error) {
	if f.providerInstances != nil {
		return f.providerInstances.CreateProviderInstance(ctx, req)
	}
	current, err := f.ProviderInstances(ctx)
	if err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	next := nextProviderInstanceID(current)
	now := time.Now().UTC()
	instance, err := ProviderInstanceForWrite(req, nil)
	if err != nil {
		return domaincashier.ProviderInstance{}, errs.BadRequest(err.Error())
	}
	normalized, err := NormalizeProviderInstance(instance, next, now)
	if err != nil {
		return domaincashier.ProviderInstance{}, errs.BadRequest(err.Error())
	}
	current = append(current, normalized)
	if err := f.saveProviderInstances(ctx, current, adminID); err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	return normalized, nil
}

func (f *ConfigFacade) UpdateProviderInstance(ctx context.Context, instanceID int64, req domaincashier.ProviderInstanceWriteRequest, adminID int64) (domaincashier.ProviderInstance, error) {
	if f.providerInstances != nil {
		return f.providerInstances.UpdateProviderInstance(ctx, instanceID, req)
	}
	current, err := f.ProviderInstances(ctx)
	if err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	index := -1
	for i, item := range current {
		if item.ID == instanceID {
			index = i
			break
		}
	}
	if index < 0 {
		return domaincashier.ProviderInstance{}, errs.New(404, errs.CodeNotFound, "payment provider instance not found")
	}
	instance, err := ProviderInstanceForWrite(req, current[index].Config)
	if err != nil {
		return domaincashier.ProviderInstance{}, errs.BadRequest(err.Error())
	}
	normalized, err := NormalizeProviderInstance(instance, instanceID, time.Now().UTC())
	if err != nil {
		return domaincashier.ProviderInstance{}, errs.BadRequest(err.Error())
	}
	normalized.CreatedAt = current[index].CreatedAt
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
	}
	normalized.UpdatedAt = time.Now().UTC()
	current[index] = normalized
	if err := f.saveProviderInstances(ctx, current, adminID); err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	return normalized, nil
}

func (f *ConfigFacade) DeleteProviderInstance(ctx context.Context, instanceID int64, adminID int64) (domaincashier.ProviderInstance, error) {
	if f.providerInstances != nil {
		return f.providerInstances.DeleteProviderInstance(ctx, instanceID)
	}
	current, err := f.ProviderInstances(ctx)
	if err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	index := -1
	var deleted domaincashier.ProviderInstance
	for i, item := range current {
		if item.ID == instanceID {
			index = i
			deleted = item
			break
		}
	}
	if index < 0 {
		return domaincashier.ProviderInstance{}, errs.New(404, errs.CodeNotFound, "payment provider instance not found")
	}
	current = append(current[:index], current[index+1:]...)
	if err := f.saveProviderInstances(ctx, current, adminID); err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	return deleted, nil
}

func (f *ConfigFacade) SchedulerState(ctx context.Context) (map[string]map[string]any, error) {
	raw, err := f.store.PaymentConfigValue(ctx, "scheduler_state")
	if err != nil {
		return nil, err
	}
	return ParseSchedulerState(raw), nil
}

func (f *ConfigFacade) SaveSchedulerState(ctx context.Context, state map[string]map[string]any, adminID int64) error {
	return f.store.SavePaymentConfigValue(ctx, "scheduler_state", NormalizeSchedulerState(state), adminID)
}

func (f *ConfigFacade) saveProviderInstances(ctx context.Context, instances []domaincashier.ProviderInstance, adminID int64) error {
	values := make([]map[string]any, 0, len(instances))
	for _, item := range instances {
		values = append(values, ProviderInstanceConfigValue(item))
	}
	return f.store.SavePaymentConfigValue(ctx, "provider_instances", values, adminID)
}

func (f *ConfigFacade) boolValue(ctx context.Context, key string) (bool, bool, error) {
	raw, err := f.store.PaymentConfigValue(ctx, key)
	if err != nil {
		return false, false, err
	}
	switch value := raw.(type) {
	case bool:
		return value, true, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes", "on":
			return true, true, nil
		case "false", "0", "no", "off":
			return false, true, nil
		}
	}
	return false, false, nil
}

func (f *ConfigFacade) stringValue(ctx context.Context, key string) (string, bool, error) {
	raw, err := f.store.PaymentConfigValue(ctx, key)
	if err != nil {
		return "", false, err
	}
	switch value := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		return trimmed, trimmed != "", nil
	case fmt.Stringer:
		trimmed := strings.TrimSpace(value.String())
		return trimmed, trimmed != "", nil
	case nil:
		return "", false, nil
	default:
		return strings.TrimSpace(fmt.Sprint(value)), true, nil
	}
}

func defaultString(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func NormalizeCustomAmountConfig(cfg domaincashier.CustomAmountConfig) (domaincashier.CustomAmountConfig, error) {
	minAmount, minValue, err := positiveDecimalString(cfg.MinAmountCNY, "min_amount_cny")
	if err != nil {
		return domaincashier.CustomAmountConfig{}, err
	}
	maxAmount, maxValue, err := positiveDecimalString(cfg.MaxAmountCNY, "max_amount_cny")
	if err != nil {
		return domaincashier.CustomAmountConfig{}, err
	}
	cnyPerPoint, _, err := positiveDecimalString(cfg.CNYPerPoint, "cny_per_point")
	if err != nil {
		return domaincashier.CustomAmountConfig{}, err
	}
	if minValue.GreaterThan(maxValue) {
		return domaincashier.CustomAmountConfig{}, errs.BadRequest("min_amount_cny must be less than or equal to max_amount_cny")
	}
	return domaincashier.CustomAmountConfig{
		Enabled:      cfg.Enabled,
		MinAmountCNY: minAmount,
		MaxAmountCNY: maxAmount,
		CNYPerPoint:  cnyPerPoint,
	}, nil
}

func ValidateCustomAmount(raw string, cfg domaincashier.CustomAmountConfig) error {
	_, amount, err := positiveDecimalString(raw, "amount_cny")
	if err != nil {
		return paymentAmountOutOfRange("amount_cny must be positive")
	}
	_, minAmount, err := positiveDecimalString(cfg.MinAmountCNY, "min_amount_cny")
	if err != nil {
		return err
	}
	_, maxAmount, err := positiveDecimalString(cfg.MaxAmountCNY, "max_amount_cny")
	if err != nil {
		return err
	}
	if amount.LessThan(minAmount) {
		return paymentAmountOutOfRange("amount_cny must be greater than or equal to min_amount_cny")
	}
	if amount.GreaterThan(maxAmount) {
		return paymentAmountOutOfRange("amount_cny must be less than or equal to max_amount_cny")
	}
	return nil
}

func paymentAmountOutOfRange(message string) error {
	return errs.New(http.StatusBadRequest, errs.CodePaymentAmountOutOfRange, message)
}

func ParseVisibleMethods(raw any) ([]domaincashier.VisibleMethod, error) {
	if raw == nil {
		return DefaultVisibleMethods(), nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, errs.BadRequest("visible_methods must be an array")
	}
	var items []domaincashier.VisibleMethod
	if err := json.Unmarshal(encoded, &items); err != nil {
		return nil, errs.BadRequest("visible_methods must be an array")
	}
	return NormalizeVisibleMethods(items)
}

func NormalizeVisibleMethods(items []domaincashier.VisibleMethod) ([]domaincashier.VisibleMethod, error) {
	normalized := make([]domaincashier.VisibleMethod, 0, len(items))
	seen := map[string]struct{}{}
	for index, item := range items {
		item.Method = strings.ToLower(strings.TrimSpace(item.Method))
		if item.Method == "" {
			return nil, errs.BadRequest("visible method is required")
		}
		if _, ok := seen[item.Method]; ok {
			return nil, errs.BadRequest("visible method is duplicated")
		}
		seen[item.Method] = struct{}{}
		item.Label = strings.TrimSpace(item.Label)
		if item.Label == "" {
			item.Label = item.Method
		}
		item.SourceProviderType = strings.ToLower(strings.TrimSpace(item.SourceProviderType))
		if item.SourceProviderType == "" {
			item.SourceProviderType = item.Method
		}
		item.SchedulerStrategy = strings.ToLower(strings.TrimSpace(item.SchedulerStrategy))
		if item.SchedulerStrategy == "" {
			item.SchedulerStrategy = "round_robin"
		}
		if item.SchedulerStrategy != "round_robin" && item.SchedulerStrategy != "random" {
			return nil, errs.BadRequest("scheduler_strategy must be round_robin or random")
		}
		if item.DisplayOrder <= 0 {
			item.DisplayOrder = (index + 1) * 10
		}
		if !visibleMethodProviderAllowed(item.Method, item.SourceProviderType) {
			return nil, errs.BadRequest("source_provider_type is not allowed for method " + item.Method)
		}
		normalized = append(normalized, item)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].DisplayOrder != normalized[j].DisplayOrder {
			return normalized[i].DisplayOrder < normalized[j].DisplayOrder
		}
		return normalized[i].Method < normalized[j].Method
	})
	return normalized, nil
}

func VisibleMethodsConfigValue(items []domaincashier.VisibleMethod) []map[string]any {
	values := make([]map[string]any, 0, len(items))
	for _, item := range items {
		values = append(values, map[string]any{
			"method":               item.Method,
			"label":                item.Label,
			"enabled":              item.Enabled,
			"source_provider_type": item.SourceProviderType,
			"scheduler_strategy":   item.SchedulerStrategy,
			"display_order":        item.DisplayOrder,
			"description":          item.Description,
		})
	}
	return values
}

func DefaultVisibleMethods() []domaincashier.VisibleMethod {
	return []domaincashier.VisibleMethod{
		{
			Method:             "mock",
			Label:              "Mock 支付",
			Enabled:            true,
			SourceProviderType: "mock",
			SchedulerStrategy:  "round_robin",
			DisplayOrder:       10,
			Description:        "测试环境模拟支付链路",
		},
	}
}

func ParseProviderInstances(raw any, now time.Time) ([]domaincashier.ProviderInstance, error) {
	if raw == nil {
		return []domaincashier.ProviderInstance{}, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, errs.BadRequest("provider_instances must be an array")
	}
	var instances []domaincashier.ProviderInstance
	if err := json.Unmarshal(encoded, &instances); err != nil {
		return nil, errs.BadRequest("provider_instances must be an array")
	}
	normalized := make([]domaincashier.ProviderInstance, 0, len(instances))
	seen := map[int64]struct{}{}
	for _, item := range instances {
		item.ID = int64FromAny(item.ID)
		if item.ID <= 0 {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		parsed, normalizeErr := NormalizeProviderInstance(item, item.ID, now)
		if normalizeErr != nil {
			return nil, errs.BadRequest(normalizeErr.Error())
		}
		parsed.CreatedAt = item.CreatedAt
		if parsed.CreatedAt.IsZero() {
			parsed.CreatedAt = now
		}
		parsed.UpdatedAt = item.UpdatedAt
		if parsed.UpdatedAt.IsZero() {
			parsed.UpdatedAt = parsed.CreatedAt
		}
		normalized = append(normalized, parsed)
	}
	return normalized, nil
}

func ProviderInstanceConfigValue(item domaincashier.ProviderInstance) map[string]any {
	return map[string]any{
		"id":                item.ID,
		"provider_type":     item.ProviderType,
		"name":              item.Name,
		"enabled":           item.Enabled,
		"supported_methods": item.SupportedMethods,
		"sort_order":        item.SortOrder,
		"scheduler_weight":  item.SchedulerWeight,
		"limits":            item.Limits,
		"config":            item.Config,
		"config_status":     item.ConfigStatus,
		"last_error":        item.LastError,
		"created_at":        item.CreatedAt,
		"updated_at":        item.UpdatedAt,
	}
}

func DefaultMockProviderInstance(now time.Time) domaincashier.ProviderInstance {
	return domaincashier.ProviderInstance{
		ID:               1,
		ProviderType:     "mock",
		Name:             "Mock Payment",
		Enabled:          true,
		SupportedMethods: []string{"mock"},
		SortOrder:        10,
		SchedulerWeight:  1,
		Limits: map[string]any{
			"min_amount_cny": "1.00000",
			"max_amount_cny": "999.00000",
		},
		ConfigStatus: "configured",
		Config:       map[string]any{"mock": true},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func ParseSchedulerState(raw any) map[string]map[string]any {
	state := map[string]map[string]any{}
	if raw == nil {
		return state
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return state
	}
	if err := json.Unmarshal(encoded, &state); err == nil {
		return NormalizeSchedulerState(state)
	}
	var loose map[string]any
	if err := json.Unmarshal(encoded, &loose); err != nil {
		return state
	}
	for key, value := range loose {
		nested, ok := value.(map[string]any)
		if !ok {
			continue
		}
		state[key] = nested
	}
	return NormalizeSchedulerState(state)
}

func NormalizeSchedulerState(state map[string]map[string]any) map[string]map[string]any {
	normalized := make(map[string]map[string]any, len(state))
	for key, item := range state {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = normalizeMap(item)
	}
	return normalized
}

func SchedulerStateIDs(state map[string]map[string]any) map[string]int64 {
	ids := make(map[string]int64, len(state))
	for key, item := range state {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		ids[key] = int64FromAny(item["last_instance_id"])
	}
	return ids
}

func MergeSchedulerStateIDs(existing map[string]map[string]any, ids map[string]int64) map[string]map[string]any {
	merged := NormalizeSchedulerState(existing)
	for key, id := range ids {
		key = strings.TrimSpace(key)
		if key == "" || id <= 0 {
			continue
		}
		merged[key] = map[string]any{"last_instance_id": id}
	}
	return merged
}

func visibleMethodProviderAllowed(method, provider string) bool {
	switch method {
	case "mock":
		return provider == "mock"
	case "alipay":
		return provider == "alipay_direct" || provider == "easypay_alipay" || provider == "mock" || provider == "jeepay_alipay"
	case "wxpay":
		return provider == "wxpay_direct" || provider == "easypay_wxpay" || provider == "mock" || provider == "jeepay_wxpay"
	default:
		return false
	}
}

func nextProviderInstanceID(instances []domaincashier.ProviderInstance) int64 {
	var maxID int64
	for _, item := range instances {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	return maxID + 1
}

func appendDefaultMockProviderInstance(instances []domaincashier.ProviderInstance, enabled bool, now time.Time) []domaincashier.ProviderInstance {
	if !enabled || hasEnabledMockProviderInstance(instances) {
		return instances
	}
	mock := DefaultMockProviderInstance(now)
	mock.ID = nextProviderInstanceID(instances)
	return append(instances, mock)
}

func hasEnabledMockProviderInstance(instances []domaincashier.ProviderInstance) bool {
	for _, instance := range instances {
		if !instance.Enabled || strings.TrimSpace(instance.ProviderType) != "mock" {
			continue
		}
		for _, method := range instance.SupportedMethods {
			if strings.TrimSpace(method) == "mock" {
				return true
			}
		}
	}
	return false
}

func sortProviderInstances(instances []domaincashier.ProviderInstance) {
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].SortOrder != instances[j].SortOrder {
			return instances[i].SortOrder < instances[j].SortOrder
		}
		return instances[i].ID < instances[j].ID
	})
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0
		}
		value, err := decimal.NewFromString(trimmed)
		if err != nil {
			return 0
		}
		return value.IntPart()
	default:
		return 0
	}
}
