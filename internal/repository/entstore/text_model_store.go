package entstore

import (
	"context"
	"fmt"
	"time"

	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/promptoptimizationrun"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/textmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/textmodelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/google/uuid"
)

type TextModelStore struct {
	client *repoent.Client
}

func NewTextModelStore(client *repoent.Client) *TextModelStore {
	return &TextModelStore{client: client}
}

func (s *TextModelStore) ListAccounts(ctx context.Context) ([]domaintextmodel.AccountRecord, error) {
	rows, err := s.client.TextModelAccount.Query().Where(textmodelaccount.DeletedAtIsNil()).Order(repoent.Asc(textmodelaccount.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domaintextmodel.AccountRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapTextModelAccount(row))
	}
	return result, nil
}

func (s *TextModelStore) GetAccount(ctx context.Context, accountID int64) (domaintextmodel.AccountRecord, error) {
	row, err := s.client.TextModelAccount.Query().Where(textmodelaccount.IDEQ(int(accountID)), textmodelaccount.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domaintextmodel.AccountRecord{}, repoerr.ErrNotFound
		}
		return domaintextmodel.AccountRecord{}, err
	}
	return mapTextModelAccount(row), nil
}

func (s *TextModelStore) CreateAccount(ctx context.Context, record domaintextmodel.AccountRecord) (domaintextmodel.AccountRecord, error) {
	row, err := s.client.TextModelAccount.Create().
		SetName(record.Name).
		SetPlatformType(record.PlatformType).
		SetAPIStyle(record.APIStyle).
		SetBaseURL(record.BaseURL).
		SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
		SetSecretFingerprint(record.SecretFingerprint).
		SetEnabled(record.Enabled).
		SetVersion(defaultInt64(record.Version, 1)).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domaintextmodel.AccountRecord{}, repoerr.ErrConflict
		}
		return domaintextmodel.AccountRecord{}, err
	}
	return mapTextModelAccount(row), nil
}

func (s *TextModelStore) UpdateAccount(ctx context.Context, record domaintextmodel.AccountRecord) (domaintextmodel.AccountRecord, error) {
	row, err := s.client.TextModelAccount.UpdateOneID(int(record.ID)).
		SetName(record.Name).
		SetPlatformType(record.PlatformType).
		SetAPIStyle(record.APIStyle).
		SetBaseURL(record.BaseURL).
		SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
		SetSecretFingerprint(record.SecretFingerprint).
		SetEnabled(record.Enabled).
		SetVersion(record.Version).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domaintextmodel.AccountRecord{}, repoerr.ErrNotFound
		}
		return domaintextmodel.AccountRecord{}, err
	}
	return mapTextModelAccount(row), nil
}

func (s *TextModelStore) DeleteAccount(ctx context.Context, accountID int64) error {
	models, err := s.client.TextModel.Query().Where(textmodel.AccountIDEQ(accountID), textmodel.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return err
	}
	if models > 0 {
		return repoerr.ErrConflict
	}
	affected, err := s.client.TextModelAccount.Update().Where(textmodelaccount.IDEQ(int(accountID)), textmodelaccount.DeletedAtIsNil()).SetDeletedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func (s *TextModelStore) ListModels(ctx context.Context, accountID int64) ([]domaintextmodel.Model, error) {
	query := s.client.TextModel.Query().Where(textmodel.DeletedAtIsNil())
	if accountID > 0 {
		query = query.Where(textmodel.AccountIDEQ(accountID))
	}
	rows, err := query.Order(repoent.Asc(textmodel.FieldAccountID), repoent.Asc(textmodel.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domaintextmodel.Model, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapTextModel(row))
	}
	return result, nil
}

func (s *TextModelStore) GetModel(ctx context.Context, modelID int64) (domaintextmodel.Model, error) {
	row, err := s.client.TextModel.Query().Where(textmodel.IDEQ(int(modelID)), textmodel.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domaintextmodel.Model{}, repoerr.ErrNotFound
		}
		return domaintextmodel.Model{}, err
	}
	return mapTextModel(row), nil
}

func (s *TextModelStore) CreateModel(ctx context.Context, model domaintextmodel.Model) (domaintextmodel.Model, error) {
	row, err := s.client.TextModel.Create().
		SetAccountID(model.AccountID).
		SetModelCode(model.ModelCode).
		SetDisplayName(model.DisplayName).
		SetInputPricePerMillionTokens(model.InputPricePerMTok).
		SetOutputPricePerMillionTokens(model.OutputPricePerMTok).
		SetCurrency(model.Currency).
		SetEnabled(model.Enabled).
		SetIsDefault(model.IsDefault).
		SetVersion(defaultInt64(model.Version, 1)).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domaintextmodel.Model{}, repoerr.ErrConflict
		}
		return domaintextmodel.Model{}, err
	}
	return mapTextModel(row), nil
}

