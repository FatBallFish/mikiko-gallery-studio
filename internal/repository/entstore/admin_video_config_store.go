package entstore

import (
	"context"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelcapability"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelratecard"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videopricerule"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videopricingstrategy"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videoprovidercostrule"
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

func (s *AdminVideoStore) SaveCostRule(ctx context.Context, input adminvideo.CostRuleWrite) (adminvideo.CostRuleSummary, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (adminvideo.CostRuleSummary, error) {
		return saveVideoCostRule(ctx, tx.Client(), input)
	})
}

func saveVideoCostRule(ctx context.Context, client *repoent.Client, input adminvideo.CostRuleWrite) (adminvideo.CostRuleSummary, error) {
	if input.ID > 0 {
		current, err := client.VideoProviderCostRule.Query().Where(videoprovidercostrule.IDEQ(int(input.ID)), videoprovidercostrule.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return adminvideo.CostRuleSummary{}, err
		}
		if current.RuleVersion != input.ExpectedRuleVersion {
			return adminvideo.CostRuleSummary{}, errs.New(409, errs.CodeConflict, "video cost rule version conflict")
		}
		_, err = current.Update().SetEnabled(false).Save(ctx)
		if err != nil {
			return adminvideo.CostRuleSummary{}, err
		}
	}
	next := input.ExpectedRuleVersion + 1
	builder := client.VideoProviderCostRule.Create().SetAccountModelID(input.AccountModelID).SetBillingMode(input.BillingMode).SetRuleVersion(next).SetCurrency(input.Currency).SetRatesJSON(input.Rates).SetCostReserveMarkup(input.CostReserveMarkup).SetSourceType(input.SourceType).SetSourceReference(input.SourceReference).SetValidationStatus(input.ValidationStatus).SetEffectiveAt(input.EffectiveAt).SetEnabled(input.Enabled)
	if input.ExpiresAt != nil {
		builder.SetExpiresAt(*input.ExpiresAt)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return adminvideo.CostRuleSummary{}, err
	}
	return adminvideo.CostRuleSummary{ID: int64(row.ID), AccountModelID: row.AccountModelID, BillingMode: row.BillingMode, RuleVersion: row.RuleVersion, Currency: row.Currency, Rates: row.RatesJSON, CostReserveMarkup: row.CostReserveMarkup, SourceType: row.SourceType, SourceReference: row.SourceReference, Validation: row.ValidationStatus, EffectiveAt: row.EffectiveAt, ExpiresAt: row.ExpiresAt, Enabled: row.Enabled}, nil
}

func (s *AdminVideoStore) SaveStrategy(ctx context.Context, input adminvideo.StrategyWrite) (adminvideo.PricingStrategySummary, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (adminvideo.PricingStrategySummary, error) {
		return saveVideoStrategy(ctx, tx.Client(), input)
	})
}

func saveVideoStrategy(ctx context.Context, client *repoent.Client, input adminvideo.StrategyWrite) (adminvideo.PricingStrategySummary, error) {
	if input.ID > 0 {
		current, err := client.VideoPricingStrategy.Query().Where(videopricingstrategy.IDEQ(int(input.ID)), videopricingstrategy.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return adminvideo.PricingStrategySummary{}, err
		}
		if current.StrategyVersion != input.ExpectedVersion {
			return adminvideo.PricingStrategySummary{}, errs.New(409, errs.CodeConflict, "video pricing strategy version conflict")
		}
		if _, err = current.Update().SetEnabled(false).Save(ctx); err != nil {
			return adminvideo.PricingStrategySummary{}, err
		}
	}
	row, err := client.VideoPricingStrategy.Create().SetCode(input.Code).SetName(input.Name).SetGrossPointValueCny(input.GrossPointValueCNY).SetMinimumNetPointIncomeCny(input.MinimumNetPointIncomeCNY).SetMaxBonusRatio(input.MaxBonusRatio).SetPaymentFeeRate(input.PaymentFeeRate).SetTargetMarginRate(input.TargetMarginRate).SetProviderCostBufferRate(input.ProviderCostBufferRate).SetPlatformFixedCostCny(input.PlatformFixedCostCNY).SetPlatformOutputSecondCostCny(input.PlatformOutputSecondCostCNY).SetPlatformReferenceCostCny(input.PlatformReferenceCostCNY).SetPlatformAudioFixedCostCny(input.PlatformAudioFixedCostCNY).SetPlatformAudioSecondCostCny(input.PlatformAudioSecondCostCNY).SetExactReserveMarkup(input.ExactReserveMarkup).SetMeteredReserveMarkup(input.MeteredReserveMarkup).SetStrategyVersion(input.ExpectedVersion + 1).SetEnabled(input.Enabled).Save(ctx)
	if err != nil {
		return adminvideo.PricingStrategySummary{}, err
	}
	return adminvideo.PricingStrategySummary{ID: int64(row.ID), Code: row.Code, Name: row.Name, StrategyVersion: row.StrategyVersion, GrossPointValueCNY: row.GrossPointValueCny, MinimumNetPointIncomeCNY: row.MinimumNetPointIncomeCny, MaxBonusRatio: row.MaxBonusRatio, TargetMarginRate: row.TargetMarginRate, ProviderCostBufferRate: row.ProviderCostBufferRate, PaymentFeeRate: row.PaymentFeeRate, PlatformFixedCostCNY: row.PlatformFixedCostCny, PlatformOutputSecondCostCNY: row.PlatformOutputSecondCostCny, PlatformReferenceCostCNY: row.PlatformReferenceCostCny, PlatformAudioFixedCostCNY: row.PlatformAudioFixedCostCny, PlatformAudioSecondCostCNY: row.PlatformAudioSecondCostCny, ExactReserveMarkup: row.ExactReserveMarkup, MeteredReserveMarkup: row.MeteredReserveMarkup, Enabled: row.Enabled}, nil
}

