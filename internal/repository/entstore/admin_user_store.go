package entstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/user"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/usergroup"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type AdminUserStore struct {
	client  *repoent.Client
	billing *BillingStore
}

func NewAdminUserStore(client *repoent.Client, billing *BillingStore) *AdminUserStore {
	if billing == nil && client != nil {
		billing = NewBillingStore(client, 5)
	}
	return &AdminUserStore{client: client, billing: billing}
}

func (s *AdminUserStore) ListUsers(ctx context.Context, req domainadminuser.ListRequest) (domainadminuser.ListPage, error) {
	page, pageSize := normalizeAdminUserPage(req.Page, req.PageSize)
	query := s.client.User.Query()
	query = query.Where(user.DeletedAtIsNil())
	if status := strings.TrimSpace(req.Status); status != "" {
		query = query.Where(user.StatusEQ(status))
	}
	if q := strings.TrimSpace(req.Query); q != "" {
		query = query.Where(user.Or(user.EmailContainsFold(q), user.NicknameContainsFold(q)))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainadminuser.ListPage{}, err
	}
	entities, err := query.Order(repoent.Desc(user.FieldCreatedAt), repoent.Desc(user.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return domainadminuser.ListPage{}, err
	}
	items := make([]domainadminuser.UserSummary, 0, len(entities))
	for _, entity := range entities {
		item, err := s.mapUser(ctx, entity)
		if err != nil {
			return domainadminuser.ListPage{}, err
		}
		items = append(items, item)
	}
	return domainadminuser.ListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *AdminUserStore) CreateUser(ctx context.Context, req domainadminuser.CreateUserRequest) (domainadminuser.UserSummary, error) {
	group, err := s.ensureUserGroup(ctx, req.UserGroupCode)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	create := s.client.User.Create().
		SetEmail(req.Email).
		SetNickname(req.Nickname).
		SetStatus(req.Status).
		SetUserGroupID(int64(group.ID)).
		SetRpmLimit(req.RPMLimit).
		SetConcurrencyLimit(req.ConcurrencyLimit).
		SetDefaultLocale(req.DefaultLocale).
		SetTheme(req.Theme)
	entity, err := create.Save(ctx)
	if err != nil {
		if isConstraintError(err) {
			return domainadminuser.UserSummary{}, repoerr.ErrConflict
		}
		return domainadminuser.UserSummary{}, err
	}
	return s.mapUser(ctx, entity)
}

func (s *AdminUserStore) GetUserDetail(ctx context.Context, userID int64, recentLedgerLimit int) (domainadminuser.Detail, error) {
	entity, err := s.client.User.Query().Where(user.IDEQ(int(userID)), user.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.Detail{}, errs.New(404, errs.CodeNotFound, "user not found")
		}
		return domainadminuser.Detail{}, err
	}
	item, err := s.mapUser(ctx, entity)
	if err != nil {
		return domainadminuser.Detail{}, err
	}
	balance := billingservice.BalanceState{AvailablePoints: "0.00000", FrozenPoints: "0.00000"}
	if s.billing != nil {
		balance, err = s.billing.GetBalance(ctx, userID)
		if err != nil {
			return domainadminuser.Detail{}, err
		}
	}
	if recentLedgerLimit <= 0 {
		recentLedgerLimit = 10
	}
	ledgers, err := s.client.PointLedger.Query().
		Where(pointledger.UserIDEQ(userID)).
		Order(repoent.Desc(pointledger.FieldID)).
		Limit(recentLedgerLimit).
		All(ctx)
	if err != nil {
		return domainadminuser.Detail{}, err
	}
	recent := make([]domainbilling.LedgerEntry, 0, len(ledgers))
	for _, entry := range ledgers {
		recent = append(recent, mapLedgerEntry(entry))
	}
	return domainadminuser.Detail{
		User: item,
		Balance: domainbilling.BalanceSummary{
			AvailablePoints:     balance.AvailablePoints,
			FrozenPoints:        balance.FrozenPoints,
			UserGroupMultiplier: "1.00000",
			CNYPerPoint:         "0.00000",
		},
		RecentLedger: recent,
	}, nil
}

