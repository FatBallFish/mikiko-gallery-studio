package entstore

import (
	"context"
	"strings"

	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/user"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/usergroup"
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

func (s *AdminUserStore) GetUserDetail(ctx context.Context, userID int64, recentLedgerLimit int) (domainadminuser.Detail, error) {
	entity, err := s.client.User.Get(ctx, int(userID))
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

func (s *AdminUserStore) UpdateUserStatus(ctx context.Context, userID int64, status string) (domainadminuser.UserSummary, error) {
	entity, err := s.client.User.Get(ctx, int(userID))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminuser.UserSummary{}, errs.New(404, errs.CodeNotFound, "user not found")
		}
		return domainadminuser.UserSummary{}, err
	}
	update := s.client.User.UpdateOneID(int(userID)).SetStatus(status)
	if entity.Status != status {
		update.AddTokenVersion(1)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return domainadminuser.UserSummary{}, err
	}
	return s.mapUser(ctx, updated)
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
		ID:            int64(entity.ID),
		Email:         entity.Email,
		Nickname:      entity.Nickname,
		Status:        entity.Status,
		UserGroupCode: groupCode,
		TokenVersion:  entity.TokenVersion,
		DefaultLocale: entity.DefaultLocale,
		Theme:         entity.Theme,
		CreatedAt:     entity.CreatedAt,
		UpdatedAt:     entity.UpdatedAt,
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
