package entstore

import (
	"context"
	"fmt"
	"time"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
)

const (
	aliasRolloutCategory = "runtime_rollout"
	aliasRolloutKey      = "no_copy_reference_aliases"
)

type AliasRolloutStore struct {
	client *repoent.Client
}

func NewAliasRolloutStore(client *repoent.Client) *AliasRolloutStore {
	return &AliasRolloutStore{client: client}
}

func (s *AliasRolloutStore) AliasCreationEnabled(ctx context.Context) (bool, error) {
	status, err := s.GetAliasCreationRollout(ctx)
	return status.Enabled, err
}

func (s *AliasRolloutStore) GetAliasCreationRollout(ctx context.Context) (domainassets.AliasCreationRollout, error) {
	row, err := s.client.ConfigItem.Query().Where(
		configitem.ConfigCategoryEQ(aliasRolloutCategory),
		configitem.ConfigKeyEQ(aliasRolloutKey),
		configitem.ScopeEQ("global"),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return domainassets.AliasCreationRollout{}, nil
	}
	if err != nil {
		return domainassets.AliasCreationRollout{}, err
	}
	enabled, _ := row.ConfigValue["enabled"].(bool)
	return domainassets.AliasCreationRollout{
		Enabled: enabled, Version: row.Version, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (s *AliasRolloutStore) UpdateAliasCreationRollout(ctx context.Context, req domainassets.UpdateAliasCreationRolloutRequest) (domainassets.AliasCreationRollout, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainassets.AliasCreationRollout{}, fmt.Errorf("begin alias rollout transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txStore := NewAliasRolloutStore(tx.Client())
	before, err := txStore.GetAliasCreationRollout(ctx)
	if err != nil {
		return domainassets.AliasCreationRollout{}, fmt.Errorf("read alias rollout before update: %w", err)
	}
	if before.Version != req.ExpectedVersion {
		return domainassets.AliasCreationRollout{}, domainassets.ErrAliasRolloutChanged
	}
	after, err := txStore.updateAliasCreationRollout(ctx, req)
	if err != nil {
		return domainassets.AliasCreationRollout{}, err
	}
	if err := txStore.createAliasRolloutAudit(ctx, req, before, after); err != nil {
		return domainassets.AliasCreationRollout{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainassets.AliasCreationRollout{}, fmt.Errorf("commit alias rollout transaction: %w", err)
	}
	committed = true
	return after, nil
}

func (s *AliasRolloutStore) updateAliasCreationRollout(ctx context.Context, req domainassets.UpdateAliasCreationRolloutRequest) (domainassets.AliasCreationRollout, error) {
	now := time.Now().UTC()
	if req.ExpectedVersion == 0 {
		_, err := s.client.ConfigItem.Create().
			SetConfigCategory(aliasRolloutCategory).
			SetConfigKey(aliasRolloutKey).
			SetScope("global").
			SetConfigValue(map[string]any{"enabled": req.Enabled}).
			SetVersion(1).
			SetUpdatedBy(req.UpdatedBy).
			SetUpdatedAt(now).
			Save(ctx)
		if repoent.IsConstraintError(err) {
			return domainassets.AliasCreationRollout{}, domainassets.ErrAliasRolloutChanged
		}
		if err != nil {
			return domainassets.AliasCreationRollout{}, err
		}
		return domainassets.AliasCreationRollout{Enabled: req.Enabled, Version: 1, UpdatedBy: req.UpdatedBy, UpdatedAt: now}, nil
	}
	updated, err := s.client.ConfigItem.Update().Where(
		configitem.ConfigCategoryEQ(aliasRolloutCategory),
		configitem.ConfigKeyEQ(aliasRolloutKey),
		configitem.ScopeEQ("global"),
		configitem.VersionEQ(req.ExpectedVersion),
	).
		SetConfigValue(map[string]any{"enabled": req.Enabled}).
		SetVersion(req.ExpectedVersion + 1).
		SetUpdatedBy(req.UpdatedBy).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return domainassets.AliasCreationRollout{}, err
	}
	if updated != 1 {
		return domainassets.AliasCreationRollout{}, domainassets.ErrAliasRolloutChanged
	}
	status := domainassets.AliasCreationRollout{
		Enabled: req.Enabled, Version: req.ExpectedVersion + 1, UpdatedBy: req.UpdatedBy, UpdatedAt: now,
	}
	return status, nil
}

func (s *AliasRolloutStore) createAliasRolloutAudit(ctx context.Context, req domainassets.UpdateAliasCreationRolloutRequest, before, after domainassets.AliasCreationRollout) error {
	action := "disable"
	if after.Enabled {
		action = "enable"
	}
	metadata := map[string]any{
		"enabled":                     after.Enabled,
		"version":                     after.Version,
		"expected_version":            req.ExpectedVersion,
		"all_api_nodes_cleanup_aware": req.AllAPINodesCleanupAware,
		"request_id":                  req.RequestID,
		"before":                      aliasRolloutAuditState(before),
		"after":                       aliasRolloutAuditState(after),
	}
	_, err := s.client.AuditLog.Create().
		SetActorType(req.ActorType).
		SetActorID(req.ActorID).
		SetAction("runtime_rollout.alias_creation." + action).
		SetTargetType("runtime_rollout").
		SetTargetID(aliasRolloutKey).
		SetResult("success").
		SetMetadata(metadata).
		SetIPAddr(req.IPAddr).
		SetUserAgent(req.UserAgent).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("record alias rollout audit: %w", err)
	}
	return nil
}

func aliasRolloutAuditState(status domainassets.AliasCreationRollout) map[string]any {
	state := map[string]any{
		"enabled":    status.Enabled,
		"version":    status.Version,
		"updated_by": status.UpdatedBy,
	}
	if !status.UpdatedAt.IsZero() {
		state["updated_at"] = status.UpdatedAt
	}
	return state
}
