package entstore

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/user"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/usergroup"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/usergroupmember"
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

	items, err := s.mapUsers(ctx, entities)
	if err != nil {
		return domainadminuser.ListPage{}, err
	}
	if req.GroupCode == "" && req.SortBy == "created_at" && req.SortDir == "desc" {
		return domainadminuser.ListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
	}

	// Fall back to in-memory filtering/sorting only when the requested view needs joined data.
	allEntities, err := query.All(ctx)
	if err != nil {
		return domainadminuser.ListPage{}, err
	}
	allItems, err := s.mapUsers(ctx, allEntities)
	if err != nil {
		return domainadminuser.ListPage{}, err
	}
	filtered := make([]domainadminuser.UserSummary, 0, len(allItems))
	for _, item := range allItems {
		if req.GroupCode != "" && !userSummaryHasGroup(item, req.GroupCode) {
			continue
		}
		filtered = append(filtered, item)
	}
	sortAdminUsers(filtered, req.SortBy, req.SortDir)
	total = len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return domainadminuser.ListPage{Items: filtered[start:end], Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *AdminUserStore) CreateUser(ctx context.Context, req domainadminuser.CreateUserRequest) (domainadminuser.UserSummary, error) {
	group, err := s.ensureUserGroup(ctx, req.UserGroupCode)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	defer func() { _ = tx.Rollback() }()
	create := tx.User.Create().
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
	if _, err := createDefaultProjectInTx(ctx, tx, int64(entity.ID)); err != nil {
		return domainadminuser.UserSummary{}, err
	}
	if err := tx.Commit(); err != nil {
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
			TrialPoints:         balance.TrialPoints,
			SubscriptionPoints:  balance.SubscriptionPoints,
			GiftPoints:          balance.GiftPoints,
			RechargePoints:      balance.RechargePoints,
			Buckets:             balance.Buckets,
			UserGroupMultiplier: "1.00000",
			CNYPerPoint:         "0.00000",
			ActiveSubscription:  balance.ActiveSubscription,
			NextExpiringGrant:   balance.NextExpiringGrant,
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
		SetStatus("enabled").
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

func (s *AdminUserStore) AssignUserGroups(ctx context.Context, req domainadminuser.MultiGroupAssignmentRequest) (domainadminuser.UserSummary, error) {
	if _, err := s.client.User.Query().Where(user.IDEQ(int(req.UserID)), user.DeletedAtIsNil()).Only(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
		}
		return domainadminuser.UserSummary{}, err
	}
	if err := s.ensureUserGroupIDs(ctx, req.GroupIDs); err != nil {
		return domainadminuser.UserSummary{}, err
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	if _, err := tx.UserGroupMember.Delete().Where(usergroupmember.UserIDEQ(req.UserID)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return domainadminuser.UserSummary{}, err
	}
	for _, groupID := range req.GroupIDs {
		if groupID <= 0 {
			continue
		}
		if _, err := tx.UserGroupMember.Create().SetUserID(req.UserID).SetGroupID(groupID).Save(ctx); err != nil {
			_ = tx.Rollback()
			return domainadminuser.UserSummary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domainadminuser.UserSummary{}, err
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
	entities, err := query.Order(repoent.Asc(usergroup.FieldSortOrder), repoent.Asc(usergroup.FieldGroupCode)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
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
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainadminuser.UserGroup, error) {
		if req.IsDefault {
			if err := tx.UserGroup.Update().Where(usergroup.IsDefaultEQ(true)).SetIsDefault(false).Exec(ctx); err != nil {
				return domainadminuser.UserGroup{}, err
			}
		}
		create := tx.UserGroup.Create().
			SetGroupCode(req.GroupCode).
			SetGroupName(req.GroupName).
			SetMultiplier(req.Multiplier).
			SetStatus(req.Status).
			SetSortOrder(req.SortOrder).
			SetIsDefault(req.IsDefault)
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
	})
}

func (s *AdminUserStore) UpdateUserGroup(ctx context.Context, groupCode string, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainadminuser.UserGroup, error) {
		if req.IsDefault {
			if err := tx.UserGroup.Update().Where(usergroup.GroupCodeNEQ(groupCode), usergroup.IsDefaultEQ(true)).SetIsDefault(false).Exec(ctx); err != nil {
				return domainadminuser.UserGroup{}, err
			}
		}
		update := tx.UserGroup.Update().Where(usergroup.GroupCodeEQ(groupCode)).
			SetGroupName(req.GroupName).
			SetMultiplier(req.Multiplier).
			SetStatus(req.Status).
			SetSortOrder(req.SortOrder).
			SetIsDefault(req.IsDefault)
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
		entity, err := tx.UserGroup.Query().Where(usergroup.GroupCodeEQ(groupCode)).Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainadminuser.UserGroup{}, errs.New(404, errs.CodeNotFound, "user group not found")
			}
			return domainadminuser.UserGroup{}, err
		}
		return mapUserGroupEntity(entity), nil
	})
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
	memberCount, err := s.client.UserGroupMember.Query().Where(usergroupmember.GroupIDEQ(int64(group.ID))).Count(ctx)
	if err != nil {
		return err
	}
	if memberCount > 0 {
		return errs.New(409, errs.CodeConflict, "user group is still referenced by user memberships")
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
	items, err := s.mapUsers(ctx, []*repoent.User{entity})
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	if len(items) == 0 {
		return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	return items[0], nil
}

func (s *AdminUserStore) mapUsers(ctx context.Context, entities []*repoent.User) ([]domainadminuser.UserSummary, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	userIDs := make([]int64, 0, len(entities))
	groupIDs := make([]int64, 0, len(entities))
	seenGroupIDs := make(map[int64]struct{}, len(entities))
	for _, entity := range entities {
		userIDs = append(userIDs, int64(entity.ID))
		if entity.UserGroupID > 0 {
			if _, ok := seenGroupIDs[entity.UserGroupID]; !ok {
				seenGroupIDs[entity.UserGroupID] = struct{}{}
				groupIDs = append(groupIDs, entity.UserGroupID)
			}
		}
	}
	memberMap, memberGroupIDs, err := s.userMemberGroupsByUser(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, groupID := range memberGroupIDs {
		if _, ok := seenGroupIDs[groupID]; ok {
			continue
		}
		seenGroupIDs[groupID] = struct{}{}
		groupIDs = append(groupIDs, groupID)
	}
	groupsByID, err := s.userGroupsByID(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	balancesByUser, err := s.userBalancesByUser(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	items := make([]domainadminuser.UserSummary, 0, len(entities))
	for _, entity := range entities {
		groupCode := "basic"
		groups := make([]domainadminuser.UserGroup, 0, 2)
		if group, ok := groupsByID[entity.UserGroupID]; ok {
			groupCode = group.GroupCode
			groups = append(groups, group)
		}
		for _, memberGroupID := range memberMap[int64(entity.ID)] {
			group, ok := groupsByID[memberGroupID]
			if !ok || strings.EqualFold(group.GroupCode, groupCode) {
				continue
			}
			groups = append(groups, group)
		}
		items = append(items, domainadminuser.UserSummary{
			ID:               int64(entity.ID),
			Email:            entity.Email,
			Nickname:         entity.Nickname,
			Status:           entity.Status,
			UserGroupCode:    groupCode,
			UserGroups:       groups,
			Balance:          balancesByUser[int64(entity.ID)],
			TokenVersion:     entity.TokenVersion,
			RPMLimit:         entity.RpmLimit,
			ConcurrencyLimit: entity.ConcurrencyLimit,
			DefaultLocale:    entity.DefaultLocale,
			Theme:            entity.Theme,
			LastSeenAt:       entity.UpdatedAt,
			ClosedAt:         entity.ClosedAt,
			CreatedAt:        entity.CreatedAt,
			UpdatedAt:        entity.UpdatedAt,
		})
	}
	return items, nil
}

func sortAdminUsers(items []domainadminuser.UserSummary, sortBy, sortDir string) {
	desc := sortDir != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		compare := 0
		switch sortBy {
		case "points":
			left, _ := strconv.ParseFloat(items[i].Balance, 64)
			right, _ := strconv.ParseFloat(items[j].Balance, 64)
			if left < right {
				compare = -1
			} else if left > right {
				compare = 1
			}
		case "last_seen_at":
			compare = compareAdminUserTime(items[i].LastSeenAt, items[j].LastSeenAt)
		default:
			compare = compareAdminUserTime(items[i].CreatedAt, items[j].CreatedAt)
		}
		if compare == 0 {
			if items[i].ID < items[j].ID {
				compare = -1
			} else if items[i].ID > items[j].ID {
				compare = 1
			}
		}
		if desc {
			return compare > 0
		}
		return compare < 0
	})
}

func userSummaryHasGroup(item domainadminuser.UserSummary, groupCode string) bool {
	if strings.EqualFold(item.UserGroupCode, groupCode) {
		return true
	}
	for _, group := range item.UserGroups {
		if strings.EqualFold(group.GroupCode, groupCode) {
			return true
		}
	}
	return false
}

func compareAdminUserTime(left, right time.Time) int {
	if left.Before(right) {
		return -1
	}
	if left.After(right) {
		return 1
	}
	return 0
}

func (s *AdminUserStore) ensureUserGroupIDs(ctx context.Context, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	intIDs := make([]int, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		intIDs = append(intIDs, int(groupID))
	}
	count, err := s.client.UserGroup.Query().Where(usergroup.IDIn(intIDs...)).Count(ctx)
	if err != nil {
		return err
	}
	if count != len(groupIDs) {
		return errs.New(404, errs.CodeNotFound, "user group not found")
	}
	return nil
}

func (s *AdminUserStore) userMemberGroupsByUser(ctx context.Context, userIDs []int64) (map[int64][]int64, []int64, error) {
	result := make(map[int64][]int64, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil, nil
	}
	members, err := s.client.UserGroupMember.Query().Where(usergroupmember.UserIDIn(userIDs...)).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	groupIDs := make([]int64, 0, len(members))
	seenGroupIDs := make(map[int64]struct{}, len(members))
	for _, member := range members {
		result[member.UserID] = append(result[member.UserID], member.GroupID)
		if _, ok := seenGroupIDs[member.GroupID]; ok {
			continue
		}
		seenGroupIDs[member.GroupID] = struct{}{}
		groupIDs = append(groupIDs, member.GroupID)
	}
	return result, groupIDs, nil
}

func (s *AdminUserStore) userGroupsByID(ctx context.Context, groupIDs []int64) (map[int64]domainadminuser.UserGroup, error) {
	result := make(map[int64]domainadminuser.UserGroup, len(groupIDs))
	if len(groupIDs) == 0 {
		return result, nil
	}
	intIDs := make([]int, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		intIDs = append(intIDs, int(groupID))
	}
	entities, err := s.client.UserGroup.Query().Where(usergroup.IDIn(intIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, entity := range entities {
		result[int64(entity.ID)] = mapUserGroupEntity(entity)
	}
	return result, nil
}

func (s *AdminUserStore) userBalancesByUser(ctx context.Context, userIDs []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = "0.00000"
	}
	if len(userIDs) == 0 {
		return result, nil
	}
	entries, err := s.client.PointLedger.Query().
		Where(pointledger.UserIDIn(userIDs...)).
		Order(repoent.Desc(pointledger.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{}, len(userIDs))
	for _, entry := range entries {
		if _, ok := result[entry.UserID]; !ok {
			continue
		}
		if _, ok := seen[entry.UserID]; ok {
			continue
		}
		seen[entry.UserID] = struct{}{}
		result[entry.UserID] = entry.BalanceAfter
	}
	return result, nil
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
		ID:          int64(entity.ID),
		GroupCode:   entity.GroupCode,
		GroupName:   entity.GroupName,
		Multiplier:  entity.Multiplier,
		Status:      entity.Status,
		Description: description,
		SortOrder:   entity.SortOrder,
		IsDefault:   entity.IsDefault,
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
