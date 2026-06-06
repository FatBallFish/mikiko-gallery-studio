package entstore

import (
	"context"
	"strings"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/google/uuid"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
)

type AdminCallRecordStore struct {
	client *repoent.Client
}

func NewAdminCallRecordStore(client *repoent.Client) *AdminCallRecordStore {
	return &AdminCallRecordStore{client: client}
}

func (s *AdminCallRecordStore) ListCallRecords(ctx context.Context, req domainadmincallrecord.ListRequest) (domainadmincallrecord.ListPage, error) {
	page, pageSize := normalizeAdminCallRecordPage(req.Page, req.PageSize)
	query := s.client.ImageTask.Query().Where(adminCallRecordPredicates(req)...)
	total, err := query.Count(ctx)
	if err != nil {
		return domainadmincallrecord.ListPage{}, err
	}
	entities, err := query.Order(repoent.Desc(imagetask.FieldCreatedAt), repoent.Desc(imagetask.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return domainadmincallrecord.ListPage{}, err
	}
	items := make([]domainadmincallrecord.Record, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapAdminCallRecord(entity))
	}
	return domainadmincallrecord.ListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func adminCallRecordPredicates(req domainadmincallrecord.ListRequest) []predicate.ImageTask {
	predicates := []predicate.ImageTask{imagetask.DeletedAtIsNil()}
	if status := strings.TrimSpace(req.Status); status != "" {
		predicates = append(predicates, imagetask.StatusEQ(status))
	}
	if errorCode := strings.TrimSpace(req.ErrorCode); errorCode != "" {
		predicates = append(predicates, imagetask.ErrorCodeEQ(errorCode))
	}
	if provider := strings.TrimSpace(req.Provider); provider != "" {
		predicates = append(predicates, predicate.ImageTask(func(s *sql.Selector) {
			s.Where(sqljson.ValueEQ(s.C(imagetask.FieldProviderTrace), provider, sqljson.Path("provider")))
		}))
	}
	if sourceChannel := strings.TrimSpace(req.SourceChannel); sourceChannel != "" {
		predicates = append(predicates, imagetask.SourceChannelEQ(sourceChannel))
	}
	if req.UserID > 0 {
		predicates = append(predicates, imagetask.UserIDEQ(req.UserID))
	}
	if req.TaskID != "" {
		if taskUUID, err := uuid.Parse(req.TaskID); err == nil {
			predicates = append(predicates, imagetask.IDEQ(taskUUID))
		}
	}
	if !req.CreatedFrom.IsZero() {
		predicates = append(predicates, imagetask.CreatedAtGTE(req.CreatedFrom))
	}
	if !req.CreatedTo.IsZero() {
		predicates = append(predicates, imagetask.CreatedAtLTE(req.CreatedTo))
	}
	return predicates
}

func mapAdminCallRecord(entity *repoent.ImageTask) domainadmincallrecord.Record {
	record := domainadmincallrecord.Record{
		TaskID:                    entity.ID.String(),
		UserID:                    entity.UserID,
		APIKeyID:                  entity.APIKeyID,
		SourceChannel:             entity.SourceChannel,
		TaskType:                  entity.TaskType,
		Status:                    entity.Status,
		AbstractModel:             entity.AbstractModel,
		Quality:                   entity.ResolvedQualityBucket,
		RequestedOutputImageCount: entity.RequestedOutputImageCount,
		SuccessOutputImageCount:   entity.SuccessOutputImageCount,
		ReferenceImageCount:       entity.ReferenceImageCount,
		EstimatedPoints:           entity.EstimatedPoints,
		ActualPoints:              entity.ActualPoints,
		ErrorCode:                 entity.ErrorCode,
		ErrorMessage:              entity.ErrorMessage,
		CreatedAt:                 entity.CreatedAt,
		UpdatedAt:                 entity.UpdatedAt,
		StartedAt:                 entity.StartedAt,
		FinishedAt:                entity.FinishedAt,
	}
	if entity.ProviderTrace != nil {
		if providerName, ok := entity.ProviderTrace["provider"].(string); ok {
			record.Provider = providerName
		}
		if attempts, err := decodeAttempts(entity.ProviderTrace["attempts"]); err == nil {
			record.AttemptCount = len(attempts)
		}
	}
	return record
}

func normalizeAdminCallRecordPage(page, pageSize int) (int, int) {
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
