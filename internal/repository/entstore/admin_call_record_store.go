package entstore

import (
	"context"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/google/uuid"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	admincallrecordservice "github.com/fatballfish/pic-gallery/internal/service/admincallrecord"
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

func (s *AdminCallRecordStore) CallDistribution(ctx context.Context, req domainadmincallrecord.DistributionRequest) (domainadmincallrecord.Distribution, error) {
	entities, err := s.client.ImageTask.Query().Where(
		imagetask.DeletedAtIsNil(),
		imagetask.CreatedAtLT(req.To),
		imagetask.UpdatedAtGTE(req.From),
	).All(ctx)
	if err != nil {
		return domainadmincallrecord.Distribution{}, err
	}
	records := make([]domainadmincallrecord.Record, 0, len(entities))
	for _, entity := range entities {
		records = append(records, mapAdminCallRecord(entity))
	}
	return admincallrecordservice.AggregateCallDistribution(records, req), nil
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
	if req.PlatformLossOnly {
		predicates = append(predicates,
			imagetask.StatusEQ(domainimagetask.StatusFailed),
			imagetask.UpstreamSucceededAtNotNil(),
			imagetask.ArtifactRecoveryStatusEQ("failed"),
		)
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
		RouteModelCode:            entity.RouteModelCode,
		AccountModelID:            entity.AccountModelID,
		ModelAccountID:            entity.ModelAccountID,
		UpstreamModelCode:         entity.UpstreamModelCode,
		BaseResolution:            entity.BaseResolution,
		Quality:                   entity.Quality,
		RequestedOutputImageCount: entity.RequestedOutputImageCount,
		SuccessOutputImageCount:   entity.SuccessOutputImageCount,
		ReferenceImageCount:       entity.ReferenceImageCount,
		EstimatedPoints:           entity.EstimatedPoints,
		ActualPoints:              entity.ActualPoints,
		ProviderRequestID:         nullableString(entity.ProviderRequestID),
		ProviderCost:              entity.ProviderCost,
		GrossMargin:               entity.GrossMargin,
		PricingSnapshot:           cloneMapStringAny(entity.PricingSnapshot),
		UpstreamSucceededAt:       entity.UpstreamSucceededAt,
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
			record.Attempts = mapAdminCallRecordAttempts(attempts)
			record.ErrorDetail = lastAttemptErrorDetail(attempts)
		}
	}
	record.FailurePhase = adminCallRecordFailurePhase(record.Status, record.UpstreamSucceededAt, record.AttemptCount)
	record.PlatformLoss = record.FailurePhase == "artifact_persistence" && entity.ArtifactRecoveryStatus == "failed"
	if entity.ArtifactRecoveryStatus != "" || entity.ArtifactAttemptCount > 0 {
		lastDiagnostic, diagnostics := decodeArtifactDiagnostics(entity.ArtifactLastDiagnostic)
		record.ArtifactRecovery = &domainadmincallrecord.ArtifactRecoverySummary{
			Status: entity.ArtifactRecoveryStatus, AttemptCount: entity.ArtifactAttemptCount,
			LastDiagnostic: lastDiagnostic, Diagnostics: diagnostics,
		}
	}
	return record
}

func adminCallRecordFailurePhase(status string, upstreamSucceededAt *time.Time, attemptCount int) string {
	if status != domainimagetask.StatusFailed {
		return ""
	}
	if upstreamSucceededAt != nil {
		return "artifact_persistence"
	}
	if attemptCount > 0 {
		return "upstream"
	}
	return "preflight"
}

func mapAdminCallRecordAttempts(attempts []domainimagetask.Attempt) []domainadmincallrecord.Attempt {
	if len(attempts) == 0 {
		return nil
	}
	items := make([]domainadmincallrecord.Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		items = append(items, domainadmincallrecord.Attempt{
			Provider:       attempt.Provider,
			AdapterType:    attempt.AdapterType,
			AccountModelID: int64PtrIfPositive(attempt.AccountModelID),
			ModelAccountID: int64PtrIfPositive(attempt.ModelAccountID),
			ModelCode:      attempt.ModelCode,
			Status:         attempt.Status,
			Error:          attempt.Error,
			ErrorCode:      attempt.ErrorCode,
			ErrorMessage:   attempt.ErrorMessage,
			ErrorDetail:    cloneMapStringAny(attempt.ErrorDetail),
			StartedAt:      attempt.StartedAt,
			FinishedAt:     attempt.FinishedAt,
		})
	}
	return items
}

func lastAttemptErrorDetail(attempts []domainimagetask.Attempt) map[string]any {
	for i := len(attempts) - 1; i >= 0; i-- {
		if len(attempts[i].ErrorDetail) > 0 {
			return cloneMapStringAny(attempts[i].ErrorDetail)
		}
	}
	return nil
}

func int64PtrIfPositive(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func cloneMapStringAny(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
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
