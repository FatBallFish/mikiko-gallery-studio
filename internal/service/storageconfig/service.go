package storageconfig

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/service/secretcodec"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	secretAccessKeyID     = "access_key_id"
	secretSecretAccessKey = "secret_access_key"
)

type Service struct {
	store       Store
	codec       *secretcodec.Codec
	bootstrap   config.StorageConfig
	environment string
}

func NewService(store Store, encryptionKey string, bootstrap config.StorageConfig, environment string) *Service {
	return &Service{store: store, codec: secretcodec.New(encryptionKey), bootstrap: bootstrap, environment: strings.TrimSpace(environment)}
}

func (s *Service) Bootstrap(ctx context.Context, updatedBy int64) error {
	if s == nil || s.store == nil {
		return errs.Internal("storage config store is not available")
	}
	existing, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	record, err := s.bootstrapRecord(updatedBy)
	if err != nil {
		return err
	}
	_, err = s.store.Save(ctx, record)
	return err
}

func (s *Service) List(ctx context.Context) ([]domainstorageconfig.ConfigView, error) {
	records, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]domainstorageconfig.ConfigView, 0, len(records))
	for _, record := range records {
		views = append(views, viewFromRecord(record))
	}
	return views, nil
}

func (s *Service) Get(ctx context.Context, id string) (domainstorageconfig.ConfigView, error) {
	record, ok, err := s.store.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	if !ok {
		return domainstorageconfig.ConfigView{}, errs.New(404, errs.CodeNotFound, "storage config not found")
	}
	return viewFromRecord(record), nil
}

func (s *Service) Create(ctx context.Context, req domainstorageconfig.WriteRequest) (domainstorageconfig.ConfigView, error) {
	record, err := s.recordForWrite(ctx, domainstorageconfig.ConfigRecord{}, false, req)
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	saved, err := s.store.Save(ctx, record)
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	return viewFromRecord(saved), nil
}

func (s *Service) Update(ctx context.Context, req domainstorageconfig.WriteRequest) (domainstorageconfig.ConfigView, error) {
	current, ok, err := s.store.GetByID(ctx, strings.TrimSpace(req.ID))
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	if !ok {
		return domainstorageconfig.ConfigView{}, errs.New(404, errs.CodeNotFound, "storage config not found")
	}
	if req.Version > 0 && req.Version != current.Version {
		return domainstorageconfig.ConfigView{}, errs.New(409, errs.CodeConflict, "storage config version conflict")
	}
	record, err := s.recordForWrite(ctx, current, true, req)
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	saved, err := s.store.Save(ctx, record)
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	return viewFromRecord(saved), nil
}

func (s *Service) SetStatus(ctx context.Context, req domainstorageconfig.StatusRequest) (domainstorageconfig.ConfigView, error) {
	current, ok, err := s.store.GetByID(ctx, strings.TrimSpace(req.ID))
	if err != nil || !ok {
		if err != nil {
			return domainstorageconfig.ConfigView{}, err
		}
		return domainstorageconfig.ConfigView{}, errs.New(404, errs.CodeNotFound, "storage config not found")
	}
	if req.Version > 0 && req.Version != current.Version {
		return domainstorageconfig.ConfigView{}, errs.New(409, errs.CodeConflict, "storage config version conflict")
	}
	current.Status = normalizeStatus(req.Status)
	current.ReadEnabled, current.WriteEnabled = req.ReadEnabled, req.WriteEnabled
	current.UpdatedBy, current.Version = req.UpdatedBy, current.Version+1
	if current.IsDefault && (current.Status != domainstorageconfig.StatusEnabled || !current.ReadEnabled || !current.WriteEnabled) {
		return domainstorageconfig.ConfigView{}, errs.BadRequest("default storage config must remain enabled for read and write")
	}
	saved, err := s.store.Save(ctx, current)
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	return viewFromRecord(saved), nil
}

