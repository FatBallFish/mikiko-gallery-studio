package cashier

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func NormalizeProviderInstance(req domaincashier.ProviderInstance, instanceID int64, now time.Time) (domaincashier.ProviderInstance, error) {
	req.ID = instanceID
	req.ProviderType = strings.ToLower(strings.TrimSpace(req.ProviderType))
	if !ProviderTypeAllowed(req.ProviderType) {
		return domaincashier.ProviderInstance{}, fmt.Errorf("provider_type is not supported")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return domaincashier.ProviderInstance{}, fmt.Errorf("name is required")
	}
	req.SupportedMethods = normalizeStringList(req.SupportedMethods)
	if len(req.SupportedMethods) == 0 {
		req.SupportedMethods = DefaultMethodsForProviderType(req.ProviderType)
	}
	for _, method := range req.SupportedMethods {
		if !ProviderSupportsMethod(req.ProviderType, method) {
			return domaincashier.ProviderInstance{}, fmt.Errorf("supported_methods is not allowed for provider_type")
		}
	}
	if req.SchedulerWeight <= 0 {
		req.SchedulerWeight = 100
	}
	if req.SortOrder < 0 {
		req.SortOrder = 0
	}
	req.Limits = normalizeMap(req.Limits)
	if err := normalizeProviderLimits(req.Limits); err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	req.Config = normalizeMap(req.Config)
	req.ConfigStatus = "missing"
	if len(req.Config) > 0 || req.ProviderType == "mock" {
		req.ConfigStatus = "configured"
	}
	req.LastError = strings.TrimSpace(req.LastError)
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}
	if req.UpdatedAt.IsZero() {
		req.UpdatedAt = req.CreatedAt
	}
	return req, nil
}

func ProviderTypeAllowed(providerType string) bool {
	switch providerType {
	case "mock", "alipay_direct", "wxpay_direct", "easypay_alipay", "easypay_wxpay", "jeepay_alipay", "jeepay_wxpay", "stripe":
		return true
	default:
		return false
	}
}

func ProviderSupportsMethod(providerType, method string) bool {
	method = strings.ToLower(strings.TrimSpace(method))
	switch providerType {
	case "mock":
		return method == "mock" || method == "alipay" || method == "wxpay"
	case "alipay_direct", "easypay_alipay", "jeepay_alipay":
		return method == "alipay"
	case "wxpay_direct", "easypay_wxpay", "jeepay_wxpay":
		return method == "wxpay"
	case "stripe":
		return method == "stripe"
	default:
		return false
	}
}

func DefaultMethodsForProviderType(providerType string) []string {
	switch providerType {
	case "wxpay_direct", "easypay_wxpay", "jeepay_wxpay":
		return []string{"wxpay"}
	case "mock":
		return []string{"mock"}
	case "stripe":
		return []string{"stripe"}
	default:
		return []string{"alipay"}
	}
}

func ProviderInstancePayload(item domaincashier.ProviderInstance) map[string]any {
	return map[string]any{
		"id":                 item.ID,
		"provider_type":      item.ProviderType,
		"name":               item.Name,
		"enabled":            item.Enabled,
		"supported_methods":  item.SupportedMethods,
		"sort_order":         item.SortOrder,
		"scheduler_weight":   item.SchedulerWeight,
		"limits":             normalizeMap(item.Limits),
		"config":             RedactProviderConfig(item.Config),
		"config_status":      item.ConfigStatus,
		"credentials_status": CredentialsStatus(item.Config, item.UpdatedAt),
		"last_error":         item.LastError,
		"created_at":         item.CreatedAt,
		"updated_at":         item.UpdatedAt,
	}
}

