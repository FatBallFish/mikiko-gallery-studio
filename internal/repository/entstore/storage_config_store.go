package entstore

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectstorageconfig"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type StorageConfigStore struct{ client *repoent.Client }

func NewStorageConfigStore(client *repoent.Client) *StorageConfigStore {
	return &StorageConfigStore{client: client}
}

func (s *StorageConfigStore) List(ctx context.Context) ([]domainstorageconfig.ConfigRecord, error) {
	rows, err := s.client.ObjectStorageConfig.Query().
		Where(objectstorageconfig.StatusNEQ(domainstorageconfig.StatusDeleted)).
		Order(repoent.Desc(objectstorageconfig.FieldIsDefault), repoent.Asc(objectstorageconfig.FieldCode)).All(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]domainstorageconfig.ConfigRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, mapObjectStorageConfig(row))
	}
	return records, nil
}

func (s *StorageConfigStore) GetByID(ctx context.Context, id string) (domainstorageconfig.ConfigRecord, bool, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, false, nil
	}
	row, err := s.client.ObjectStorageConfig.Query().Where(objectstorageconfig.IDEQ(parsed), objectstorageconfig.StatusNEQ(domainstorageconfig.StatusDeleted)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) GetByCode(ctx context.Context, code string) (domainstorageconfig.ConfigRecord, bool, error) {
	row, err := s.client.ObjectStorageConfig.Query().Where(objectstorageconfig.CodeEQ(strings.TrimSpace(code)), objectstorageconfig.StatusNEQ(domainstorageconfig.StatusDeleted)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) GetDefaultWritable(ctx context.Context) (domainstorageconfig.ConfigRecord, bool, error) {
	row, err := s.client.ObjectStorageConfig.Query().Where(
		objectstorageconfig.StatusEQ(domainstorageconfig.StatusEnabled),
		objectstorageconfig.ReadEnabledEQ(true), objectstorageconfig.WriteEnabledEQ(true), objectstorageconfig.IsDefaultEQ(true),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) GetLegacyByDriver(ctx context.Context, driver string) (domainstorageconfig.ConfigRecord, bool, error) {
	row, err := s.client.ObjectStorageConfig.Query().Where(
		objectstorageconfig.CodeEQ("bootstrap-"+strings.ToLower(strings.TrimSpace(driver))),
		objectstorageconfig.StatusEQ(domainstorageconfig.StatusEnabled), objectstorageconfig.ReadEnabledEQ(true),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) Save(ctx context.Context, record domainstorageconfig.ConfigRecord) (domainstorageconfig.ConfigRecord, error) {
	if strings.TrimSpace(record.ID) == "" {
		create := s.client.ObjectStorageConfig.Create().SetCode(record.Code).SetName(record.Name).SetDriver(record.Driver).
			SetProvider(record.Provider).SetStatus(record.Status).SetReadEnabled(record.ReadEnabled).SetWriteEnabled(record.WriteEnabled).
			SetIsDefault(record.IsDefault).SetPrefix(record.Prefix).SetForcePathStyle(record.ForcePathStyle).
			SetPublicValue(cloneConfigValue(record.PublicValue)).SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
			SetSecretFingerprint(record.SecretFingerprint).SetSecretFields(append([]string{}, record.SecretFields...)).
			SetLastProbeStatus(record.LastProbeStatus).SetLastProbeMessage(record.LastProbeMessage).
			SetVersion(defaultInt64(record.Version, 1)).SetUpdatedBy(record.UpdatedBy)
		setObjectStorageConfigCreateOptionalFields(create, record)
		if record.LastProbeAt != nil {
			create.SetLastProbeAt(*record.LastProbeAt)
		}
		created, err := create.Save(ctx)
		if err != nil {
			return domainstorageconfig.ConfigRecord{}, err
		}
		return mapObjectStorageConfig(created), nil
	}
	id, err := uuid.Parse(strings.TrimSpace(record.ID))
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	update := s.client.ObjectStorageConfig.UpdateOneID(id).
		Where(objectstorageconfig.VersionEQ(record.Version-1), objectstorageconfig.IsDefaultEQ(record.IsDefault)).
		SetName(record.Name).SetDriver(record.Driver).
		SetProvider(record.Provider).SetStatus(record.Status).SetReadEnabled(record.ReadEnabled).SetWriteEnabled(record.WriteEnabled).
		SetIsDefault(record.IsDefault).SetPrefix(record.Prefix).SetForcePathStyle(record.ForcePathStyle).
		SetPublicValue(cloneConfigValue(record.PublicValue)).SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
		SetSecretFingerprint(record.SecretFingerprint).SetSecretFields(append([]string{}, record.SecretFields...)).
		SetLastProbeStatus(record.LastProbeStatus).SetLastProbeMessage(record.LastProbeMessage).
		SetVersion(defaultInt64(record.Version, 1)).SetUpdatedBy(record.UpdatedBy).SetUpdatedAt(time.Now().UTC())
	setObjectStorageConfigUpdateOptionalFields(update, record)
	if record.LastProbeAt != nil {
		update.SetLastProbeAt(*record.LastProbeAt)
	} else {
		update.ClearLastProbeAt()
	}
	updated, err := update.Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, repoerr.ErrConflict
		}
		return domainstorageconfig.ConfigRecord{}, err
	}
	return mapObjectStorageConfig(updated), nil
}

func (s *StorageConfigStore) SetDefault(ctx context.Context, id string, expectedVersion, updatedBy int64) (domainstorageconfig.ConfigRecord, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, repoerr.ErrNotFound
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	defer tx.Rollback()
	current, err := tx.ObjectStorageConfig.Query().Where(objectstorageconfig.IDEQ(parsed), objectstorageconfig.VersionEQ(expectedVersion), objectstorageconfig.StatusNEQ(domainstorageconfig.StatusDeleted)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, repoerr.ErrConflict
		}
		return domainstorageconfig.ConfigRecord{}, err
	}
	if _, err := tx.ObjectStorageConfig.Update().Where(objectstorageconfig.IsDefaultEQ(true), objectstorageconfig.IDNEQ(parsed)).SetIsDefault(false).Save(ctx); err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	updated, err := tx.ObjectStorageConfig.UpdateOneID(parsed).
		SetIsDefault(true).
		SetVersion(current.Version + 1).
		SetUpdatedBy(updatedBy).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	return mapObjectStorageConfig(updated), nil
}

