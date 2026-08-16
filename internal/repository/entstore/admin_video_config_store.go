package entstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccountmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelcandidate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelcapability"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelratecard"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videorouteconfig"
	adminvideo "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func (s *AdminVideoStore) SaveCapability(ctx context.Context, input adminvideo.CapabilityWrite) (adminvideo.CapabilitySummary, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (adminvideo.CapabilitySummary, error) {
		return saveVideoCapability(ctx, tx.Client(), input)
	})
}

func saveVideoCapability(ctx context.Context, client *repoent.Client, input adminvideo.CapabilityWrite) (adminvideo.CapabilitySummary, error) {
	row, err := client.VideoModelCapability.Query().Where(videomodelcapability.AccountModelIDEQ(input.AccountModelID), videomodelcapability.DeletedAtIsNil()).Only(ctx)
	if repoent.IsNotFound(err) {
		if input.ExpectedVersion != "" {
			return adminvideo.CapabilitySummary{}, errs.New(409, errs.CodeConflict, "video capability version conflict")
		}
		row, err = client.VideoModelCapability.Create().SetAccountModelID(input.AccountModelID).SetCapabilityVersion(input.CapabilityVersion).SetCapabilityJSON(input.Capability).SetValidationStatus(input.ValidationStatus).SetEnabled(input.Enabled).Save(ctx)
	} else if err == nil {
		if row.CapabilityVersion != input.ExpectedVersion {
			return adminvideo.CapabilitySummary{}, errs.New(409, errs.CodeConflict, "video capability version conflict")
		}
		row, err = row.Update().SetCapabilityVersion(input.CapabilityVersion).SetCapabilityJSON(input.Capability).SetValidationStatus(input.ValidationStatus).SetEnabled(input.Enabled).Save(ctx)
	}
	if err != nil {
		return adminvideo.CapabilitySummary{}, err
	}
	return adminvideo.CapabilitySummary{AccountModelID: row.AccountModelID, Version: row.CapabilityVersion, ValidationState: row.ValidationStatus, Capability: row.CapabilityJSON, Enabled: row.Enabled}, nil
}

func (s *AdminVideoStore) ListVideoModelRateCards(ctx context.Context, accountModelID int64) ([]adminvideo.RateCardSummary, error) {
	rows, err := s.client.VideoModelRateCard.Query().Where(
		videomodelratecard.AccountModelIDEQ(accountModelID),
		videomodelratecard.DeletedAtIsNil(),
	).Order(repoent.Asc(videomodelratecard.FieldRateVersion)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]adminvideo.RateCardSummary, 0, len(rows))
	for _, row := range rows {
		result = append(result, projectVideoModelRateCard(row))
	}
	return result, nil
}

func (s *AdminVideoStore) SaveVideoModelRateCard(ctx context.Context, input adminvideo.RateCardWrite) (adminvideo.RateCardSummary, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (adminvideo.RateCardSummary, error) {
		query := tx.VideoModelRateCard.Query().Where(
			videomodelratecard.AccountModelIDEQ(input.AccountModelID),
			videomodelratecard.DeletedAtIsNil(),
		).Order(repoent.Desc(videomodelratecard.FieldRateVersion))
		current, err := query.First(ctx)
		if repoent.IsNotFound(err) {
			current = nil
			err = nil
		}
		if err != nil {
			return adminvideo.RateCardSummary{}, err
		}
		currentVersion := 0
		if current != nil {
			currentVersion = current.RateVersion
		}
		if currentVersion != input.ExpectedRateVersion {
			return adminvideo.RateCardSummary{}, errs.New(409, errs.CodeConflict, "video rate card version conflict")
		}
		if current != nil && current.Enabled {
			if _, err := current.Update().SetEnabled(false).Save(ctx); err != nil {
				return adminvideo.RateCardSummary{}, err
			}
		}
		effectiveAt := input.EffectiveAt
		if effectiveAt.IsZero() {
			effectiveAt = time.Now().UTC()
		}
		currency := input.Currency
		if currency == "" {
			currency = "CNY"
		}
		row, err := tx.VideoModelRateCard.Create().
			SetAccountModelID(input.AccountModelID).
			SetProviderCode(input.ProviderCode).
			SetPricingSchema(input.PricingSchema).
			SetRateVersion(currentVersion + 1).
			SetCurrency(currency).
			SetRateConfig(deepCloneAnyMap(input.RateConfig)).
			SetSourceReference(input.SourceReference).
			SetEffectiveAt(effectiveAt).
			SetEnabled(input.Enabled).
			Save(ctx)
		if err != nil {
			return adminvideo.RateCardSummary{}, err
		}
		return projectVideoModelRateCard(row), nil
	})
}