func RedactProviderConfig(config map[string]any) map[string]any {
	redacted := map[string]any{}
	for key, value := range normalizeMap(config) {
		if ConfigKeyIsSecret(key) {
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func CredentialsStatus(config map[string]any, updatedAt time.Time) map[string]any {
	secretMaterial := secretMaterial(config)
	if strings.TrimSpace(secretMaterial) == "" {
		return map[string]any{"has_secret": false}
	}
	sum := sha256.Sum256([]byte(secretMaterial))
	status := map[string]any{
		"has_secret":  true,
		"fingerprint": fmt.Sprintf("sha256:%x", sum[:8]),
	}
	if !updatedAt.IsZero() {
		status["updated_at"] = updatedAt
	}
	return status
}

func ConfigKeyIsSecret(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "key", "pkey", "api_v3_key", "apiv3_key", "mch_key", "merchant_key":
		return true
	default:
		return strings.Contains(key, "secret") || strings.Contains(key, "private_key") || strings.Contains(key, "token") || strings.Contains(key, "mch_key") || strings.Contains(key, "api_key")
	}
}

func ProviderInstanceForWrite(req domaincashier.ProviderInstanceWriteRequest, existingConfig map[string]any) (domaincashier.ProviderInstance, error) {
	next := req.ProviderInstance
	next.Config = MergeProviderConfigForWrite(req.Config, req.Secrets, req.ClearSecrets, existingConfig)
	if err := RejectMaskedSecrets(req.Secrets); err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	if err := ValidateProviderConfiguration(next.ProviderType, next.Config); err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	return next, nil
}

func ValidateProviderConfiguration(providerType string, config map[string]any) error {
	requirements := map[string][]string{
		"jeepay_alipay": {"gateway_url", "mch_no", "app_id", "key", "way_code"},
		"jeepay_wxpay":  {"gateway_url", "mch_no", "app_id", "key", "way_code"},
		"stripe":        {"publishable_key", "secret_key", "webhook_secret"},
	}
	required := requirements[strings.ToLower(strings.TrimSpace(providerType))]
	missing := make([]string, 0)
	for _, field := range required {
		if strings.TrimSpace(fmt.Sprint(config[field])) == "" || config[field] == nil {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return errs.New(http.StatusBadRequest, errs.CodePaymentProviderConfigInvalid, "payment provider configuration is missing required fields: "+strings.Join(missing, ", "))
}

func MergeProviderConfigForWrite(config, secrets map[string]any, clearSecrets []string, existingConfig map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range normalizeMap(config) {
		if ConfigKeyIsSecret(key) {
			if !isEmptySecretValue(value) {
				merged[key] = value
			}
			continue
		}
		merged[key] = value
	}
	for key, value := range normalizeMap(existingConfig) {
		if ConfigKeyIsSecret(key) {
			merged[key] = value
		}
	}
	for _, key := range normalizeStringList(clearSecrets) {
		if ConfigKeyIsSecret(key) {
			delete(merged, key)
		}
	}
	for key, value := range normalizeMap(secrets) {
		if ConfigKeyIsSecret(key) && !isEmptySecretValue(value) {
			merged[key] = value
		}
	}
	return merged
}

func RejectMaskedSecrets(secrets map[string]any) error {
	for key, value := range normalizeMap(secrets) {
		if !ConfigKeyIsSecret(key) {
			continue
		}
		if isMaskedSecretPlaceholder(value) {
			return fmt.Errorf("invalid_secret_placeholder")
		}
	}
	return nil
}

func isEmptySecretValue(value any) bool {
	return strings.TrimSpace(fmt.Sprint(value)) == ""
}

func isMaskedSecretPlaceholder(value any) bool {
	trimmed := strings.TrimSpace(fmt.Sprint(value))
	if len(trimmed) < 4 {
		return false
	}
	for _, char := range trimmed {
		if char != '*' && char != '•' {
			return false
		}
	}
	return true
}

func normalizeProviderLimits(limits map[string]any) error {
	for _, key := range []string{"min_amount_cny", "max_amount_cny", "daily_amount_limit_cny"} {
		raw, ok := limits[key]
		if !ok || raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
			continue
		}
		formatted, value, err := positiveDecimalString(fmt.Sprint(raw), key)
		if err != nil {
			return err
		}
		if !value.IsPositive() {
			return fmt.Errorf("%s must be positive", key)
		}
		limits[key] = formatted
	}
	if minRaw, minOK := limits["min_amount_cny"]; minOK {
		if maxRaw, maxOK := limits["max_amount_cny"]; maxOK {
			_, minValue, minErr := positiveDecimalString(fmt.Sprint(minRaw), "min_amount_cny")
			_, maxValue, maxErr := positiveDecimalString(fmt.Sprint(maxRaw), "max_amount_cny")
			if minErr != nil {
				return minErr
			}
			if maxErr != nil {
				return maxErr
			}
			if minValue.GreaterThan(maxValue) {
				return fmt.Errorf("min_amount_cny must be less than or equal to max_amount_cny")
			}
		}
	}
	return nil
}

func positiveDecimalString(raw, field string) (string, decimal.Decimal, error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !value.IsPositive() {
		return "", decimal.Zero, fmt.Errorf("%s must be positive", field)
	}
	return value.StringFixed(5), value, nil
}

func normalizeStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	normalized := make(map[string]any, len(value))
	for key, raw := range value {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = raw
	}
	return normalized
}

func secretMaterial(config map[string]any) string {
	parts := make([]string, 0)
	for key, value := range normalizeMap(config) {
		if ConfigKeyIsSecret(key) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}
