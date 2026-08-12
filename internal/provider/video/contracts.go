package video

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

type Input struct {
	AssetID         string `json:"asset_id"`
	Role            string `json:"role"`
	URL             string `json:"url"`
	Ordinal         int    `json:"ordinal"`
	StorageConfigID string `json:"-"`
	StorageDriver   string `json:"-"`
	ObjectKey       string `json:"-"`
	MIMEType        string `json:"-"`
}

type Request struct {
	TaskID          string         `json:"task_id"`
	ItemID          string         `json:"item_id"`
	AttemptID       string         `json:"attempt_id"`
	IdempotencyKey  string         `json:"idempotency_key"`
	TaskType        string         `json:"task_type"`
	Prompt          string         `json:"prompt"`
	DurationSeconds int            `json:"duration_seconds"`
	Resolution      string         `json:"resolution"`
	AspectRatio     string         `json:"aspect_ratio"`
	GenerateAudio   bool           `json:"generate_audio"`
	OutputFormat    string         `json:"output_format"`
	Inputs          []Input        `json:"inputs,omitempty"`
	ProviderOptions map[string]any `json:"provider_options,omitempty"`
}

type Job struct {
	ID        string `json:"id"`
	State     State  `json:"state"`
	RequestID string `json:"request_id,omitempty"`
}

type JobRef struct {
	ID string `json:"id"`
}

type Artifact struct {
	URL       string     `json:"url"`
	MIMEType  string     `json:"mime_type,omitempty"`
	SizeBytes int64      `json:"size_bytes,omitempty"`
	SHA256    string     `json:"sha256,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type Status struct {
	JobID        string         `json:"job_id"`
	State        State          `json:"state"`
	Artifacts    []Artifact     `json:"artifacts,omitempty"`
	Usage        map[string]any `json:"usage,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Raw          map[string]any `json:"raw,omitempty"`
}

type CancelResult struct {
	Accepted bool  `json:"accepted"`
	State    State `json:"state"`
}

