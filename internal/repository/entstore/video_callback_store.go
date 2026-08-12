package entstore

import (
	"context"

	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videoprovidercallbackevent"
	videocallbackservice "github.com/fatballfish/pic-gallery/internal/service/videocallback"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type VideoCallbackStore struct{ client *repoent.Client }

func NewVideoCallbackStore(client *repoent.Client) *VideoCallbackStore {
	return &VideoCallbackStore{client: client}
}

func (s *VideoCallbackStore) RecordEvent(ctx context.Context, record videocallbackservice.EventRecord) (bool, error) {
	_, err := s.client.VideoProviderCallbackEvent.Create().
		SetProviderCode(record.ProviderCode).
		SetModelAccountID(record.ModelAccountID).
		SetProviderEventID(record.ProviderEventID).
		SetProviderJobID(record.ProviderJobID).
		SetStatus("received").
		SetPayloadSnapshot(record.PayloadSnapshot).
		SetReceivedAt(record.ReceivedAt).
		Save(ctx)
	if err == nil {
		return false, nil
	}
	if !repoent.IsConstraintError(err) {
		return false, err
	}
	exists, queryErr := s.client.VideoProviderCallbackEvent.Query().Where(
		videoprovidercallbackevent.ModelAccountIDEQ(record.ModelAccountID),
		videoprovidercallbackevent.ProviderEventIDEQ(record.ProviderEventID),
	).Exist(ctx)
	if queryErr != nil {
		return false, queryErr
	}
	if exists {
		return true, nil
	}
	return false, err
}

type VideoCallbackAccount struct {
	ModelAccountID int64
	AdapterType    string
	BaseURL        string
	Credentials    map[string]string
	TimeoutMS      int
	Extra          map[string]any
}

func (s *VideoCallbackStore) GetCallbackAccount(ctx context.Context, publicID uuid.UUID, providerCode string) (videocallbackservice.Account, error) {
	account, err := s.client.ModelAccount.Query().Where(
		modelaccount.PublicIDEQ(publicID), modelaccount.AdapterTypeEQ(providerCode), modelaccount.DeletedAtIsNil(),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return videocallbackservice.Account{}, errs.New(404, errs.CodeNotFound, "video callback endpoint not found")
	}
	if err != nil {
		return videocallbackservice.Account{}, err
	}
	return videocallbackservice.Account{
		ModelAccountID: int64(account.ID), AdapterType: account.AdapterType, BaseURL: account.BaseURL,
		Credentials: account.CredentialsEncrypted, TimeoutMS: account.TimeoutMs, Extra: account.Extra,
	}, nil
}