func mapObjectStorageConfig(row *repoent.ObjectStorageConfig) domainstorageconfig.ConfigRecord {
	if row == nil {
		return domainstorageconfig.ConfigRecord{}
	}
	return domainstorageconfig.ConfigRecord{
		ID: row.ID.String(), Code: row.Code, Name: row.Name, Driver: row.Driver, Provider: row.Provider, Status: row.Status,
		ReadEnabled: row.ReadEnabled, WriteEnabled: row.WriteEnabled, IsDefault: row.IsDefault,
		Endpoint: storageStringValue(row.Endpoint), Region: storageStringValue(row.Region), Bucket: storageStringValue(row.Bucket),
		Prefix: row.Prefix, ForcePathStyle: row.ForcePathStyle, PublicBaseURL: storageStringValue(row.PublicBaseURL), LocalRoot: storageStringValue(row.LocalRoot),
		PublicValue: cloneConfigValue(row.PublicValue), SecretEncrypted: cloneConfigValue(row.SecretEncrypted), SecretFingerprint: row.SecretFingerprint,
		SecretFields: append([]string{}, row.SecretFields...), LastProbeStatus: row.LastProbeStatus, LastProbeMessage: row.LastProbeMessage,
		LastProbeAt: row.LastProbeAt, Version: row.Version, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func setObjectStorageConfigCreateOptionalFields(create *repoent.ObjectStorageConfigCreate, record domainstorageconfig.ConfigRecord) {
	if record.Endpoint != "" {
		create.SetEndpoint(record.Endpoint)
	}
	if record.Region != "" {
		create.SetRegion(record.Region)
	}
	if record.Bucket != "" {
		create.SetBucket(record.Bucket)
	}
	if record.PublicBaseURL != "" {
		create.SetPublicBaseURL(record.PublicBaseURL)
	}
	if record.LocalRoot != "" {
		create.SetLocalRoot(record.LocalRoot)
	}
}

func setObjectStorageConfigUpdateOptionalFields(update *repoent.ObjectStorageConfigUpdateOne, record domainstorageconfig.ConfigRecord) {
	if record.Endpoint != "" {
		update.SetEndpoint(record.Endpoint)
	} else {
		update.ClearEndpoint()
	}
	if record.Region != "" {
		update.SetRegion(record.Region)
	} else {
		update.ClearRegion()
	}
	if record.Bucket != "" {
		update.SetBucket(record.Bucket)
	} else {
		update.ClearBucket()
	}
	if record.PublicBaseURL != "" {
		update.SetPublicBaseURL(record.PublicBaseURL)
	} else {
		update.ClearPublicBaseURL()
	}
	if record.LocalRoot != "" {
		update.SetLocalRoot(record.LocalRoot)
	} else {
		update.ClearLocalRoot()
	}
}

func storageStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
