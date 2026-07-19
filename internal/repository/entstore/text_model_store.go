package entstore

import (
	"context"
	"fmt"

	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/textmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/textmodelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type TextModelStore struct {
	client *repoent.Client
}

func NewTextModelStore(client *repoent.Client) *TextModelStore {
	return &TextModelStore{client: client}
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
