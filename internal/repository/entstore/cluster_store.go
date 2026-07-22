package entstore

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/clusternode"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/clustertoken"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/installation"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
)

type ClusterStore struct {
	client *repoent.Client
}

func NewClusterStore(client *repoent.Client) *ClusterStore {
	return &ClusterStore{client: client}
}

func (s *ClusterStore) GetInstallation(ctx context.Context, installationID string) (domaincluster.Installation, error) {
	if s == nil || s.client == nil {
		return domaincluster.Installation{}, fmt.Errorf("cluster store is not configured")
	}
	entity, err := s.client.Installation.Query().Where(installation.InstallationIDEQ(installationID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return domaincluster.Installation{}, clusterservice.ErrInstallationNotFound
	}
	if err != nil {
		return domaincluster.Installation{}, err
	}
	initialized := entity.SetupOperationID != nil && entity.SetupAdminID != nil && entity.SetupConfigRevision != nil && entity.SetupRequestDigest != nil
	configRevision := int64(0)
	if entity.SetupConfigRevision != nil {
		configRevision = int64(*entity.SetupConfigRevision)
	}
	return domaincluster.Installation{
		InstallationID: entity.InstallationID, Initialized: initialized,
		ApplicationVersion: entity.AppVersion, RuntimeSchemaVersion: entity.ConfigSchemaVersion,
		ConfigRevision: configRevision,
	}, nil
}

func (s *ClusterStore) CreateToken(ctx context.Context, record domaincluster.TokenRecord) (domaincluster.TokenRecord, error) {
	if s == nil || s.client == nil {
		return domaincluster.TokenRecord{}, fmt.Errorf("cluster store is not configured")
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domaincluster.TokenRecord{}, fmt.Errorf("begin cluster token transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txStore := NewClusterStore(tx.Client())
	created, err := txStore.createToken(ctx, record)
	if err != nil {
		return domaincluster.TokenRecord{}, err
	}
	if err := txStore.createAuditRecords(ctx, []domainaudit.RecordRequest{clusterservice.TokenCreateAuditRecord(created.Token)}, record.CreatedAt); err != nil {
		return domaincluster.TokenRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return domaincluster.TokenRecord{}, fmt.Errorf("commit cluster token transaction: %w", err)
	}
	committed = true
	return created, nil
}

func (s *ClusterStore) createToken(ctx context.Context, record domaincluster.TokenRecord) (domaincluster.TokenRecord, error) {
	entity, err := s.client.ClusterToken.Create().
		SetTokenID(record.TokenID).
		SetTokenHash(record.TokenHash).
		SetInstallationID(record.InstallationID).
		SetRole(clustertoken.Role(record.Role)).
		SetExpiresAt(record.ExpiresAt).
		SetAuditActor(record.CreatedBy).
		SetCreatedAt(record.CreatedAt).
		SetUpdatedAt(record.UpdatedAt).
		Save(ctx)
	if repoent.IsConstraintError(err) {
		return domaincluster.TokenRecord{}, clusterservice.ErrTokenUnavailable
	}
	if err != nil {
		return domaincluster.TokenRecord{}, err
	}
	return mapClusterToken(entity), nil
}

func (s *ClusterStore) ListTokens(ctx context.Context, installationID string, request domaincluster.ListTokensRequest) (domaincluster.TokenPage, error) {
	query := s.client.ClusterToken.Query().Where(clustertoken.InstallationIDEQ(installationID))
	if request.Role != "" {
		query = query.Where(clustertoken.RoleEQ(clustertoken.Role(request.Role)))
	}
	at := request.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	switch strings.TrimSpace(request.Status) {
	case "active":
		query = query.Where(clustertoken.ConsumedAtIsNil(), clustertoken.RevokedAtIsNil(), clustertoken.ExpiresAtGT(at))
	case "expired":
		query = query.Where(clustertoken.ConsumedAtIsNil(), clustertoken.RevokedAtIsNil(), clustertoken.ExpiresAtLTE(at))
	case "consumed":
		query = query.Where(clustertoken.ConsumedAtNotNil())
	case "revoked":
		query = query.Where(clustertoken.RevokedAtNotNil())
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domaincluster.TokenPage{}, err
	}
	entities, err := query.Order(repoent.Desc(clustertoken.FieldCreatedAt), repoent.Desc(clustertoken.FieldID)).
		Offset((request.Page - 1) * request.PageSize).
		Limit(request.PageSize).
		All(ctx)
	if err != nil {
		return domaincluster.TokenPage{}, err
	}
	items := make([]domaincluster.Token, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapClusterToken(entity).Token)
	}
	return domaincluster.TokenPage{Items: items, Page: request.Page, PageSize: request.PageSize, Total: total}, nil
}

func (s *ClusterStore) RevokeToken(ctx context.Context, installationID, tokenID, actorID string, revokedAt time.Time) (domaincluster.TokenRecord, error) {
	if s == nil || s.client == nil {
		return domaincluster.TokenRecord{}, fmt.Errorf("cluster store is not configured")
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domaincluster.TokenRecord{}, fmt.Errorf("begin cluster token transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txStore := NewClusterStore(tx.Client())
	revoked, err := txStore.revokeToken(ctx, installationID, tokenID, revokedAt)
	if err != nil {
		return domaincluster.TokenRecord{}, err
	}
	if err := txStore.createAuditRecords(ctx, []domainaudit.RecordRequest{clusterservice.TokenRevokeAuditRecord(revoked.Token, actorID)}, revokedAt); err != nil {
		return domaincluster.TokenRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return domaincluster.TokenRecord{}, fmt.Errorf("commit cluster token transaction: %w", err)
	}
	committed = true
	return revoked, nil
}

func (s *ClusterStore) revokeToken(ctx context.Context, installationID, tokenID string, revokedAt time.Time) (domaincluster.TokenRecord, error) {
	updated, err := s.client.ClusterToken.Update().
		Where(
			clustertoken.InstallationIDEQ(installationID), clustertoken.TokenIDEQ(tokenID),
			clustertoken.ExpiresAtGT(revokedAt), clustertoken.ConsumedAtIsNil(), clustertoken.RevokedAtIsNil(),
		).
		SetRevokedAt(revokedAt).
		SetUpdatedAt(revokedAt).
		Save(ctx)
	if err != nil {
		return domaincluster.TokenRecord{}, err
	}
	if updated != 1 {
		return domaincluster.TokenRecord{}, s.classifyTokenMutation(ctx, installationID, tokenID, "", "", revokedAt)
	}
	return s.getToken(ctx, installationID, tokenID)
}

func (s *ClusterStore) ConsumeToken(ctx context.Context, installationID, tokenID, tokenHash string, role domaincluster.JoinRole, nodeID string, consumedAt time.Time) (domaincluster.TokenRecord, error) {
	updated, err := s.client.ClusterToken.Update().
		Where(
			clustertoken.InstallationIDEQ(installationID), clustertoken.TokenIDEQ(tokenID),
			clustertoken.TokenHashEQ(tokenHash), clustertoken.RoleEQ(clustertoken.Role(role)),
			clustertoken.ExpiresAtGT(consumedAt), clustertoken.ConsumedAtIsNil(), clustertoken.RevokedAtIsNil(),
		).
		SetConsumedAt(consumedAt).
		SetConsumedByNodeID(nodeID).
		SetUpdatedAt(consumedAt).
		Save(ctx)
	if err != nil {
		return domaincluster.TokenRecord{}, err
	}
	if updated != 1 {
		return domaincluster.TokenRecord{}, s.classifyTokenMutation(ctx, installationID, tokenID, tokenHash, role, consumedAt)
	}
	return s.getToken(ctx, installationID, tokenID)
}

func (s *ClusterStore) AcceptEnrollment(ctx context.Context, installationID, tokenID, tokenHash string, node domaincluster.Node, acceptedAt time.Time) (domaincluster.TokenRecord, domaincluster.Node, error) {
	if s == nil || s.client == nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, fmt.Errorf("cluster store is not configured")
	}
	if node.InstallationID != installationID {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, clusterservice.ErrNodeConflict
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, fmt.Errorf("begin cluster enrollment transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txStore := NewClusterStore(tx.Client())
	record, err := txStore.ConsumeToken(ctx, installationID, tokenID, tokenHash, domaincluster.JoinRole(node.Role), node.NodeID, acceptedAt)
	if err != nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, err
	}
	registered, err := txStore.createNodeStrict(ctx, node)
	if err != nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, err
	}
	if err := txStore.createAuditRecords(ctx, clusterservice.EnrollmentAuditRecords(record.Token, registered), acceptedAt); err != nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, err
	}
	if err := tx.Commit(); err != nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, fmt.Errorf("commit cluster enrollment transaction: %w", err)
	}
	committed = true
	return record, registered, nil
}

func (s *ClusterStore) createAuditRecords(ctx context.Context, records []domainaudit.RecordRequest, at time.Time) error {
	for _, record := range records {
		_, err := s.client.AuditLog.Create().
			SetActorType(record.ActorType).
			SetActorID(record.ActorID).
			SetAction(record.Action).
			SetTargetType(record.TargetType).
			SetTargetID(record.TargetID).
			SetResult(record.Result).
			SetMetadata(record.Metadata).
			SetCreatedAt(at).
			SetUpdatedAt(at).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("record cluster audit: %w", err)
		}
	}
	return nil
}