func (s *Service) SetDefault(ctx context.Context, req domainstorageconfig.SetDefaultRequest) (domainstorageconfig.ConfigView, error) {
	current, ok, err := s.store.GetByID(ctx, strings.TrimSpace(req.ID))
	if err != nil || !ok {
		if err != nil {
			return domainstorageconfig.ConfigView{}, err
		}
		return domainstorageconfig.ConfigView{}, errs.New(404, errs.CodeNotFound, "storage config not found")
	}
	if req.Version > 0 && req.Version != current.Version {
		return domainstorageconfig.ConfigView{}, errs.New(409, errs.CodeConflict, "storage config version conflict")
	}
	if current.Status != domainstorageconfig.StatusEnabled || !current.ReadEnabled || !current.WriteEnabled {
		return domainstorageconfig.ConfigView{}, errs.BadRequest("storage config must be enabled for read and write before becoming default")
	}
	if current.LastProbeStatus != domainstorageconfig.ProbeStatusSuccess {
		return domainstorageconfig.ConfigView{}, errs.BadRequest("storage config must pass probe before becoming default")
	}
	if err := s.store.ClearDefault(ctx); err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	current.IsDefault, current.UpdatedBy, current.Version = true, req.UpdatedBy, current.Version+1
	saved, err := s.store.Save(ctx, current)
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	return viewFromRecord(saved), nil
}

func (s *Service) ResolveDefaultWritable(ctx context.Context) (domainstorageconfig.ResolvedConfig, error) {
	record, ok, err := s.store.GetDefaultWritable(ctx)
	if err != nil {
		return domainstorageconfig.ResolvedConfig{}, err
	}
	if !ok {
		return domainstorageconfig.ResolvedConfig{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "default storage config is unavailable")
	}
	return s.resolveRecord(record)
}

