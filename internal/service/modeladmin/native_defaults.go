package modeladmin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// Validate only the new defaults subtree; preserve all other extension fields.
func validateNativeDefaults(extra map[string]any, adapter, model string) error {
	raw, ok := extra["native_image"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return errs.BadRequest("invalid native image defaults")
	}
	var node struct {
		Model    string                `json:"model"`
		Defaults *nativeimage.Defaults `json:"defaults"`
	}
	if err = json.Unmarshal(data, &node); err != nil {
		return errs.BadRequest(fmt.Sprintf("invalid native image defaults: %v", err))
	}
	if node.Defaults == nil {
		return nil
	}
	if node.Model != "" {
		model = node.Model
	}
	if model == "" {
		if adapter == nativeimage.AdapterQwen {
			model = "qwen-image-3.0"
		}
		if adapter == nativeimage.AdapterSeedream {
			model = "doubao-seedream-5-0-pro-260628"
		}
	}
	profile, ok := nativeimage.ProfileForModel(model)
	if !ok || profile.Adapter != adapter {
		return errs.BadRequest("native defaults require a matching official model")
	}
	if err = node.Defaults.Validate(profile); err != nil {
		return errs.BadRequest(err.Error())
	}
	return nil
}

func (s *Service) validateModelNativeDefaults(ctx context.Context, accountID int64, model string, extra map[string]any) error {
	account, err := s.store.GetModelAccount(ctx, accountID)
	if err != nil {
		return normalizeStoreError(err, "model account not found")
	}
	candidate := modelhub.ProviderCandidate{AdapterType: account.AdapterType, ModelCode: model, AccountExtra: account.Extra, ModelExtra: extra}
	if !modelhub.IsNativeImageCandidate(candidate) {
		return nil
	}
	// Only enforce the new contract when defaults have been supplied at either level.
	hasDefaults := false
	for _, source := range []map[string]any{account.Extra, extra} {
		data, err := json.Marshal(source["native_image"])
		if err != nil {
			return errs.BadRequest("invalid native image defaults")
		}
		var node map[string]json.RawMessage
		if json.Unmarshal(data, &node) == nil && node["defaults"] != nil {
			hasDefaults = true
		}
	}
	if !hasDefaults {
		return nil
	}
	if _, err := modelhub.NativeConfig(candidate); err != nil {
		return errs.BadRequest(err.Error())
	}
	return nil
}
