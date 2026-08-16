package video

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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
