package entstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
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
	client    *repoent.Client
	batchSize int
}

const (
	defaultCallDistributionBatchSize       = 256
	maxCallDistributionTraceTransportBytes = 16 << 20
	defaultCallDistributionTraceBatchBytes = 16 << 20
	callDistributionTraceBytesAlias        = "provider_trace_bytes"
	providerTraceContextCheckInterval      = 4 << 10
)

var errInvalidCallDistributionTrace = errors.New("invalid call distribution trace")

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
	tx, err := s.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return domainadmincallrecord.Distribution{}, fmt.Errorf("begin call distribution snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := s.callDistributionInTx(ctx, tx.Client(), req)
	if err != nil {
		return domainadmincallrecord.Distribution{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainadmincallrecord.Distribution{}, fmt.Errorf("commit call distribution snapshot: %w", err)
	}
	return result, nil
}

type callDistributionRow struct {
	ID                  uuid.UUID  `json:"id"`
	Status              string     `json:"status"`
	RouteModelCode      string     `json:"route_model_code"`
	ProviderTraceBytes  int64      `json:"provider_trace_bytes"`
	UpstreamSucceededAt *time.Time `json:"upstream_succeeded_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type callDistributionTraceRow struct {
	ID            uuid.UUID `json:"id"`
	ProviderTrace []byte    `json:"provider_trace"`
}

type callDistributionCursor struct {
	updatedAt time.Time
	id        uuid.UUID
}

func (s *AdminCallRecordStore) callDistributionInTx(ctx context.Context, client *repoent.Client, req domainadmincallrecord.DistributionRequest) (domainadmincallrecord.Distribution, error) {
	batchSize := s.batchSize
	if batchSize <= 0 {
		batchSize = defaultCallDistributionBatchSize
	}
	accumulator := admincallrecordservice.NewDistributionAccumulator(req)
	var cursor *callDistributionCursor
	for {
		predicates := []predicate.ImageTask{
			imagetask.CreatedAtLT(req.To),
			imagetask.UpdatedAtGTE(req.From),
			callDistributionTraceLengthSelection(),
		}
		if cursor != nil {
			cursorValue := *cursor
			predicates = append(predicates, predicate.ImageTask(func(selector *entsql.Selector) {
				selector.Where(entsql.Or(
					entsql.GT(selector.C(imagetask.FieldUpdatedAt), cursorValue.updatedAt),
					entsql.And(
						entsql.EQ(selector.C(imagetask.FieldUpdatedAt), cursorValue.updatedAt),
						entsql.GT(selector.C(imagetask.FieldID), cursorValue.id),
					),
				))
			}))
		}

		var rows []callDistributionRow
		err := client.ImageTask.Query().
			Where(predicates...).
			Order(repoent.Asc(imagetask.FieldUpdatedAt), repoent.Asc(imagetask.FieldID)).
			Limit(batchSize).
			Select(
				imagetask.FieldID,
				imagetask.FieldStatus,
				imagetask.FieldRouteModelCode,
				imagetask.FieldUpstreamSucceededAt,
				imagetask.FieldCreatedAt,
				imagetask.FieldUpdatedAt,
			).
			Scan(ctx, &rows)
		if err != nil {
			return domainadmincallrecord.Distribution{}, fmt.Errorf("scan call distribution batch: %w", err)
		}
		traceBatches, err := callDistributionTraceBatches(rows, defaultCallDistributionTraceBatchBytes)
		if err != nil {
			return domainadmincallrecord.Distribution{}, err
		}
		for _, traceBatch := range traceBatches {
			if err := accumulateCallDistributionTraceBatch(ctx, client, accumulator, traceBatch, defaultCallDistributionTraceBatchBytes); err != nil {
				return domainadmincallrecord.Distribution{}, err
			}
		}
		if len(rows) < batchSize {
			return accumulator.Result(), nil
		}
		last := rows[len(rows)-1]
		cursor = &callDistributionCursor{updatedAt: last.UpdatedAt, id: last.ID}
	}
}

func callDistributionTraceLengthSelection() predicate.ImageTask {
	return func(selector *entsql.Selector) {
		column := selector.C(imagetask.FieldProviderTrace)
		expression := entsql.ExprFunc(func(builder *entsql.Builder) {
			builder.WriteString("COALESCE(")
			switch selector.Dialect() {
			case dialect.Postgres:
				builder.WriteString("octet_length(CAST(").Ident(column).WriteString(" AS text))")
			default:
				builder.WriteString("length(CAST(").Ident(column).WriteString(" AS BLOB))")
			}
			builder.WriteString(", 0)")
		})
		selector.AppendSelectExprAs(expression, callDistributionTraceBytesAlias)
	}
}

func callDistributionTraceBatches(rows []callDistributionRow, budget int) ([][]callDistributionRow, error) {
	batches := make([][]callDistributionRow, 0, 1)
	for start := 0; start < len(rows); {
		batchBytes := int64(0)
		end := start
		for end < len(rows) {
			traceBytes := rows[end].ProviderTraceBytes
			if traceBytes < 0 || traceBytes > maxCallDistributionTraceTransportBytes {
				return nil, errInvalidCallDistributionTrace
			}
			if end > start && batchBytes+traceBytes > int64(budget) {
				break
			}
			batchBytes += traceBytes
			end++
		}
		batches = append(batches, rows[start:end])
		start = end
	}
	return batches, nil
}

func accumulateCallDistributionTraceBatch(
	ctx context.Context,
	client *repoent.Client,
	accumulator *admincallrecordservice.DistributionAccumulator,
	metadata []callDistributionRow,
	budget int,
) error {
	ids := make([]uuid.UUID, len(metadata))
	for index := range metadata {
		ids[index] = metadata[index].ID
	}
	var traces []callDistributionTraceRow
	if err := client.ImageTask.Query().
		Where(imagetask.IDIn(ids...)).
		Select(imagetask.FieldID, imagetask.FieldProviderTrace).
		Scan(ctx, &traces); err != nil {
		return fmt.Errorf("scan call distribution traces: %w", err)
	}
	if len(traces) != len(metadata) {
		return errInvalidCallDistributionTrace
	}

	byID := make(map[uuid.UUID][]byte, len(traces))
	actualBytes := 0
	for _, trace := range traces {
		if _, exists := byID[trace.ID]; exists || len(trace.ProviderTrace) > maxCallDistributionTraceTransportBytes {
			return errInvalidCallDistributionTrace
		}
		actualBytes += len(trace.ProviderTrace)
		if actualBytes > budget {
			return errInvalidCallDistributionTrace
		}
		byID[trace.ID] = trace.ProviderTrace
	}
	for _, row := range metadata {
		if err := ctx.Err(); err != nil {
			return err
		}
		trace, exists := byID[row.ID]
		if !exists || int64(len(trace)) != row.ProviderTraceBytes {
			return errInvalidCallDistributionTrace
		}
		record, err := callDistributionRecord(ctx, row, trace)
		if err != nil {
			return err
		}
		accumulator.Add(record)
	}
	return nil
}

func callDistributionRecord(ctx context.Context, row callDistributionRow, trace []byte) (domainadmincallrecord.Record, error) {
	attempts, err := decodeCallDistributionAttempts(ctx, trace)
	if err != nil {
		return domainadmincallrecord.Record{}, err
	}
	return domainadmincallrecord.Record{
		TaskID:              row.ID.String(),
		Status:              row.Status,
		RouteModelCode:      row.RouteModelCode,
		UpstreamSucceededAt: row.UpstreamSucceededAt,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		Attempts:            attempts,
	}, nil
}

func decodeCallDistributionAttempts(ctx context.Context, trace []byte) ([]domainadmincallrecord.Attempt, error) {
	if len(trace) == 0 {
		return nil, nil
	}
	if len(trace) > maxCallDistributionTraceTransportBytes {
		return nil, errInvalidCallDistributionTrace
	}
	compactBytes, err := compactProviderTraceJSONSize(ctx, trace)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errInvalidCallDistributionTrace
	}
	if compactBytes > maxProviderTraceSemanticBytes {
		return nil, errInvalidCallDistributionTrace
	}

	decoder := json.NewDecoder(bytes.NewReader(trace))
	opening, err := decoder.Token()
	if err != nil {
		return nil, errInvalidCallDistributionTrace
	}
	if opening == nil {
		if err := requireCallDistributionTraceEOF(decoder); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return nil, errInvalidCallDistributionTrace
	}

	var attempts []domainadmincallrecord.Attempt
	attemptsSeen := false
	for decoder.More() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key, err := decoder.Token()
		if err != nil {
			return nil, errInvalidCallDistributionTrace
		}
		name, ok := key.(string)
		if !ok {
			return nil, errInvalidCallDistributionTrace
		}
		if name != "attempts" {
			var discarded json.RawMessage
			if err := decoder.Decode(&discarded); err != nil {
				return nil, errInvalidCallDistributionTrace
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			continue
		}
		if attemptsSeen {
			return nil, errInvalidCallDistributionTrace
		}
		attemptsSeen = true
		attempts, err = decodeCallDistributionAttemptArray(ctx, decoder)
		if err != nil {
			return nil, err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, errInvalidCallDistributionTrace
	}
	if err := requireCallDistributionTraceEOF(decoder); err != nil {
		return nil, err
	}
	return attempts, nil
}

func decodeCallDistributionAttemptArray(ctx context.Context, decoder *json.Decoder) ([]domainadmincallrecord.Attempt, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, errInvalidCallDistributionTrace
	}
	if token == nil {
		return nil, nil
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		return nil, errInvalidCallDistributionTrace
	}
	attempts := make([]domainadmincallrecord.Attempt, 0)
	for decoder.More() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(attempts) >= maxProviderTraceAttempts {
			return nil, errInvalidCallDistributionTrace
		}
		var attempt domainimagetask.Attempt
		if err := decoder.Decode(&attempt); err != nil {
			return nil, errInvalidCallDistributionTrace
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attempts = append(attempts, domainadmincallrecord.Attempt{StartedAt: attempt.StartedAt})
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim(']') {
		return nil, errInvalidCallDistributionTrace
	}
	return attempts, nil
}

func compactProviderTraceJSONSize(ctx context.Context, trace []byte) (int, error) {
	size := 0
	inString := false
	escaped := false
	for index, value := range trace {
		if index%providerTraceContextCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		if inString {
			size++
			switch {
			case escaped:
				escaped = false
			case value == '\\':
				escaped = true
			case value == '"':
				inString = false
			}
			continue
		}
		switch value {
		case ' ', '\t', '\n', '\r':
			continue
		case '"':
			inString = true
		}
		size++
	}
	if inString || escaped {
		return 0, errInvalidCallDistributionTrace
	}
	return size, nil
}

func requireCallDistributionTraceEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	}
	return errInvalidCallDistributionTrace
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
		predicates = append(predicates, predicate.ImageTask(func(s *entsql.Selector) {
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