func (s *Service) ResolveByID(ctx context.Context, id string) (domainstorageconfig.ResolvedConfig, error) {
	record, ok, err := s.store.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domainstorageconfig.ResolvedConfig{}, err
	}
	if !ok || record.Status == domainstorageconfig.StatusDeleted || !record.ReadEnabled {
		return domainstorageconfig.ResolvedConfig{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	return s.resolveRecord(record)
}

func (s *Service) ResolveLegacyByDriver(ctx context.Context, driver string) (domainstorageconfig.ResolvedConfig, error) {
	record, ok, err := s.store.GetLegacyByDriver(ctx, normalizeDriver(driver))
	if err != nil {
		return domainstorageconfig.ResolvedConfig{}, err
	}
	if !ok {
		return domainstorageconfig.ResolvedConfig{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "legacy storage config is unavailable")
	}
	return s.resolveRecord(record)
}

func (s *Service) UpdateProbe(ctx context.Context, id string, result domainstorageconfig.ProbeResult, updatedBy int64) (domainstorageconfig.ConfigView, error) {
	current, ok, err := s.store.GetByID(ctx, strings.TrimSpace(id))
	if err != nil || !ok {
		if err != nil {
			return domainstorageconfig.ConfigView{}, err
		}
		return domainstorageconfig.ConfigView{}, errs.New(404, errs.CodeNotFound, "storage config not found")
	}
	current.LastProbeStatus = normalizeProbeStatus(result.Status)
	current.LastProbeMessage = truncate(strings.TrimSpace(result.Message), 512)
	checkedAt := result.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	current.LastProbeAt, current.UpdatedBy, current.Version = &checkedAt, updatedBy, current.Version+1
	saved, err := s.store.Save(ctx, current)
	if err != nil {
		return domainstorageconfig.ConfigView{}, err
	}
	view := viewFromRecord(saved)
	view.LastProbe.LatencyMS = result.LatencyMS
	return view, nil
}

func (s *Service) ResolveForProbe(ctx context.Context, id string) (domainstorageconfig.ResolvedConfig, error) {
	record, ok, err := s.store.GetByID(ctx, strings.TrimSpace(id))
	if err != nil || !ok {
		if err != nil {
			return domainstorageconfig.ResolvedConfig{}, err
		}
		return domainstorageconfig.ResolvedConfig{}, errs.New(404, errs.CodeNotFound, "storage config not found")
	}
	return s.resolveRecord(record)
}

func (s *Service) resolveRecord(record domainstorageconfig.ConfigRecord) (domainstorageconfig.ResolvedConfig, error) {
	secrets, err := s.codec.DecryptJSON(record.SecretEncrypted)
	if err != nil {
		return domainstorageconfig.ResolvedConfig{}, err
	}
	return domainstorageconfig.ResolvedConfig{ConfigRecord: record, Secrets: secrets}, nil
}

func (s *Service) recordForWrite(ctx context.Context, current domainstorageconfig.ConfigRecord, updating bool, req domainstorageconfig.WriteRequest) (domainstorageconfig.ConfigRecord, error) {
	now := time.Now().UTC()
	record := current
	if !updating {
		record.Code, record.CreatedAt, record.Version, record.LastProbeStatus = strings.TrimSpace(req.Code), now, 1, domainstorageconfig.ProbeStatusNever
		if record.Code == "" {
			return domainstorageconfig.ConfigRecord{}, errs.BadRequest("storage config code is required")
		}
		if existing, ok, err := s.store.GetByCode(ctx, record.Code); err != nil {
			return domainstorageconfig.ConfigRecord{}, err
		} else if ok && existing.ID != current.ID {
			return domainstorageconfig.ConfigRecord{}, errs.New(409, errs.CodeConflict, "storage config code already exists")
		}
	} else {
		record.Version++
	}
	record.Name, record.Driver = strings.TrimSpace(req.Name), normalizeDriver(req.Driver)
	record.Provider, record.Status = normalizeProvider(req.Provider, record.Driver), normalizeStatus(req.Status)
	record.ReadEnabled, record.WriteEnabled = req.ReadEnabled, req.WriteEnabled
	record.Endpoint, record.Region, record.Bucket = strings.TrimSpace(req.Endpoint), strings.TrimSpace(req.Region), strings.TrimSpace(req.Bucket)
	record.Prefix, record.ForcePathStyle = strings.Trim(strings.TrimSpace(req.Prefix), "/"), req.ForcePathStyle
	record.PublicBaseURL, record.LocalRoot = strings.TrimSpace(req.PublicBaseURL), strings.TrimSpace(req.LocalRoot)
	record.UpdatedAt, record.UpdatedBy = now, req.UpdatedBy
	if record.Name == "" {
		record.Name = record.Code
	}
	if record.Provider == domainstorageconfig.ProviderR2 && record.Region == "" {
		record.Region = "auto"
	}
	secrets, err := s.secretsForWrite(record, req)
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	if err := s.validate(record, secrets); err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	record.SecretEncrypted, err = s.codec.EncryptJSON(secrets)
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	record.SecretFields = sortedSecretFields(secrets)
	record.SecretFingerprint = secretcodec.Fingerprint(secrets, record.SecretFields)
	return record, nil
}

func (s *Service) secretsForWrite(current domainstorageconfig.ConfigRecord, req domainstorageconfig.WriteRequest) (map[string]any, error) {
	secrets := map[string]any{}
	if len(current.SecretEncrypted) > 0 {
		decoded, err := s.codec.DecryptJSON(current.SecretEncrypted)
		if err != nil {
			return nil, err
		}
		for key, value := range decoded {
			secrets[key] = value
		}
	}
	for _, key := range req.ClearSecrets {
		delete(secrets, strings.TrimSpace(key))
	}
	for key, value := range req.Secrets {
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if value == "" {
			return nil, errs.BadRequest("storage secret cannot be empty; use clear_secrets to remove it")
		}
		if isMaskedSecretPlaceholder(value) {
			return nil, errs.New(400, "INVALID_SECRET_PLACEHOLDER", "masked secret placeholder is not allowed")
		}
		secrets[key] = value
	}
	return secrets, nil
}

func (s *Service) validate(record domainstorageconfig.ConfigRecord, secrets map[string]any) error {
	if record.Driver != domainstorageconfig.DriverLocal && record.Driver != domainstorageconfig.DriverS3 {
		return errs.BadRequest("storage driver must be local or s3")
	}
	if record.Status != domainstorageconfig.StatusEnabled && record.Status != domainstorageconfig.StatusDisabled {
		return errs.BadRequest("storage status must be enabled or disabled")
	}
	if record.Driver == domainstorageconfig.DriverLocal {
		return nil
	}
	if record.Endpoint == "" || record.Region == "" || record.Bucket == "" {
		return errs.BadRequest("s3 endpoint, region, and bucket are required")
	}
	if strings.TrimSpace(fmt.Sprint(secrets[secretAccessKeyID])) == "" || strings.TrimSpace(fmt.Sprint(secrets[secretSecretAccessKey])) == "" {
		return errs.BadRequest("s3 access key id and secret access key are required")
	}
	endpoint, err := url.Parse(record.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return errs.BadRequest("s3 endpoint must include scheme and host")
	}
	if !strings.EqualFold(s.environment, "local") && endpoint.Scheme != "https" && !hostLooksLocal(endpoint.Hostname()) {
		return errs.BadRequest("s3 endpoint must use https outside local environment")
	}
	return nil
}

func (s *Service) bootstrapRecord(updatedBy int64) (domainstorageconfig.ConfigRecord, error) {
	driver, now := normalizeDriver(s.bootstrap.Driver), time.Now().UTC()
	record := domainstorageconfig.ConfigRecord{
		Code: "bootstrap-" + driver, Name: "Bootstrap " + strings.ToUpper(driver), Driver: driver,
		Provider: normalizeProvider("", driver), Status: domainstorageconfig.StatusEnabled,
		ReadEnabled: true, WriteEnabled: true, IsDefault: true, PublicBaseURL: strings.TrimSpace(s.bootstrap.PublicBaseURL),
		LocalRoot: strings.TrimSpace(s.bootstrap.LocalRoot), LastProbeStatus: domainstorageconfig.ProbeStatusSuccess,
		LastProbeMessage: "bootstrap config", Version: 1, UpdatedBy: updatedBy, CreatedAt: now, UpdatedAt: now,
	}
	secrets := map[string]any{}
	if driver == domainstorageconfig.DriverS3 {
		record.Provider, record.Endpoint, record.Region = domainstorageconfig.ProviderCustomS3, strings.TrimSpace(s.bootstrap.S3.Endpoint), strings.TrimSpace(s.bootstrap.S3.Region)
		record.Bucket, record.Prefix, record.ForcePathStyle = strings.TrimSpace(s.bootstrap.S3.Bucket), strings.Trim(strings.TrimSpace(s.bootstrap.S3.Prefix), "/"), s.bootstrap.S3.ForcePathStyle
		secrets[secretAccessKeyID], secrets[secretSecretAccessKey] = strings.TrimSpace(s.bootstrap.S3.AccessKeyID), strings.TrimSpace(s.bootstrap.S3.SecretAccessKey)
	}
	if err := s.validate(record, secrets); err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	encrypted, err := s.codec.EncryptJSON(secrets)
	if err != nil {
		return domainstorageconfig.ConfigRecord{}, err
	}
	record.SecretEncrypted, record.SecretFields = encrypted, sortedSecretFields(secrets)
	record.SecretFingerprint = secretcodec.Fingerprint(secrets, record.SecretFields)
	return record, nil
}

func viewFromRecord(record domainstorageconfig.ConfigRecord) domainstorageconfig.ConfigView {
	view := domainstorageconfig.ConfigView{
		ID: record.ID, Code: record.Code, Name: record.Name, Driver: record.Driver, Provider: record.Provider, Status: record.Status,
		ReadEnabled: record.ReadEnabled, WriteEnabled: record.WriteEnabled, IsDefault: record.IsDefault,
		Endpoint: record.Endpoint, Region: record.Region, Bucket: record.Bucket, Prefix: record.Prefix, ForcePathStyle: record.ForcePathStyle,
		PublicBaseURL: record.PublicBaseURL, LocalRoot: record.LocalRoot,
		SecretStatus: domainstorageconfig.SecretStatus{HasSecret: record.SecretFingerprint != "", Fingerprint: record.SecretFingerprint, SecretFields: append([]string{}, record.SecretFields...)},
		LastProbe:    domainstorageconfig.ProbeView{Status: normalizeProbeStatus(record.LastProbeStatus), CheckedAt: record.LastProbeAt, Message: record.LastProbeMessage},
		Version:      record.Version, UpdatedBy: record.UpdatedBy, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if record.SecretFingerprint != "" {
		updated := record.UpdatedAt
		view.SecretStatus.UpdatedAt = &updated
	}
	return view
}

func normalizeDriver(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domainstorageconfig.DriverLocal
	}
	return value
}

func normalizeProvider(value, driver string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "" {
		return value
	}
	if normalizeDriver(driver) == domainstorageconfig.DriverS3 {
		return domainstorageconfig.ProviderCustomS3
	}
	return domainstorageconfig.ProviderLocal
}

func normalizeStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domainstorageconfig.StatusEnabled
	}
	return value
}

func normalizeProbeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case domainstorageconfig.ProbeStatusSuccess:
		return domainstorageconfig.ProbeStatusSuccess
	case domainstorageconfig.ProbeStatusFailed:
		return domainstorageconfig.ProbeStatusFailed
	default:
		return domainstorageconfig.ProbeStatusNever
	}
}

func sortedSecretFields(secrets map[string]any) []string {
	fields := make([]string, 0, len(secrets))
	for key, value := range secrets {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(fmt.Sprint(value)) != "" {
			fields = append(fields, strings.TrimSpace(key))
		}
	}
	sort.Strings(fields)
	return fields
}

func isMaskedSecretPlaceholder(value string) bool {
	trimmed := strings.TrimSpace(value)
	return len(trimmed) >= 4 && strings.Trim(trimmed, "*•") == ""
}

func hostLooksLocal(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
