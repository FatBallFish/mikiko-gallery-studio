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

type StorageConfigStore struct {
	client *repoent.Client
}

func NewStorageConfigStore(client *repoent.Client) *StorageConfigStore {
	return &StorageConfigStore{client: client}
}

func (s *StorageConfigStore) List(ctx context.Context) ([]domainstorageconfig.ConfigRecord, error) {
	rows, err := s.client.ObjectStorageConfig.Query().
		Where(objectstorageconfig.StatusNEQ(domainstorageconfig.StatusDeleted)).
		Order(repoent.Desc(objectstorageconfig.FieldIsDefault), repoent.Asc(objectstorageconfig.FieldCode)).
		All(ctx)
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
		return domainstorageconfig.ConfigRecord{}, false, repoerr.ErrNotFound
	}
	row, err := s.client.ObjectStorageConfig.Query().
		Where(objectstorageconfig.IDEQ(parsed), objectstorageconfig.StatusNEQ(domainstorageconfig.StatusDeleted)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) GetByCode(ctx context.Context, code string) (domainstorageconfig.ConfigRecord, bool, error) {
	row, err := s.client.ObjectStorageConfig.Query().
		Where(objectstorageconfig.CodeEQ(strings.TrimSpace(code)), objectstorageconfig.StatusNEQ(domainstorageconfig.StatusDeleted)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) GetDefaultWritable(ctx context.Context) (domainstorageconfig.ConfigRecord, bool, error) {
	row, err := s.client.ObjectStorageConfig.Query().
		Where(
			objectstorageconfig.StatusEQ(domainstorageconfig.StatusEnabled),
			objectstorageconfig.ReadEnabledEQ(true),
			objectstorageconfig.WriteEnabledEQ(true),
			objectstorageconfig.IsDefaultEQ(true),
		).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) GetLegacyByDriver(ctx context.Context, driver string) (domainstorageconfig.ConfigRecord, bool, error) {
	code := "bootstrap-" + strings.ToLower(strings.TrimSpace(driver))
	row, err := s.client.ObjectStorageConfig.Query().
		Where(
			objectstorageconfig.CodeEQ(code),
			objectstorageconfig.StatusEQ(domainstorageconfig.StatusEnabled),
			objectstorageconfig.ReadEnabledEQ(true),
		).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainstorageconfig.ConfigRecord{}, false, nil
		}
		return domainstorageconfig.ConfigRecord{}, false, err
	}
	return mapObjectStorageConfig(row), true, nil
}

