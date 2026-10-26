package entstore

import (
	"context"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/oauthrecord"
	"github.com/fatballfish/pic-gallery/internal/service/oauthserver"
)

type OAuthStore struct{ client *repoent.Client }

func NewOAuthStore(client *repoent.Client) *OAuthStore { return &OAuthStore{client} }
func (s *OAuthStore) Get(ctx context.Context, id string) (oauthserver.Record, error) {
	r, e := s.client.OAuthRecord.Get(ctx, id)
	if repoent.IsNotFound(e) {
		return oauthserver.Record{}, oauthserver.ErrNotFound
	}
	if e != nil {
		return oauthserver.Record{}, e
	}
	return oauthserver.Record{ID: r.ID, Kind: r.Kind, UserID: r.UserID, Payload: r.Payload, Version: r.Version}, nil
}
func (s *OAuthStore) Create(ctx context.Context, r oauthserver.Record) error {
	e := s.client.OAuthRecord.Create().SetID(r.ID).SetKind(r.Kind).SetUserID(r.UserID).SetPayload(r.Payload).SetVersion(1).Exec(ctx)
	if repoent.IsConstraintError(e) {
		return oauthserver.ErrConflict
	}
	return e
}
func (s *OAuthStore) CompareSwap(ctx context.Context, r oauthserver.Record) error {
	n, e := s.client.OAuthRecord.Update().Where(oauthrecord.IDEQ(r.ID), oauthrecord.VersionEQ(r.Version)).SetPayload(r.Payload).AddVersion(1).Save(ctx)
	if e != nil {
		return e
	}
	if n != 1 {
		return oauthserver.ErrConflict
	}
	return nil
}
func (s *OAuthStore) List(ctx context.Context, kind string, uid int64) ([]oauthserver.Record, error) {
	rows, e := s.client.OAuthRecord.Query().Where(oauthrecord.KindEQ(kind), oauthrecord.UserIDEQ(uid)).All(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]oauthserver.Record, 0, len(rows))
	for _, r := range rows {
		out = append(out, oauthserver.Record{ID: r.ID, Kind: r.Kind, UserID: r.UserID, Payload: r.Payload, Version: r.Version})
	}
	return out, nil
}

var _ oauthserver.Store = (*OAuthStore)(nil)