func (s *TextModelStore) UpdateModel(ctx context.Context, model domaintextmodel.Model) (domaintextmodel.Model, error) {
	row, err := s.client.TextModel.UpdateOneID(int(model.ID)).
		SetAccountID(model.AccountID).
		SetModelCode(model.ModelCode).
		SetDisplayName(model.DisplayName).
		SetInputPricePerMillionTokens(model.InputPricePerMTok).
		SetOutputPricePerMillionTokens(model.OutputPricePerMTok).
		SetCurrency(model.Currency).
		SetEnabled(model.Enabled).
		SetIsDefault(model.IsDefault && model.Enabled).
		SetVersion(model.Version).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domaintextmodel.Model{}, repoerr.ErrNotFound
		}
		if repoent.IsConstraintError(err) {
			return domaintextmodel.Model{}, repoerr.ErrConflict
		}
		return domaintextmodel.Model{}, err
	}
	return mapTextModel(row), nil
}

func (s *TextModelStore) DeleteModel(ctx context.Context, modelID int64) error {
	affected, err := s.client.TextModel.Update().Where(textmodel.IDEQ(int(modelID)), textmodel.DeletedAtIsNil()).SetDeletedAt(time.Now().UTC()).SetIsDefault(false).Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func (s *TextModelStore) SetDefaultModel(ctx context.Context, modelID int64) (domaintextmodel.Model, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domaintextmodel.Model{}, err
	}
	rollback := func(cause error) (domaintextmodel.Model, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return domaintextmodel.Model{}, fmt.Errorf("%w (rollback: %v)", cause, rollbackErr)
		}
		return domaintextmodel.Model{}, cause
	}
	target, err := tx.TextModel.Query().Where(
		textmodel.IDEQ(int(modelID)),
		textmodel.DeletedAtIsNil(),
		textmodel.EnabledEQ(true),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return rollback(repoerr.ErrNotFound)
		}
		return rollback(err)
	}
	accountExists, err := tx.TextModelAccount.Query().Where(
		textmodelaccount.IDEQ(int(target.AccountID)),
		textmodelaccount.DeletedAtIsNil(),
		textmodelaccount.EnabledEQ(true),
	).Exist(ctx)
	if err != nil {
		return rollback(err)
	}
	if !accountExists {
		return rollback(repoerr.ErrConflict)
	}
	if _, err := tx.TextModel.Update().Where(textmodel.DeletedAtIsNil(), textmodel.IsDefaultEQ(true)).SetIsDefault(false).Save(ctx); err != nil {
		return rollback(err)
	}
	updated, err := tx.TextModel.UpdateOneID(int(modelID)).SetIsDefault(true).SetVersion(target.Version + 1).Save(ctx)
	if err != nil {
		return rollback(err)
	}
	result := mapTextModel(updated)
	if err := tx.Commit(); err != nil {
		return domaintextmodel.Model{}, err
	}
	return result, nil
}

func (s *TextModelStore) GetDefaultModel(ctx context.Context) (domaintextmodel.AccountRecord, domaintextmodel.Model, error) {
	modelRow, err := s.client.TextModel.Query().Where(
		textmodel.DeletedAtIsNil(),
		textmodel.EnabledEQ(true),
		textmodel.IsDefaultEQ(true),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domaintextmodel.AccountRecord{}, domaintextmodel.Model{}, repoerr.ErrNotFound
		}
		return domaintextmodel.AccountRecord{}, domaintextmodel.Model{}, err
	}
	accountRow, err := s.client.TextModelAccount.Query().Where(
		textmodelaccount.IDEQ(int(modelRow.AccountID)),
		textmodelaccount.DeletedAtIsNil(),
		textmodelaccount.EnabledEQ(true),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domaintextmodel.AccountRecord{}, domaintextmodel.Model{}, repoerr.ErrNotFound
		}
		return domaintextmodel.AccountRecord{}, domaintextmodel.Model{}, err
	}
	return mapTextModelAccount(accountRow), mapTextModel(modelRow), nil
}

