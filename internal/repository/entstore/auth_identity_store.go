package entstore

import (
	"context"

	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/useridentity"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

func mapUserIdentityEntity(entity *repoent.UserIdentity) domainauth.UserIdentity {
	return domainauth.UserIdentity{
		ID:        int64(entity.ID),
		UserID:    entity.UserID,
		Provider:  entity.Provider,
		Subject:   entity.Subject,
		Email:     entity.Email,
		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt,
	}
}

func (s *AuthStore) FindUserIdentity(ctx context.Context, provider, subject string) (domainauth.UserIdentity, error) {
	entity, err := s.client.UserIdentity.Query().
		Where(useridentity.ProviderEQ(provider), useridentity.SubjectEQ(subject)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainauth.UserIdentity{}, repoerr.ErrNotFound
		}
		return domainauth.UserIdentity{}, err
	}
	return mapUserIdentityEntity(entity), nil
}

// CreateUserWithIdentity creates the user (default project included) and the
// federated identity in one transaction.
func (s *AuthStore) CreateUserWithIdentity(ctx context.Context, user domainauth.User, identity domainauth.UserIdentity) (domainauth.User, error) {
	groupEntity, err := s.ensureUserGroup(ctx, user.GroupCode, user.GroupMultiplier)
	if err != nil {
		return domainauth.User{}, err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainauth.User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	entity, err := createUserEntityInTx(ctx, tx, user, int64(groupEntity.ID))
	if err != nil {
		return domainauth.User{}, err
	}
	if _, err := tx.UserIdentity.Create().
		SetUserID(int64(entity.ID)).
		SetProvider(identity.Provider).
		SetSubject(identity.Subject).
		SetEmail(identity.Email).
		Save(ctx); err != nil {
		return domainauth.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

// AttachUserIdentity links an existing user to a federated identity. A repeat
// call for the same (provider, subject) only refreshes the email snapshot and
// never rebinds the identity to another user.
func (s *AuthStore) AttachUserIdentity(ctx context.Context, identity domainauth.UserIdentity) (domainauth.UserIdentity, error) {
	existing, err := s.FindUserIdentity(ctx, identity.Provider, identity.Subject)
	if err == nil {
		if existing.Email != identity.Email {
			updated, err := s.client.UserIdentity.UpdateOneID(int(existing.ID)).SetEmail(identity.Email).Save(ctx)
			if err != nil {
				return domainauth.UserIdentity{}, err
			}
			return mapUserIdentityEntity(updated), nil
		}
		return existing, nil
	}
	if err != repoerr.ErrNotFound {
		return domainauth.UserIdentity{}, err
	}
	created, err := s.client.UserIdentity.Create().
		SetUserID(identity.UserID).
		SetProvider(identity.Provider).
		SetSubject(identity.Subject).
		SetEmail(identity.Email).
		Save(ctx)
	if err != nil {
		return domainauth.UserIdentity{}, err
	}
	return mapUserIdentityEntity(created), nil
}
