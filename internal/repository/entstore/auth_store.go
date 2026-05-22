package entstore

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/refreshsession"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/user"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/usergroup"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type RefreshSessionRecord struct {
	ID                  string
	FamilyID            string
	UserID              int64
	RefreshTokenHash    string
	Status              string
	ExpiresAt           int64
	ReplacedBySessionID string
}

type AuthStore struct {
	client *repoent.Client
}

func NewAuthStore(client *repoent.Client) *AuthStore {
	return &AuthStore{client: client}
}

func (s *AuthStore) GetUserByEmail(ctx context.Context, email string) (domainauth.User, error) {
	entity, err := s.client.User.Query().Where(user.EmailEQ(email)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainauth.User{}, repoerr.ErrNotFound
		}
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

func (s *AuthStore) CreateUser(ctx context.Context, user domainauth.User) (domainauth.User, error) {
	groupEntity, err := s.ensureUserGroup(ctx, user.GroupCode, user.GroupMultiplier)
	if err != nil {
		return domainauth.User{}, err
	}

	entity, err := s.client.User.Create().
		SetEmail(user.Email).
		SetNillablePasswordHash(authNullableString(user.PasswordHash)).
		SetNillableEmailVerifiedAt(user.EmailVerifiedAt).
		SetNickname(user.Nickname).
		SetStatus(user.Status).
		SetUserGroupID(int64(groupEntity.ID)).
		SetRpmLimit(user.RPMLimit).
		SetConcurrencyLimit(user.ConcurrencyLimit).
		SetDefaultLocale(user.DefaultLocale).
		SetTheme(user.Theme).
		SetNillablePasswordUpdatedAt(user.PasswordUpdatedAt).
		SetNillableClosedAt(user.ClosedAt).
		Save(ctx)
	if err != nil {
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

func (s *AuthStore) GetUserByID(ctx context.Context, id int64) (domainauth.User, error) {
	entity, err := s.client.User.Get(ctx, int(id))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainauth.User{}, repoerr.ErrNotFound
		}
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

func (s *AuthStore) UpdateUser(ctx context.Context, domainUser domainauth.User) (domainauth.User, error) {
	groupEntity, err := s.ensureUserGroup(ctx, domainUser.GroupCode, domainUser.GroupMultiplier)
	if err != nil {
		return domainauth.User{}, err
	}

	update := s.client.User.UpdateOneID(int(domainUser.ID)).
		SetEmail(domainUser.Email).
		SetNickname(domainUser.Nickname).
		SetBio(domainUser.Bio).
		SetStatus(domainUser.Status).
		SetUserGroupID(int64(groupEntity.ID)).
		SetTokenVersion(domainUser.TokenVersion).
		SetRpmLimit(domainUser.RPMLimit).
		SetConcurrencyLimit(domainUser.ConcurrencyLimit).
		SetDefaultLocale(domainUser.DefaultLocale).
		SetTheme(domainUser.Theme)
	if domainUser.PasswordHash != "" {
		update.SetPasswordHash(domainUser.PasswordHash)
	}
	if domainUser.EmailVerifiedAt != nil {
		update.SetEmailVerifiedAt(*domainUser.EmailVerifiedAt)
	}
	if domainUser.PasswordUpdatedAt != nil {
		update.SetPasswordUpdatedAt(*domainUser.PasswordUpdatedAt)
	} else {
		update.ClearPasswordUpdatedAt()
	}
	if domainUser.ClosedAt != nil {
		update.SetClosedAt(*domainUser.ClosedAt)
	} else {
		update.ClearClosedAt()
	}
	if domainUser.AvatarObjectKey != "" {
		update.SetAvatarObjectKey(domainUser.AvatarObjectKey)
	} else {
		update.ClearAvatarObjectKey()
	}
	entity, err := update.Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainauth.User{}, repoerr.ErrNotFound
		}
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

func (s *AuthStore) IncrementTokenVersion(ctx context.Context, userID int64) error {
	return s.client.User.UpdateOneID(int(userID)).AddTokenVersion(1).Exec(ctx)
}

func (s *AuthStore) UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string, passwordUpdatedAt time.Time) (domainauth.User, error) {
	entity, err := s.client.User.UpdateOneID(int(userID)).
		SetPasswordHash(passwordHash).
		SetPasswordUpdatedAt(passwordUpdatedAt.UTC()).
		AddTokenVersion(1).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainauth.User{}, repoerr.ErrNotFound
		}
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

func (s *AuthStore) MarkUserClosed(ctx context.Context, userID int64, closedAt time.Time) (domainauth.User, error) {
	entity, err := s.client.User.UpdateOneID(int(userID)).
		SetStatus("closed").
		SetClosedAt(closedAt.UTC()).
		AddTokenVersion(1).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainauth.User{}, repoerr.ErrNotFound
		}
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

func (s *AuthStore) RevokeRefreshSessionsByUser(ctx context.Context, userID int64) error {
	_, err := s.client.RefreshSession.Update().
		Where(refreshsession.UserIDEQ(userID)).
		SetStatus("revoked").
		Save(ctx)
	return err
}

func (s *AuthStore) SaveRefreshSession(ctx context.Context, session RefreshSessionRecord) error {
	sessionID, err := uuid.Parse(session.ID)
	if err != nil {
		return err
	}
	familyID, err := uuid.Parse(session.FamilyID)
	if err != nil {
		return err
	}
	create := s.client.RefreshSession.Create().
		SetID(sessionID).
		SetSessionFamilyID(familyID).
		SetUserID(session.UserID).
		SetRefreshTokenHash(session.RefreshTokenHash).
		SetStatus(session.Status).
		SetExpiresAt(unixToTime(session.ExpiresAt))
	if session.ReplacedBySessionID != "" {
		replacedBy, err := uuid.Parse(session.ReplacedBySessionID)
		if err != nil {
			return err
		}
		create.SetReplacedBySessionID(replacedBy)
	}
	return create.Exec(ctx)
}

func (s *AuthStore) GetRefreshSessionByHash(ctx context.Context, tokenHash string) (RefreshSessionRecord, error) {
	entity, err := s.client.RefreshSession.Query().Where(refreshsession.RefreshTokenHashEQ(tokenHash)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return RefreshSessionRecord{}, repoerr.ErrNotFound
		}
		return RefreshSessionRecord{}, err
	}
	return mapRefreshSessionEntity(entity), nil
}

func (s *AuthStore) MarkRefreshSessionRotated(ctx context.Context, sessionID string, replacedBySessionID string) error {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	update := s.client.RefreshSession.UpdateOneID(id).SetStatus("rotated")
	if replacedBySessionID != "" {
		replacedByID, err := uuid.Parse(replacedBySessionID)
		if err != nil {
			return err
		}
		update.SetReplacedBySessionID(replacedByID)
	}
	return update.Exec(ctx)
}

func (s *AuthStore) MarkRefreshSessionExpired(ctx context.Context, sessionID string) error {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	return s.client.RefreshSession.UpdateOneID(id).SetStatus("expired").Exec(ctx)
}

func (s *AuthStore) MarkRefreshSessionRevoked(ctx context.Context, sessionID string) error {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	return s.client.RefreshSession.UpdateOneID(id).SetStatus("revoked").Exec(ctx)
}

func (s *AuthStore) MarkFamilyReplayBlocked(ctx context.Context, familyID string) error {
	parsed, err := uuid.Parse(familyID)
	if err != nil {
		return err
	}
	_, err = s.client.RefreshSession.Update().Where(refreshsession.SessionFamilyIDEQ(parsed)).SetStatus("replay_blocked").Save(ctx)
	return err
}

func (s *AuthStore) RevokeRefreshSessionByHash(ctx context.Context, tokenHash string) error {
	_, err := s.client.RefreshSession.Update().Where(refreshsession.RefreshTokenHashEQ(tokenHash)).SetStatus("revoked").Save(ctx)
	return err
}

func (s *AuthStore) UpdateUserProfile(ctx context.Context, req domainauth.UpdateProfileRequest) (domainauth.User, error) {
	update := s.client.User.UpdateOneID(int(req.UserID)).
		SetNickname(req.Nickname).
		SetBio(req.Bio).
		SetDefaultLocale(req.DefaultLocale).
		SetTheme(req.Theme)
	if req.AvatarObjectKey != "" {
		update.SetAvatarObjectKey(req.AvatarObjectKey)
	} else {
		update.ClearAvatarObjectKey()
	}
	entity, err := update.Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainauth.User{}, repoerr.ErrNotFound
		}
		return domainauth.User{}, err
	}
	return s.mapUserEntity(ctx, entity)
}