func (s *AdminVideoStore) DeleteVideoModelRateCard(ctx context.Context, id int64, expectedVersion int) error {
	row, err := s.client.VideoModelRateCard.Query().Where(
		videomodelratecard.IDEQ(int(id)),
		videomodelratecard.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return err
	}
	if row.RateVersion != expectedVersion {
		return errs.New(409, errs.CodeConflict, "video rate card version conflict")
	}
	_, err = row.Update().SetEnabled(false).SetDeletedAt(time.Now().UTC()).Save(ctx)
	return err
}

func (s *AdminVideoStore) GetEffectiveVideoModelRateCard(ctx context.Context, accountModelID int64, at time.Time) (adminvideo.RateCardSummary, error) {
	row, err := s.client.VideoModelRateCard.Query().Where(
		videomodelratecard.AccountModelIDEQ(accountModelID),
		videomodelratecard.EnabledEQ(true),
		videomodelratecard.EffectiveAtLTE(at),
		videomodelratecard.DeletedAtIsNil(),
	).Order(repoent.Desc(videomodelratecard.FieldRateVersion)).First(ctx)
	if err != nil {
		return adminvideo.RateCardSummary{}, err
	}
	return projectVideoModelRateCard(row), nil
}

func projectVideoModelRateCard(row *repoent.VideoModelRateCard) adminvideo.RateCardSummary {
	return adminvideo.RateCardSummary{
		ID: int64(row.ID), AccountModelID: row.AccountModelID, ProviderCode: row.ProviderCode,
		PricingSchema: row.PricingSchema, RateVersion: row.RateVersion, Currency: row.Currency,
		RateConfig: deepCloneAnyMap(row.RateConfig), SourceReference: row.SourceReference,
		EffectiveAt: row.EffectiveAt, Enabled: row.Enabled,
	}
}

func (s *AdminVideoStore) GetVideoModelPricingContext(ctx context.Context, accountModelID int64) (adminvideo.ModelPricingContext, error) {
	model, err := s.client.ModelAccountModel.Query().Where(
		modelaccountmodel.IDEQ(int(accountModelID)), modelaccountmodel.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return adminvideo.ModelPricingContext{}, err
	}
	account, err := s.client.ModelAccount.Query().Where(
		modelaccount.IDEQ(int(model.AccountID)), modelaccount.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return adminvideo.ModelPricingContext{}, err
	}
	capability, err := s.client.VideoModelCapability.Query().Where(
		videomodelcapability.AccountModelIDEQ(accountModelID), videomodelcapability.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return adminvideo.ModelPricingContext{}, err
	}
	return adminvideo.ModelPricingContext{
		AccountModelID: accountModelID, ProviderCode: account.AdapterType,
		ModelCode: model.ModelCode, Capability: deepCloneAnyMap(capability.CapabilityJSON),
	}, nil
}