type CallbackEvent struct {
	Challenge string `json:"challenge,omitempty"`
	EventID   string `json:"event_id,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	Status    Status `json:"status"`
}

type Usage struct {
	OutputSeconds       string         `json:"output_seconds"`
	InputVideoSeconds   string         `json:"input_video_seconds"`
	ReferenceImageCount int            `json:"reference_image_count"`
	ProviderTokens      string         `json:"provider_tokens"`
	Raw                 map[string]any `json:"raw,omitempty"`
}

type Provider interface {
	Submit(context.Context, Request) (Job, error)
	Get(context.Context, JobRef) (Status, error)
	Cancel(context.Context, JobRef) (CancelResult, error)
	VerifyCallback(context.Context, http.Header, []byte) (CallbackEvent, error)
	NormalizeUsage(Status) (Usage, error)
}

type ValidationStatus string

const (
	ValidationUntested ValidationStatus = "untested"
	ValidationValid    ValidationStatus = "valid"
	ValidationInvalid  ValidationStatus = "invalid"
)

type ExecutionMode string

const (
	ExecutionModePoll         ExecutionMode = "poll"
	ExecutionModeCallbackPoll ExecutionMode = "callback_poll"
)

type CapabilityContract struct {
	SchemaVersion int                       `json:"schema_version"`
	ContractID    string                    `json:"contract_id"`
	Models        []ProviderModelCapability `json:"models"`
}

type ProviderModelCapability struct {
	ProviderCode       string           `json:"provider_code"`
	ModelCode          string           `json:"model_code"`
	DisplayName        string           `json:"display_name"`
	ValidationStatus   ValidationStatus `json:"validation_status"`
	ExecutionMode      ExecutionMode    `json:"execution_mode"`
	TaskTypes          []string         `json:"task_types"`
	InputFormats       []string         `json:"input_formats"`
	OutputFormats      []string         `json:"output_formats"`
	ProviderNativeMaxN int              `json:"provider_native_max_n"`
	SourceReference    string           `json:"source_reference"`
	Notes              []string         `json:"notes,omitempty"`
}

func ParseCapabilityContract(raw []byte) (CapabilityContract, error) {
	var contract CapabilityContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		return CapabilityContract{}, fmt.Errorf("decode capability contract: %w", err)
	}
	if contract.SchemaVersion != 1 {
		return CapabilityContract{}, fmt.Errorf("unsupported capability contract schema_version %d", contract.SchemaVersion)
	}
	if strings.TrimSpace(contract.ContractID) == "" {
		return CapabilityContract{}, fmt.Errorf("capability contract_id is required")
	}
	seen := make(map[string]struct{}, len(contract.Models))
	for index, model := range contract.Models {
		if err := validateProviderModelCapability(model); err != nil {
			return CapabilityContract{}, fmt.Errorf("models[%d]: %w", index, err)
		}
		key := strings.ToLower(strings.TrimSpace(model.ProviderCode)) + "/" + strings.ToLower(strings.TrimSpace(model.ModelCode))
		if _, ok := seen[key]; ok {
			return CapabilityContract{}, fmt.Errorf("models[%d]: duplicate provider/model %q", index, key)
		}
		seen[key] = struct{}{}
	}
	return contract, nil
}

func validateProviderModelCapability(model ProviderModelCapability) error {
	if strings.TrimSpace(model.ProviderCode) == "" || strings.TrimSpace(model.ModelCode) == "" {
		return fmt.Errorf("provider_code and model_code are required")
	}
	switch model.ValidationStatus {
	case ValidationUntested, ValidationValid, ValidationInvalid:
	default:
		return fmt.Errorf("unsupported validation_status %q", model.ValidationStatus)
	}
	switch model.ExecutionMode {
	case ExecutionModePoll, ExecutionModeCallbackPoll:
	default:
		return fmt.Errorf("unsupported execution_mode %q", model.ExecutionMode)
	}
	if len(model.TaskTypes) == 0 || len(model.OutputFormats) == 0 {
		return fmt.Errorf("task_types and output_formats are required")
	}
	if model.ProviderNativeMaxN < 1 || model.ProviderNativeMaxN > 10 {
		return fmt.Errorf("provider_native_max_n must be between 1 and 10")
	}
	if strings.TrimSpace(model.SourceReference) == "" {
		return fmt.Errorf("source_reference is required")
	}
	return nil
}

type PricingContract struct {
	SchemaVersion               int                `json:"schema_version"`
	ContractID                  string             `json:"contract_id"`
	GrossPointValueCNY          string             `json:"gross_point_value_cny"`
	MaximumBonusRatio           string             `json:"maximum_bonus_ratio"`
	PaymentFeeRate              string             `json:"payment_fee_rate"`
	MinimumNetPointIncomeCNY    string             `json:"minimum_net_point_income_cny"`
	TargetGrossMarginRate       string             `json:"target_gross_margin_rate"`
	ProviderCostBufferRate      string             `json:"provider_cost_buffer_rate"`
	PlatformFixedCostCNY        string             `json:"platform_fixed_cost_cny"`
	PlatformOutputSecondCostCNY string             `json:"platform_output_second_cost_cny"`
	PlatformReferenceCostCNY    string             `json:"platform_reference_cost_cny"`
	InitialSalesRates           []InitialSalesRate `json:"initial_sales_rates"`
}

type InitialSalesRate struct {
	ProviderCode       string           `json:"provider_code"`
	ModelCode          string           `json:"model_code"`
	Resolution         string           `json:"resolution"`
	PricingMode        string           `json:"pricing_mode"`
	OutputSecondPoints string           `json:"output_second_points"`
	FixedTaskPoints    string           `json:"fixed_task_points"`
	ReserveMarkup      string           `json:"reserve_markup"`
	ValidationStatus   ValidationStatus `json:"validation_status"`
	Enabled            bool             `json:"enabled"`
	SourceReference    string           `json:"source_reference"`
}

func ParsePricingContract(raw []byte) (PricingContract, error) {
	var contract PricingContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		return PricingContract{}, fmt.Errorf("decode pricing contract: %w", err)
	}
	if contract.SchemaVersion != 1 {
		return PricingContract{}, fmt.Errorf("unsupported pricing contract schema_version %d", contract.SchemaVersion)
	}
	if strings.TrimSpace(contract.ContractID) == "" {
		return PricingContract{}, fmt.Errorf("pricing contract_id is required")
	}
	protectedDecimals := map[string]string{
		"gross_point_value_cny":           contract.GrossPointValueCNY,
		"maximum_bonus_ratio":             contract.MaximumBonusRatio,
		"payment_fee_rate":                contract.PaymentFeeRate,
		"minimum_net_point_income_cny":    contract.MinimumNetPointIncomeCNY,
		"target_gross_margin_rate":        contract.TargetGrossMarginRate,
		"provider_cost_buffer_rate":       contract.ProviderCostBufferRate,
		"platform_fixed_cost_cny":         contract.PlatformFixedCostCNY,
		"platform_output_second_cost_cny": contract.PlatformOutputSecondCostCNY,
		"platform_reference_cost_cny":     contract.PlatformReferenceCostCNY,
	}
	for name, value := range protectedDecimals {
		if _, err := parseNonNegativeDecimal(value); err != nil {
			return PricingContract{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	seen := make(map[string]struct{}, len(contract.InitialSalesRates))
	for index, rate := range contract.InitialSalesRates {
		if strings.TrimSpace(rate.ProviderCode) == "" || strings.TrimSpace(rate.ModelCode) == "" || strings.TrimSpace(rate.Resolution) == "" {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d]: provider_code, model_code and resolution are required", index)
		}
		if rate.PricingMode != "exact" && rate.PricingMode != "metered" {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d]: unsupported pricing_mode %q", index, rate.PricingMode)
		}
		if _, err := parseNonNegativeDecimal(rate.OutputSecondPoints); err != nil {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d].output_second_points: %w", index, err)
		}
		if _, err := parseNonNegativeDecimal(rate.FixedTaskPoints); err != nil {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d].fixed_task_points: %w", index, err)
		}
		markup, err := parseNonNegativeDecimal(rate.ReserveMarkup)
		if err != nil || markup.LessThan(decimal.NewFromInt(1)) || markup.GreaterThan(decimal.NewFromInt(2)) {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d].reserve_markup must be between 1 and 2", index)
		}
		if rate.ValidationStatus != ValidationUntested && rate.ValidationStatus != ValidationValid && rate.ValidationStatus != ValidationInvalid {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d]: unsupported validation_status %q", index, rate.ValidationStatus)
		}
		if rate.ValidationStatus != ValidationValid && rate.Enabled {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d]: only a validated rate can be enabled", index)
		}
		if strings.TrimSpace(rate.SourceReference) == "" {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d].source_reference is required", index)
		}
		key := strings.ToLower(rate.ProviderCode + "/" + rate.ModelCode + "/" + rate.Resolution)
		if _, ok := seen[key]; ok {
			return PricingContract{}, fmt.Errorf("initial_sales_rates[%d]: duplicate rate %q", index, key)
		}
		seen[key] = struct{}{}
	}
	return contract, nil
}

func parseNonNegativeDecimal(value string) (decimal.Decimal, error) {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid decimal %q", value)
	}
	if parsed.IsNegative() {
		return decimal.Zero, fmt.Errorf("decimal must not be negative")
	}
	return parsed, nil
}