func (s *AuthStore) mapUserEntity(ctx context.Context, entity *repoent.User) (domainauth.User, error) {
	if entity == nil {
		return domainauth.User{}, nil
	}

	groupCode := "basic"
	groupMultiplier := "1.00000"
	if entity.UserGroupID > 0 {
		groupEntity, err := s.client.UserGroup.Query().Where(usergroup.IDEQ(int(entity.UserGroupID))).Only(ctx)
		if err != nil && !repoent.IsNotFound(err) {
			return domainauth.User{}, err
		}
		if err == nil {
			groupCode = groupEntity.GroupCode
			groupMultiplier = groupEntity.Multiplier
		}
	}

	avatar := ""
	if entity.AvatarObjectKey != nil {
		avatar = *entity.AvatarObjectKey
	}
	return domainauth.User{
		ID:                int64(entity.ID),
		Email:             entity.Email,
		PasswordHash:      authStringValue(entity.PasswordHash),
		Nickname:          entity.Nickname,
		Bio:               entity.Bio,
		AvatarObjectKey:   avatar,
		Status:            entity.Status,
		GroupCode:         groupCode,
		GroupMultiplier:   groupMultiplier,
		TokenVersion:      entity.TokenVersion,
		RPMLimit:          entity.RpmLimit,
		ConcurrencyLimit:  entity.ConcurrencyLimit,
		DefaultLocale:     entity.DefaultLocale,
		Theme:             entity.Theme,
		EmailVerifiedAt:   entity.EmailVerifiedAt,
		PasswordUpdatedAt: entity.PasswordUpdatedAt,
		ClosedAt:          entity.ClosedAt,
		CreatedAt:         entity.CreatedAt,
	}, nil
}