func (s *AdminVideoStore) GetVideoRouteQuoteContext(ctx context.Context, routeModelID int64, at time.Time) (adminvideo.RouteQuoteContext, error) {
	config, err := s.client.VideoRouteConfig.Query().Where(
		videorouteconfig.RouteModelIDEQ(routeModelID), videorouteconfig.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return adminvideo.RouteQuoteContext{}, err
	}
	result := adminvideo.RouteQuoteContext{Route: adminvideo.RouteConfigSummary{
		RouteModelID: routeModelID, ConfigVersion: config.ConfigVersion,
		CandidateParameterMappings: deepCloneAnyMap(config.CandidateParameterMappings),
		MinimumTaskPoints:          config.MinimumTaskPoints, RoundingStepPoints: config.RoundingStepPoints,
		TaskTypes: append([]string(nil), config.TaskTypes...), VisibleOptions: deepCloneAnyMap(config.VisibleOptions),
		Defaults: deepCloneAnyMap(config.Defaults), MaxOutputCount: config.MaxOutputCount, Enabled: config.Enabled,
	}}
	rows, err := s.client.RouteModelCandidate.Query().Where(
		routemodelcandidate.RouteModelIDEQ(routeModelID), routemodelcandidate.EnabledEQ(true), routemodelcandidate.DeletedAtIsNil(),
	).Order(repoent.Asc(routemodelcandidate.FieldPriority), repoent.Asc(routemodelcandidate.FieldFallbackOrder)).All(ctx)
	if err != nil {
		return adminvideo.RouteQuoteContext{}, err
	}
	for _, candidate := range rows {
		quoteCandidate := adminvideo.RouteQuoteCandidate{
			RouteCandidateID:   int64(candidate.ID),
			AccountModelID:     candidate.AccountModelID,
			ResolutionMappings: decodeCandidateResolutionMappings(config.CandidateParameterMappings, candidate.AccountModelID),
		}
		model, modelErr := s.client.ModelAccountModel.Query().Where(
			modelaccountmodel.IDEQ(int(candidate.AccountModelID)), modelaccountmodel.DeletedAtIsNil(),
		).Only(ctx)
		if repoent.IsNotFound(modelErr) {
			quoteCandidate.PreflightExclusionCode = errs.CodeVideoCandidateNotPriceable
			result.Candidates = append(result.Candidates, quoteCandidate)
			continue
		}
		if modelErr != nil {
			return adminvideo.RouteQuoteContext{}, modelErr
		}
		quoteCandidate.ModelCode = model.ModelCode
		if !model.Enabled {
			quoteCandidate.PreflightExclusionCode = errs.CodeVideoCandidateNotPriceable
		}
		account, accountErr := s.client.ModelAccount.Query().Where(
			modelaccount.IDEQ(int(model.AccountID)), modelaccount.DeletedAtIsNil(),
		).Only(ctx)
		if repoent.IsNotFound(accountErr) {
			quoteCandidate.PreflightExclusionCode = errs.CodeVideoCandidateNotPriceable
			result.Candidates = append(result.Candidates, quoteCandidate)
			continue
		}
		if accountErr != nil {
			return adminvideo.RouteQuoteContext{}, accountErr
		}
		quoteCandidate.ProviderCode = account.AdapterType
		if account.Status != "enabled" {
			quoteCandidate.PreflightExclusionCode = errs.CodeVideoCandidateNotPriceable
		}
		capabilityRow, capabilityErr := s.client.VideoModelCapability.Query().Where(
			videomodelcapability.AccountModelIDEQ(candidate.AccountModelID), videomodelcapability.EnabledEQ(true),
			videomodelcapability.ValidationStatusEQ("verified"), videomodelcapability.DeletedAtIsNil(),
		).Only(ctx)
		if capabilityErr == nil {
			quoteCandidate.CapabilityVersion = capabilityRow.CapabilityVersion
			quoteCandidate.Capability = deepCloneAnyMap(capabilityRow.CapabilityJSON)
		} else if !repoent.IsNotFound(capabilityErr) {
			return adminvideo.RouteQuoteContext{}, capabilityErr
		}
		rateCard, rateErr := s.GetEffectiveVideoModelRateCard(ctx, candidate.AccountModelID, at)
		if rateErr != nil && !repoent.IsNotFound(rateErr) {
			return adminvideo.RouteQuoteContext{}, rateErr
		}
		quoteCandidate.RateCard = rateCard
		result.Candidates = append(result.Candidates, quoteCandidate)
	}
	result.CNYPerPoint, result.ConversionVersion, err = s.videoCNYPerPoint(ctx)
	if err != nil {
		return adminvideo.RouteQuoteContext{}, err
	}
	return result, nil
}

func decodeCandidateResolutionMappings(value map[string]any, accountModelID int64) map[string]string {
	result := map[string]string{}
	key := strconv.FormatInt(accountModelID, 10)
	candidateValue, ok := value[key]
	if !ok {
		return result
	}
	payload, err := json.Marshal(candidateValue)
	if err != nil {
		return result
	}
	var decoded struct {
		Resolutions map[string]string `json:"resolutions"`
	}
	if json.Unmarshal(payload, &decoded) != nil {
		return result
	}
	for source, target := range decoded.Resolutions {
		result[source] = target
	}
	return result
}

func (s *AdminVideoStore) videoCNYPerPoint(ctx context.Context) (string, string, error) {
	row, err := s.client.ConfigItem.Query().Where(
		configitem.ConfigCategoryEQ("billing_pricing"), configitem.ConfigKeyEQ("cny_per_point"), configitem.ScopeEQ("global"),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return "0.01000", "billing-default", nil
	}
	if err != nil {
		return "", "", err
	}
	value, ok := configScalarString(row.ConfigValue["value"])
	if !ok {
		return "", "", fmt.Errorf("billing_pricing.cny_per_point is invalid")
	}
	return value, fmt.Sprintf("billing-config-v%d", row.Version), nil
}