func (s *AdminVideoStore) SavePriceRule(ctx context.Context, input adminvideo.PriceRuleWrite) (adminvideo.PriceRuleSummary, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (adminvideo.PriceRuleSummary, error) {
		return saveVideoPriceRule(ctx, tx.Client(), input)
	})
}

func saveVideoPriceRule(ctx context.Context, client *repoent.Client, input adminvideo.PriceRuleWrite) (adminvideo.PriceRuleSummary, error) {
	if input.ID > 0 {
		current, err := client.VideoPriceRule.Query().Where(videopricerule.IDEQ(int(input.ID)), videopricerule.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return adminvideo.PriceRuleSummary{}, err
		}
		if current.RuleVersion != input.ExpectedVersion {
			return adminvideo.PriceRuleSummary{}, errs.New(409, errs.CodeConflict, "video price rule version conflict")
		}
		if _, err = current.Update().SetEnabled(false).Save(ctx); err != nil {
			return adminvideo.PriceRuleSummary{}, err
		}
	}
	builder := client.VideoPriceRule.Create().SetPricingStrategyID(input.StrategyID).SetTaskType(input.TaskType).SetResolution(input.Resolution).SetAudioMode(input.AudioMode).SetPricingMode(input.PricingMode).SetRuleVersion(input.ExpectedVersion + 1).SetEffectiveAt(input.EffectiveAt).SetOutputSecondPoints(input.OutputSecondPoints).SetFixedTaskPoints(input.FixedTaskPoints).SetReferenceImagePoints(input.ReferenceImagePoints).SetInputVideoSecondPoints(input.InputVideoSecondPoints).SetReferenceAudioSecondPoints(input.ReferenceAudioSecondPoints).SetGeneratedAudioFixedPoints(input.GeneratedAudioFixedPoints).SetGeneratedAudioSecondPoints(input.GeneratedAudioSecondPoints).SetMinimumBillableSeconds(input.MinimumBillableSeconds).SetMinimumTaskPoints(input.MinimumTaskPoints).SetReserveMarkup(input.ReserveMarkup).SetSafetyPoints(input.SafetyPoints).SetCandidateCostUpperCny(input.CandidateCostUpperCNY).SetSafetySnapshot(input.SafetySnapshot).SetEnabled(input.Enabled).SetInternalNote(input.InternalNote)
	if input.ExpiresAt != nil {
		builder.SetExpiresAt(*input.ExpiresAt)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return adminvideo.PriceRuleSummary{}, err
	}
	return adminvideo.PriceRuleSummary{ID: int64(row.ID), StrategyID: row.PricingStrategyID, TaskType: row.TaskType, Resolution: row.Resolution, AudioMode: row.AudioMode, PricingMode: row.PricingMode, RuleVersion: row.RuleVersion, EffectiveAt: row.EffectiveAt, ExpiresAt: row.ExpiresAt, OutputSecondPoints: row.OutputSecondPoints, FixedTaskPoints: row.FixedTaskPoints, ReferenceImagePoints: row.ReferenceImagePoints, InputVideoSecondPoints: row.InputVideoSecondPoints, ReferenceAudioSecondPoints: row.ReferenceAudioSecondPoints, GeneratedAudioFixedPoints: row.GeneratedAudioFixedPoints, GeneratedAudioSecondPoints: row.GeneratedAudioSecondPoints, MinimumBillableSeconds: row.MinimumBillableSeconds, MinimumTaskPoints: row.MinimumTaskPoints, ReserveMarkup: row.ReserveMarkup, SafetyPoints: row.SafetyPoints, SalesPoints: row.MinimumTaskPoints, CandidateCostUpperCNY: row.CandidateCostUpperCny, Enabled: row.Enabled}, nil
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
	case adminvideo.ConfigCostRule:
		row, err := s.client.VideoProviderCostRule.Query().Where(videoprovidercostrule.IDEQ(int(id)), videoprovidercostrule.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return err
		}
		if int64(row.RuleVersion) != expected {
			return errs.New(409, errs.CodeConflict, "video cost rule version conflict")
		}
		_, err = row.Update().SetEnabled(false).SetDeletedAt(now).Save(ctx)
		return err
	case adminvideo.ConfigStrategy:
		row, err := s.client.VideoPricingStrategy.Query().Where(videopricingstrategy.IDEQ(int(id)), videopricingstrategy.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return err
		}
		if int64(row.StrategyVersion) != expected {
			return errs.New(409, errs.CodeConflict, "video strategy version conflict")
		}
		_, err = row.Update().SetEnabled(false).SetDeletedAt(now).Save(ctx)
		return err
	case adminvideo.ConfigPriceRule:
		row, err := s.client.VideoPriceRule.Query().Where(videopricerule.IDEQ(int(id)), videopricerule.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return err
		}
		if int64(row.RuleVersion) != expected {
			return errs.New(409, errs.CodeConflict, "video price rule version conflict")
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