func (s *AdminUserStore) ensureUserGroup(ctx context.Context, groupCode string) (*repoent.UserGroup, error) {
	groupCode = strings.ToLower(strings.TrimSpace(groupCode))
	if groupCode == "" {
		groupCode = "basic"
	}
	group, err := s.client.UserGroup.Query().Where(usergroup.GroupCodeEQ(groupCode)).Only(ctx)
	if err == nil {
		return group, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, err
	}
	if groupCode != "basic" {
		return nil, errs.New(404, errs.CodeNotFound, "user group not found")
	}
	return s.client.UserGroup.Create().
		SetGroupCode("basic").
		SetGroupName("basic").
		SetMultiplier("1.00000").
		SetStatus("active").
		Save(ctx)
}

func (s *AdminUserStore) UpdateUserStatus(ctx context.Context, userID int64, status string) (domainadminuser.UserSummary, error) {
	entity, err := s.client.User.Query().Where(user.IDEQ(int(userID)), user.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
		}
		return domainadminuser.UserSummary{}, err
	}
	update := s.client.User.Update().Where(user.IDEQ(int(userID)), user.DeletedAtIsNil()).SetStatus(status)
	if entity.Status != status {
		update.AddTokenVersion(1)
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	if affected == 0 {
		return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	return s.GetUserSummary(ctx, userID)
}

func (s *AdminUserStore) UpdateUserLimits(ctx context.Context, req domainadminuser.LimitsRequest) (domainadminuser.UserSummary, error) {
	affected, err := s.client.User.Update().
		Where(user.IDEQ(int(req.UserID)), user.DeletedAtIsNil()).
		SetRpmLimit(req.RPMLimit).
		SetConcurrencyLimit(req.ConcurrencyLimit).
		Save(ctx)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	if affected == 0 {
		return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	return s.GetUserSummary(ctx, req.UserID)
}

func (s *AdminUserStore) AssignUserGroup(ctx context.Context, req domainadminuser.GroupAssignmentRequest) (domainadminuser.UserSummary, error) {
	group, err := s.client.UserGroup.Query().Where(usergroup.GroupCodeEQ(req.UserGroupCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user group not found")
		}
		return domainadminuser.UserSummary{}, err
	}
	affected, err := s.client.User.Update().Where(user.IDEQ(int(req.UserID)), user.DeletedAtIsNil()).SetUserGroupID(int64(group.ID)).Save(ctx)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	if affected == 0 {
		return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	return s.GetUserSummary(ctx, req.UserID)
}

func (s *AdminUserStore) GetUserSummary(ctx context.Context, userID int64) (domainadminuser.UserSummary, error) {
	entity, err := s.client.User.Query().Where(user.IDEQ(int(userID)), user.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
		}
		return domainadminuser.UserSummary{}, err
	}
	return s.mapUser(ctx, entity)
}

func (s *AdminUserStore) DeleteUser(ctx context.Context, userID int64) (domainadminuser.UserSummary, error) {
	entity, err := s.client.User.Query().Where(user.IDEQ(int(userID)), user.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
		}
		return domainadminuser.UserSummary{}, err
	}
	deletedAt := time.Now().UTC()
	deletedEmail := fmt.Sprintf("deleted+%d+%d@deleted.local", entity.ID, deletedAt.UnixNano())
	affected, err := s.client.User.Update().
		Where(user.IDEQ(entity.ID), user.DeletedAtIsNil()).
		SetEmail(deletedEmail).
		SetStatus("closed").
		SetDeletedAt(deletedAt).
		SetClosedAt(deletedAt).
		AddTokenVersion(1).
		Save(ctx)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	if affected == 0 {
		return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	entity.Email = deletedEmail
	entity.Status = "closed"
	entity.DeletedAt = &deletedAt
	entity.ClosedAt = &deletedAt
	entity.TokenVersion++
	return s.mapUser(ctx, entity)
}

func (s *AdminUserStore) ListUserGroups(ctx context.Context, req domainadminuser.UserGroupListRequest) (domainadminuser.UserGroupListPage, error) {
	page, pageSize := normalizeAdminUserPage(req.Page, req.PageSize)
	query := s.client.UserGroup.Query()
	if status := strings.TrimSpace(req.Status); status != "" {
		query = query.Where(usergroup.StatusEQ(status))
	}
	if q := strings.TrimSpace(req.Query); q != "" {
		query = query.Where(usergroup.Or(usergroup.GroupCodeContainsFold(q), usergroup.GroupNameContainsFold(q)))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainadminuser.UserGroupListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(usergroup.FieldGroupCode)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainadminuser.UserGroupListPage{}, err
	}
	items := make([]domainadminuser.UserGroup, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapUserGroupEntity(entity))
	}
	return domainadminuser.UserGroupListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *AdminUserStore) GetUserGroup(ctx context.Context, groupCode string) (domainadminuser.UserGroup, error) {
	entity, err := s.client.UserGroup.Query().Where(usergroup.GroupCodeEQ(groupCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.UserGroup{}, errs.New(404, errs.CodeNotFound, "user group not found")
		}
		return domainadminuser.UserGroup{}, err
	}
	return mapUserGroupEntity(entity), nil
}

func (s *AdminUserStore) CreateUserGroup(ctx context.Context, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error) {
	create := s.client.UserGroup.Create().
		SetGroupCode(req.GroupCode).
		SetGroupName(req.GroupName).
		SetMultiplier(req.Multiplier).
		SetStatus(req.Status)
	if req.Description != nil {
		create.SetDescription(strings.TrimSpace(*req.Description))
	}
	entity, err := create.Save(ctx)
	if err != nil {
		if isConstraintError(err) {
			return domainadminuser.UserGroup{}, repoerr.ErrConflict
		}
		return domainadminuser.UserGroup{}, err
	}
	return mapUserGroupEntity(entity), nil
}

func (s *AdminUserStore) UpdateUserGroup(ctx context.Context, groupCode string, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error) {
	update := s.client.UserGroup.Update().Where(usergroup.GroupCodeEQ(groupCode)).
		SetGroupName(req.GroupName).
		SetMultiplier(req.Multiplier).
		SetStatus(req.Status)
	if req.Description != nil {
		update.SetDescription(strings.TrimSpace(*req.Description))
	} else {
		update.ClearDescription()
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return domainadminuser.UserGroup{}, err
	}
	if affected == 0 {
		return domainadminuser.UserGroup{}, errs.New(404, errs.CodeNotFound, "user group not found")
	}
	return s.GetUserGroup(ctx, groupCode)
}

func (s *AdminUserStore) DeleteUserGroup(ctx context.Context, groupCode string) error {
	group, err := s.client.UserGroup.Query().Where(usergroup.GroupCodeEQ(groupCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return errs.New(404, errs.CodeNotFound, "user group not found")
		}
		return err
	}
	count, err := s.client.User.Query().Where(user.UserGroupIDEQ(int64(group.ID))).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.New(409, errs.CodeConflict, "user group is still assigned to users")
	}
	affected, err := s.client.UserGroup.Delete().Where(usergroup.IDEQ(group.ID)).Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return errs.New(404, errs.CodeNotFound, "user group not found")
	}
	return nil
}