func configScalarString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, typed != ""
	case json.Number:
		return typed.String(), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	default:
		return "", false
	}
}

func (s *AdminVideoStore) SaveRouteConfig(ctx context.Context, input adminvideo.RouteConfigWrite) (adminvideo.RouteConfigSummary, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (adminvideo.RouteConfigSummary, error) {
		return saveVideoRouteConfig(ctx, tx.Client(), input)
	})
}

func saveVideoRouteConfig(ctx context.Context, client *repoent.Client, input adminvideo.RouteConfigWrite) (adminvideo.RouteConfigSummary, error) {
	row, err := client.VideoRouteConfig.Query().Where(videorouteconfig.RouteModelIDEQ(input.RouteModelID), videorouteconfig.DeletedAtIsNil()).Only(ctx)
	visible := cloneAnyMap(input.VisibleOptions)
	visible["combinations"] = input.VisibleCombinations
	if repoent.IsNotFound(err) {
		if input.ExpectedVersion != "" {
			return adminvideo.RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "video route config version conflict")
		}
		row, err = client.VideoRouteConfig.Create().SetRouteModelID(input.RouteModelID).SetTaskTypes(input.TaskTypes).SetVisibleOptions(visible).SetDefaults(input.Defaults).SetMaxOutputCount(input.MaxOutputCount).SetCandidateParameterMappings(deepCloneAnyMap(input.CandidateParameterMappings)).SetMinimumTaskPoints(normalizeMinimumTaskPoints(input.MinimumTaskPoints)).SetRoundingStepPoints(normalizeRoundingStep(input.RoundingStepPoints)).SetConfigVersion(input.ConfigVersion).SetEnabled(input.Enabled).Save(ctx)
	} else if err == nil {
		if row.ConfigVersion != input.ExpectedVersion {
			return adminvideo.RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "video route config version conflict")
		}
		row, err = row.Update().SetTaskTypes(input.TaskTypes).SetVisibleOptions(visible).SetDefaults(input.Defaults).SetMaxOutputCount(input.MaxOutputCount).SetCandidateParameterMappings(deepCloneAnyMap(input.CandidateParameterMappings)).SetMinimumTaskPoints(normalizeMinimumTaskPoints(input.MinimumTaskPoints)).SetRoundingStepPoints(normalizeRoundingStep(input.RoundingStepPoints)).SetConfigVersion(input.ConfigVersion).SetEnabled(input.Enabled).Save(ctx)
	}
	if err != nil {
		return adminvideo.RouteConfigSummary{}, err
	}
	return adminvideo.RouteConfigSummary{RouteModelID: row.RouteModelID, ConfigVersion: row.ConfigVersion, CandidateParameterMappings: deepCloneAnyMap(row.CandidateParameterMappings), MinimumTaskPoints: row.MinimumTaskPoints, RoundingStepPoints: row.RoundingStepPoints, TaskTypes: row.TaskTypes, VisibleOptions: row.VisibleOptions, Defaults: row.Defaults, MaxOutputCount: row.MaxOutputCount, Enabled: row.Enabled}, nil
}

func cloneAnyMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source)+1)
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func deepCloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = deepCloneAnyValue(value)
	}
	return cloned
}

func deepCloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCloneAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = deepCloneAnyValue(item)
		}
		return cloned
	default:
		return value
	}
}

func normalizeMinimumTaskPoints(value string) string {
	if value == "" {
		return "0.00000"
	}
	return value
}

func normalizeRoundingStep(value int) int {
	if value == 0 {
		return 1
	}
	return value
}

func (s *AdminVideoStore) DeleteVideoConfig(ctx context.Context, kind adminvideo.ConfigKind, id int64, expected int64) error {
	now := time.Now().UTC()
	switch kind {
	case adminvideo.ConfigCapability:
		row, err := s.client.VideoModelCapability.Query().Where(videomodelcapability.AccountModelIDEQ(id), videomodelcapability.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return err
		}
		_, err = row.Update().SetEnabled(false).SetDeletedAt(now).Save(ctx)
		return err
	case adminvideo.ConfigRoute:
		row, err := s.client.VideoRouteConfig.Query().Where(videorouteconfig.RouteModelIDEQ(id), videorouteconfig.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return err
		}
		_, err = row.Update().SetEnabled(false).SetDeletedAt(now).Save(ctx)
		return err
	default:
		return errs.BadRequest("unknown video config kind")
	}
}