func (s *ClusterStore) classifyTokenMutation(ctx context.Context, installationID, tokenID, tokenHash string, role domaincluster.JoinRole, at time.Time) error {
	record, err := s.getToken(ctx, installationID, tokenID)
	if err != nil {
		return err
	}
	if tokenHash != "" && (subtle.ConstantTimeCompare([]byte(record.TokenHash), []byte(tokenHash)) != 1 || record.Role != role) {
		return clusterservice.ErrTokenNotFound
	}
	if record.ConsumedAt != nil || record.RevokedAt != nil {
		return clusterservice.ErrTokenUnavailable
	}
	if !record.ExpiresAt.After(at) {
		if tokenHash == "" {
			return clusterservice.ErrTokenUnavailable
		}
		return clusterservice.ErrTokenNotFound
	}
	return clusterservice.ErrTokenUnavailable
}

func (s *ClusterStore) getToken(ctx context.Context, installationID, tokenID string) (domaincluster.TokenRecord, error) {
	entity, err := s.client.ClusterToken.Query().Where(clustertoken.InstallationIDEQ(installationID), clustertoken.TokenIDEQ(tokenID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return domaincluster.TokenRecord{}, clusterservice.ErrTokenNotFound
	}
	if err != nil {
		return domaincluster.TokenRecord{}, err
	}
	return mapClusterToken(entity), nil
}

func (s *ClusterStore) CreateNode(ctx context.Context, node domaincluster.Node) (domaincluster.Node, error) {
	entity, err := s.client.ClusterNode.Create().
		SetNodeID(node.NodeID).
		SetInstallationID(node.InstallationID).
		SetRole(clusternode.Role(node.Role)).
		SetAppVersion(node.ApplicationVersion).
		SetRuntimeSchemaVersion(node.RuntimeSchemaVersion).
		SetConfigRevision(node.ConfigRevision).
		SetHealth(clusternode.Health(node.Health)).
		SetLastError(node.LastError).
		SetCreatedAt(node.CreatedAt).
		SetUpdatedAt(node.UpdatedAt).
		Save(ctx)
	if err == nil {
		return mapClusterNode(entity), nil
	}
	if !repoent.IsConstraintError(err) {
		return domaincluster.Node{}, err
	}
	existing, lookupErr := s.client.ClusterNode.Query().Where(clusternode.NodeIDEQ(node.NodeID)).Only(ctx)
	if lookupErr != nil {
		return domaincluster.Node{}, errors.Join(clusterservice.ErrNodeConflict, lookupErr)
	}
	mapped := mapClusterNode(existing)
	if mapped.InstallationID == node.InstallationID && mapped.Role == node.Role && mapped.ApplicationVersion == node.ApplicationVersion && mapped.RuntimeSchemaVersion == node.RuntimeSchemaVersion && mapped.ConfigRevision == node.ConfigRevision {
		return mapped, nil
	}
	return domaincluster.Node{}, clusterservice.ErrNodeConflict
}

func (s *ClusterStore) createNodeStrict(ctx context.Context, node domaincluster.Node) (domaincluster.Node, error) {
	entity, err := s.client.ClusterNode.Create().
		SetNodeID(node.NodeID).
		SetInstallationID(node.InstallationID).
		SetRole(clusternode.Role(node.Role)).
		SetAppVersion(node.ApplicationVersion).
		SetRuntimeSchemaVersion(node.RuntimeSchemaVersion).
		SetConfigRevision(node.ConfigRevision).
		SetHealth(clusternode.Health(node.Health)).
		SetLastError(node.LastError).
		SetCreatedAt(node.CreatedAt).
		SetUpdatedAt(node.UpdatedAt).
		Save(ctx)
	if repoent.IsConstraintError(err) {
		return domaincluster.Node{}, clusterservice.ErrNodeConflict
	}
	if err != nil {
		return domaincluster.Node{}, err
	}
	return mapClusterNode(entity), nil
}

func (s *ClusterStore) HeartbeatNode(ctx context.Context, installationID string, request domaincluster.HeartbeatRequest, heartbeatAt time.Time) (domaincluster.Node, error) {
	updated, err := s.client.ClusterNode.Update().
		Where(clusternode.InstallationIDEQ(installationID), clusternode.NodeIDEQ(request.NodeID)).
		SetHealth(clusternode.Health(request.Health)).
		SetLastError(request.LastError).
		SetAppVersion(request.ApplicationVersion).
		SetRuntimeSchemaVersion(request.RuntimeSchemaVersion).
		SetConfigRevision(request.ConfigRevision).
		SetLastHeartbeatAt(heartbeatAt).
		SetUpdatedAt(heartbeatAt).
		Save(ctx)
	if err != nil {
		return domaincluster.Node{}, err
	}
	if updated != 1 {
		return domaincluster.Node{}, clusterservice.ErrNodeNotFound
	}
	entity, err := s.client.ClusterNode.Query().Where(clusternode.InstallationIDEQ(installationID), clusternode.NodeIDEQ(request.NodeID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return domaincluster.Node{}, clusterservice.ErrNodeNotFound
	}
	if err != nil {
		return domaincluster.Node{}, err
	}
	return mapClusterNode(entity), nil
}

func mapClusterToken(entity *repoent.ClusterToken) domaincluster.TokenRecord {
	return domaincluster.TokenRecord{
		Token: domaincluster.Token{
			TokenID: entity.TokenID, InstallationID: entity.InstallationID, Role: domaincluster.JoinRole(entity.Role),
			ExpiresAt: entity.ExpiresAt, ConsumedAt: entity.ConsumedAt, RevokedAt: entity.RevokedAt,
			CreatedBy: entity.AuditActor, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
			ConsumedByNodeID: clusterStringValue(entity.ConsumedByNodeID),
		},
		TokenHash: entity.TokenHash,
	}
}

func mapClusterNode(entity *repoent.ClusterNode) domaincluster.Node {
	return domaincluster.Node{
		NodeID: entity.NodeID, InstallationID: entity.InstallationID, Role: domaincluster.NodeRole(entity.Role),
		ApplicationVersion: entity.AppVersion, RuntimeSchemaVersion: entity.RuntimeSchemaVersion,
		ConfigRevision: entity.ConfigRevision, Health: domaincluster.NodeHealth(entity.Health), LastError: entity.LastError,
		LastHeartbeatAt: entity.LastHeartbeatAt, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

func clusterStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