func (s *AdminUserStore) mapUser(ctx context.Context, entity *repoent.User) (domainadminuser.UserSummary, error) {
	groupCode := "basic"
	if entity.UserGroupID > 0 {
		groupEntity, err := s.client.UserGroup.Query().Where(usergroup.IDEQ(int(entity.UserGroupID))).Only(ctx)
		if err != nil && !repoent.IsNotFound(err) {
			return domainadminuser.UserSummary{}, err
		}
		if err == nil {
			groupCode = groupEntity.GroupCode
		}
	}
	return domainadminuser.UserSummary{
		ID:               int64(entity.ID),
		Email:            entity.Email,
		Nickname:         entity.Nickname,
		Status:           entity.Status,
		UserGroupCode:    groupCode,
		TokenVersion:     entity.TokenVersion,
		RPMLimit:         entity.RpmLimit,
		ConcurrencyLimit: entity.ConcurrencyLimit,
		DefaultLocale:    entity.DefaultLocale,
		Theme:            entity.Theme,
		ClosedAt:         entity.ClosedAt,
		CreatedAt:        entity.CreatedAt,
		UpdatedAt:        entity.UpdatedAt,
	}, nil
}

func normalizeAdminUserPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func mapUserGroupEntity(entity *repoent.UserGroup) domainadminuser.UserGroup {
	var description *string
	if entity.Description != nil {
		value := *entity.Description
		description = &value
	}
	return domainadminuser.UserGroup{
		GroupCode:   entity.GroupCode,
		GroupName:   entity.GroupName,
		Multiplier:  entity.Multiplier,
		Status:      entity.Status,
		Description: description,
		CreatedAt:   entity.CreatedAt,
		UpdatedAt:   entity.UpdatedAt,
	}
}

func isConstraintError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique constraint") || strings.Contains(text, "duplicate key")
}