func (s *AuthStore) ensureUserGroup(ctx context.Context, code string, multiplier string) (*repoent.UserGroup, error) {
	if code == "" {
		code = "basic"
	}
	if multiplier == "" {
		multiplier = "1.00000"
	}

	groupEntity, err := s.client.UserGroup.Query().Where(usergroup.GroupCodeEQ(code)).Only(ctx)
	if err == nil {
		return groupEntity, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, err
	}

	return s.client.UserGroup.Create().
		SetGroupCode(code).
		SetGroupName(code).
		SetMultiplier(multiplier).
		SetStatus("active").
		Save(ctx)
}

func mapRefreshSessionEntity(entity *repoent.RefreshSession) RefreshSessionRecord {
	record := RefreshSessionRecord{
		ID:               entity.ID.String(),
		FamilyID:         entity.SessionFamilyID.String(),
		UserID:           entity.UserID,
		RefreshTokenHash: entity.RefreshTokenHash,
		Status:           entity.Status,
		ExpiresAt:        entity.ExpiresAt.Unix(),
	}
	if entity.ReplacedBySessionID != nil {
		record.ReplacedBySessionID = entity.ReplacedBySessionID.String()
	}
	return record
}

func unixToTime(value int64) (t time.Time) {
	return time.Unix(value, 0).UTC()
}

func authStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func authNullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