func (s *TextModelStore) SaveOptimizationRun(ctx context.Context, run domaintextmodel.OptimizationRun) (domaintextmodel.OptimizationRun, error) {
	id, err := uuid.Parse(run.ID)
	if err != nil {
		id = uuid.New()
	}
	current, queryErr := s.client.PromptOptimizationRun.Query().Where(promptoptimizationrun.IDEQ(id)).Only(ctx)
	if queryErr != nil && !repoent.IsNotFound(queryErr) {
		return domaintextmodel.OptimizationRun{}, queryErr
	}
	if repoent.IsNotFound(queryErr) {
		row, createErr := s.client.PromptOptimizationRun.Create().
			SetID(id).
			SetUserID(run.UserID).
			SetAccountID(run.AccountID).
			SetModelID(run.ModelID).
			SetModelCode(run.ModelCode).
			SetAPIStyle(run.APIStyle).
			SetConfigVersion(run.ConfigVersion).
			SetPromptSha256(run.PromptSHA256).
			SetStatus(run.Status).
			SetInputTokens(run.InputTokens).
			SetOutputTokens(run.OutputTokens).
			SetEstimatedPoints(run.EstimatedPoints).
			SetActualPoints(run.ActualPoints).
			SetProviderRequestID(run.ProviderRequestID).
			SetErrorCode(run.ErrorCode).
			SetErrorMessage(run.ErrorMessage).
			SetMetadata(cloneConfigValue(run.Metadata)).
			Save(ctx)
		if createErr != nil {
			return domaintextmodel.OptimizationRun{}, createErr
		}
		return mapPromptOptimizationRun(row), nil
	}
	row, err := s.client.PromptOptimizationRun.UpdateOneID(current.ID).
		SetStatus(run.Status).
		SetInputTokens(run.InputTokens).
		SetOutputTokens(run.OutputTokens).
		SetActualPoints(run.ActualPoints).
		SetProviderRequestID(run.ProviderRequestID).
		SetErrorCode(run.ErrorCode).
		SetErrorMessage(run.ErrorMessage).
		SetMetadata(cloneConfigValue(run.Metadata)).
		Save(ctx)
	if err != nil {
		return domaintextmodel.OptimizationRun{}, err
	}
	return mapPromptOptimizationRun(row), nil
}

func mapTextModelAccount(row *repoent.TextModelAccount) domaintextmodel.AccountRecord {
	if row == nil {
		return domaintextmodel.AccountRecord{}
	}
	return domaintextmodel.AccountRecord{
		ID:                int64(row.ID),
		Name:              row.Name,
		PlatformType:      row.PlatformType,
		APIStyle:          row.APIStyle,
		BaseURL:           row.BaseURL,
		SecretEncrypted:   cloneConfigValue(row.SecretEncrypted),
		SecretFingerprint: row.SecretFingerprint,
		Enabled:           row.Enabled,
		Version:           row.Version,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		DeletedAt:         row.DeletedAt,
	}
}

func mapTextModel(row *repoent.TextModel) domaintextmodel.Model {
	if row == nil {
		return domaintextmodel.Model{}
	}
	return domaintextmodel.Model{
		ID:                 int64(row.ID),
		AccountID:          row.AccountID,
		ModelCode:          row.ModelCode,
		DisplayName:        row.DisplayName,
		InputPricePerMTok:  row.InputPricePerMillionTokens,
		OutputPricePerMTok: row.OutputPricePerMillionTokens,
		Currency:           row.Currency,
		Enabled:            row.Enabled,
		IsDefault:          row.IsDefault,
		Version:            row.Version,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
		DeletedAt:          row.DeletedAt,
	}
}

func mapPromptOptimizationRun(row *repoent.PromptOptimizationRun) domaintextmodel.OptimizationRun {
	if row == nil {
		return domaintextmodel.OptimizationRun{}
	}
	return domaintextmodel.OptimizationRun{
		ID: row.ID.String(), UserID: row.UserID, AccountID: row.AccountID, ModelID: row.ModelID,
		ModelCode: row.ModelCode, APIStyle: row.APIStyle, ConfigVersion: row.ConfigVersion,
		PromptSHA256: row.PromptSha256, Status: row.Status, InputTokens: row.InputTokens,
		OutputTokens: row.OutputTokens, EstimatedPoints: row.EstimatedPoints, ActualPoints: row.ActualPoints,
		ProviderRequestID: row.ProviderRequestID, ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage,
		Metadata: cloneConfigValue(row.Metadata), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
