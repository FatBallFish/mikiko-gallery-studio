package entstore

import (
	"context"
	"net/http"
	"strings"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainredeem "github.com/fatballfish/pic-gallery/internal/domain/redeem"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/redeemcode"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type RedeemAdminStore struct {
	client *repoent.Client
}

func NewRedeemAdminStore(client *repoent.Client) *RedeemAdminStore {
	return &RedeemAdminStore{client: client}
}

func (s *RedeemAdminStore) ListCodes(ctx context.Context, req domainredeem.ListRequest) (domainredeem.ListPage, error) {
	page, pageSize := normalizeRedeemPage(req.Page, req.PageSize)
	query := s.client.RedeemCode.Query()
	if status := strings.TrimSpace(req.Status); status != "" {
		query = query.Where(redeemcode.StatusEQ(status))
	}
	if code := strings.ToUpper(strings.TrimSpace(req.Code)); code != "" {
		query = query.Where(redeemcode.CodeContains(code))
	}
	if req.BatchID > 0 {
		query = query.Where(redeemcode.BatchIDEQ(req.BatchID))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainredeem.ListPage{}, err
	}
	entities, err := query.Order(repoent.Desc(redeemcode.FieldCreatedAt), repoent.Desc(redeemcode.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return domainredeem.ListPage{}, err
	}
	items := make([]domainredeem.Code, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapRedeemCode(entity))
	}
	return domainredeem.ListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *RedeemAdminStore) CreateCode(ctx context.Context, req domainredeem.CreateRequest) (domainredeem.Code, error) {
	entity, err := s.client.RedeemCode.Create().
		SetBatchID(req.BatchID).
		SetCode(strings.ToUpper(strings.TrimSpace(req.Code))).
		SetStatus(req.Status).
		SetRewardType(req.RewardType).
		SetRewardValue(req.RewardValue).
		SetValidFrom(req.ValidFrom).
		SetValidUntil(req.ValidUntil).
		SetMaxRedemptions(req.MaxRedemptions).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainredeem.Code{}, errs.New(http.StatusConflict, errs.CodeConflict, "redeem code already exists")
		}
		return domainredeem.Code{}, err
	}
	return mapRedeemCode(entity), nil
}

func (s *RedeemAdminStore) CodeExists(ctx context.Context, code string) (bool, error) {
	return s.client.RedeemCode.Query().Where(redeemcode.CodeEQ(strings.ToUpper(strings.TrimSpace(code)))).Exist(ctx)
}

func (s *RedeemAdminStore) UpdateStatus(ctx context.Context, id int64, status string) (domainredeem.Code, error) {
	entity, err := s.client.RedeemCode.UpdateOneID(int(id)).SetStatus(status).Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainredeem.Code{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "redeem code not found")
		}
		return domainredeem.Code{}, err
	}
	return mapRedeemCode(entity), nil
}

func (s *RedeemAdminStore) ListRedemptions(ctx context.Context, codeID int64, page, pageSize int) (domainredeem.RedemptionsPage, error) {
	if exists, err := s.client.RedeemCode.Query().Where(redeemcode.IDEQ(int(codeID))).Exist(ctx); err != nil {
		return domainredeem.RedemptionsPage{}, err
	} else if !exists {
		return domainredeem.RedemptionsPage{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "redeem code not found")
	}
	page, pageSize = normalizeRedeemPage(page, pageSize)
	query := s.client.PointLedger.Query().Where(pointledger.RedeemCodeIDEQ(codeID))
	total, err := query.Count(ctx)
	if err != nil {
		return domainredeem.RedemptionsPage{}, err
	}
	entries, err := query.Order(repoent.Desc(pointledger.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return domainredeem.RedemptionsPage{}, err
	}
	items := make([]domainbilling.LedgerEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, mapLedgerEntry(entry))
	}
	return domainredeem.RedemptionsPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func mapRedeemCode(entity *repoent.RedeemCode) domainredeem.Code {
	return domainredeem.Code{
		ID:             int64(entity.ID),
		BatchID:        entity.BatchID,
		Code:           entity.Code,
		Status:         entity.Status,
		RewardType:     entity.RewardType,
		RewardValue:    entity.RewardValue,
		ValidFrom:      entity.ValidFrom,
		ValidUntil:     entity.ValidUntil,
		MaxRedemptions: entity.MaxRedemptions,
		RedeemedCount:  entity.RedeemedCount,
		LastRedeemedBy: entity.LastRedeemedBy,
		CreatedAt:      entity.CreatedAt,
		UpdatedAt:      entity.UpdatedAt,
	}
}

func normalizeRedeemPage(page, pageSize int) (int, int) {
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