func (s *StorageConfigStore) Save(ctx context.Context, record domainstorageconfig.ConfigRecord) (domainstorageconfig.ConfigRecord, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(record.ID) == "" {
		create := s.client.ObjectStorageConfig.Create().
			SetCode(record.Code).
			SetName(record.Name).
			SetDriver(record.Driver).
			SetProvider(record.Provider).
			SetStatus(record.Status).
			SetReadEnabled(record.ReadEnabled).
			SetWriteEnabled(record.WriteEnabled).
			SetIsDefault(record.IsDefault).
			SetPrefix(record.Prefix).
			SetForcePathStyle(record.ForcePathStyle).
			SetPublicValue(cloneConfigValue(record.PublicValue)).
			SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
			SetSecretFingerprint(record.SecretFingerprint).
			SetSecretFields(append([]string{}, record.SecretFields...)).
			SetLastProbeStatus(record.LastProbeStatus).
			SetLastProbeMessage(record.LastProbeMessage).
			SetVersion(defaultInt64(record.Version, 1)).
			SetUpdatedBy(record.UpdatedBy)
		setObjectStorageConfigOptionalFields(create, record)
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
		SetName(record.Name).
		SetDriver(record.Driver).
		SetProvider(record.Provider).
		SetStatus(record.Status).
		SetReadEnabled(record.ReadEnabled).
		SetWriteEnabled(record.WriteEnabled).
		SetIsDefault(record.IsDefault).
		SetPrefix(record.Prefix).
		SetForcePathStyle(record.ForcePathStyle).
		SetPublicValue(cloneConfigValue(record.PublicValue)).
		SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
		SetSecretFingerprint(record.SecretFingerprint).
		SetSecretFields(append([]string{}, record.SecretFields...)).
		SetLastProbeStatus(record.LastProbeStatus).
		SetLastProbeMessage(record.LastProbeMessage).
		SetVersion(defaultInt64(record.Version, 1)).
		SetUpdatedBy(record.UpdatedBy).
		SetUpdatedAt(now)
	setObjectStorageConfigUpdateOptionalFields(update, record)
	if record.LastProbeAt != nil {
		update.SetLastProbeAt(*record.LastProbeAt)
	} else {
		update.ClearLastProbeAt()
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	return mapObjectStorageConfig(updated), nil
}

func (s *StorageConfigStore) ClearDefault(ctx context.Context) error {
	_, err := s.client.ObjectStorageConfig.Update().
		Where(objectstorageconfig.IsDefaultEQ(true)).
		SetIsDefault(false).
		Save(ctx)
	return err
}

func mapObjectStorageConfig(row *repoent.ObjectStorageConfig) domainstorageconfig.ConfigRecord {
	if row == nil {
		return domainstorageconfig.ConfigRecord{}
	}
	return domainstorageconfig.ConfigRecord{
		ID:                row.ID.String(),
		Code:              row.Code,
		Name:              row.Name,
		Driver:            row.Driver,
		Provider:          row.Provider,
		Status:            row.Status,
		ReadEnabled:       row.ReadEnabled,
		WriteEnabled:      row.WriteEnabled,
		IsDefault:         row.IsDefault,
		Endpoint:          storageStringValue(row.Endpoint),
		Region:            storageStringValue(row.Region),
		Bucket:            storageStringValue(row.Bucket),
		Prefix:            row.Prefix,
		ForcePathStyle:    row.ForcePathStyle,
		PublicBaseURL:     storageStringValue(row.PublicBaseURL),
		LocalRoot:         storageStringValue(row.LocalRoot),
		PublicValue:       cloneConfigValue(row.PublicValue),
		SecretEncrypted:   cloneConfigValue(row.SecretEncrypted),
		SecretFingerprint: row.SecretFingerprint,
		SecretFields:      append([]string{}, row.SecretFields...),
		LastProbeStatus:   row.LastProbeStatus,
		LastProbeMessage:  row.LastProbeMessage,
		LastProbeAt:       row.LastProbeAt,
		Version:           row.Version,
		UpdatedBy:         row.UpdatedBy,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func setObjectStorageConfigOptionalFields(create *repoent.ObjectStorageConfigCreate, record domainstorageconfig.ConfigRecord) {
	if strings.TrimSpace(record.Endpoint) != "" {
		create.SetEndpoint(record.Endpoint)
	}
	if strings.TrimSpace(record.Region) != "" {
		create.SetRegion(record.Region)
	}
	if strings.TrimSpace(record.Bucket) != "" {
		create.SetBucket(record.Bucket)
	}
	if strings.TrimSpace(record.PublicBaseURL) != "" {
		create.SetPublicBaseURL(record.PublicBaseURL)
	}
	if strings.TrimSpace(record.LocalRoot) != "" {
		create.SetLocalRoot(record.LocalRoot)
	}
}

func setObjectStorageConfigUpdateOptionalFields(update *repoent.ObjectStorageConfigUpdateOne, record domainstorageconfig.ConfigRecord) {
	if strings.TrimSpace(record.Endpoint) != "" {
		update.SetEndpoint(record.Endpoint)
	} else {
		update.ClearEndpoint()
	}
	if strings.TrimSpace(record.Region) != "" {
		update.SetRegion(record.Region)
	} else {
		update.ClearRegion()
	}
	if strings.TrimSpace(record.Bucket) != "" {
		update.SetBucket(record.Bucket)
	} else {
		update.ClearBucket()
	}
	if strings.TrimSpace(record.PublicBaseURL) != "" {
		update.SetPublicBaseURL(record.PublicBaseURL)
	} else {
		update.ClearPublicBaseURL()
	}
	if strings.TrimSpace(record.LocalRoot) != "" {
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
