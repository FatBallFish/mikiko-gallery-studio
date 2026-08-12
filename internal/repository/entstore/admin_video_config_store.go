package entstore

import (
	"context"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelcapability"
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
	return adminvideo.CostRuleSummary{ID: int64(row.ID), AccountModelID: row.AccountModelID, BillingMode: row.BillingMode, RuleVersion: row.RuleVersion, Currency: row.Currency, Rates: row.RatesJSON, Validation: row.ValidationStatus, EffectiveAt: row.EffectiveAt, ExpiresAt: row.ExpiresAt, Enabled: row.Enabled}, nil
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
	return adminvideo.PricingStrategySummary{ID: int64(row.ID), Code: row.Code, Name: row.Name, StrategyVersion: row.StrategyVersion, MinimumNetPointIncomeCNY: row.MinimumNetPointIncomeCny, TargetMarginRate: row.TargetMarginRate, ProviderCostBufferRate: row.ProviderCostBufferRate, PaymentFeeRate: row.PaymentFeeRate, PlatformFixedCostCNY: row.PlatformFixedCostCny, PlatformOutputSecondCostCNY: row.PlatformOutputSecondCostCny, PlatformReferenceCostCNY: row.PlatformReferenceCostCny, Enabled: row.Enabled}, nil
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
	builder := client.VideoPriceRule.Create().SetPricingStrategyID(input.StrategyID).SetTaskType(input.TaskType).SetResolution(input.Resolution).SetAudioMode(input.AudioMode).SetRuleVersion(input.ExpectedVersion + 1).SetEffectiveAt(input.EffectiveAt).SetOutputSecondPoints(input.OutputSecondPoints).SetFixedTaskPoints(input.FixedTaskPoints).SetReferenceImagePoints(input.ReferenceImagePoints).SetInputVideoSecondPoints(input.InputVideoSecondPoints).SetReferenceAudioSecondPoints(input.ReferenceAudioSecondPoints).SetGeneratedAudioFixedPoints(input.GeneratedAudioFixedPoints).SetGeneratedAudioSecondPoints(input.GeneratedAudioSecondPoints).SetMinimumBillableSeconds(input.MinimumBillableSeconds).SetMinimumTaskPoints(input.MinimumTaskPoints).SetReserveMarkup(input.ReserveMarkup).SetSafetyPoints(input.SafetyPoints).SetCandidateCostUpperCny(input.CandidateCostUpperCNY).SetSafetySnapshot(input.SafetySnapshot).SetEnabled(input.Enabled).SetInternalNote(input.InternalNote)
	if input.ExpiresAt != nil {
		builder.SetExpiresAt(*input.ExpiresAt)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return adminvideo.PriceRuleSummary{}, err
	}
	return adminvideo.PriceRuleSummary{ID: int64(row.ID), StrategyID: row.PricingStrategyID, TaskType: row.TaskType, Resolution: row.Resolution, AudioMode: row.AudioMode, RuleVersion: row.RuleVersion, SafetyPoints: row.SafetyPoints, SalesPoints: row.MinimumTaskPoints, CandidateCostUpperCNY: row.CandidateCostUpperCny, Enabled: row.Enabled}, nil
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
		row, err = client.VideoRouteConfig.Create().SetRouteModelID(input.RouteModelID).SetTaskTypes(input.TaskTypes).SetVisibleOptions(visible).SetDefaults(input.Defaults).SetMaxOutputCount(input.MaxOutputCount).SetPricingStrategyID(input.PricingStrategyID).SetConfigVersion(input.ConfigVersion).SetEnabled(input.Enabled).Save(ctx)
	} else if err == nil {
		if row.ConfigVersion != input.ExpectedVersion {
			return adminvideo.RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "video route config version conflict")
		}
		row, err = row.Update().SetTaskTypes(input.TaskTypes).SetVisibleOptions(visible).SetDefaults(input.Defaults).SetMaxOutputCount(input.MaxOutputCount).SetPricingStrategyID(input.PricingStrategyID).SetConfigVersion(input.ConfigVersion).SetEnabled(input.Enabled).Save(ctx)
	}
	if err != nil {
		return adminvideo.RouteConfigSummary{}, err
	}
	return adminvideo.RouteConfigSummary{RouteModelID: row.RouteModelID, ConfigVersion: row.ConfigVersion, PricingStrategyID: row.PricingStrategyID, TaskTypes: row.TaskTypes, VisibleOptions: row.VisibleOptions, Defaults: row.Defaults, MaxOutputCount: row.MaxOutputCount, Enabled: row.Enabled}, nil
}

func cloneAnyMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source)+1)
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
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
