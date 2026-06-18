package secureconfig

import (
	"context"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainsecureconfig "github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
	"github.com/fatballfish/pic-gallery/internal/service/secretcodec"
	"github.com/fatballfish/pic-gallery/internal/service/smtpdelivery"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	smtpCategory = "smtp"
	smtpKey      = "default"
)

type Record struct {
	Category          string
	Key               string
	PublicValue       map[string]any
	SecretEncrypted   map[string]any
	SecretFingerprint string
	SecretFields      []string
	Version           int64
	UpdatedBy         int64
	UpdatedAt         time.Time
}

type Store interface {
	Get(ctx context.Context, category, key string) (Record, bool, error)
	Save(ctx context.Context, record Record) (Record, error)
}

type SMTPConnectivityValidator func(ctx context.Context, cfg config.SMTPConfig) error

type Service struct {
	store         Store
	codec         *secretcodec.Codec
	fallback      config.SMTPConfig
	environment   string
	smtpValidator SMTPConnectivityValidator
}

func NewService(store Store, encryptionKey string, fallback config.SMTPConfig, environment string) *Service {
	return &Service{
		store:         store,
		codec:         secretcodec.New(encryptionKey),
		fallback:      fallback,
		environment:   strings.TrimSpace(environment),
		smtpValidator: smtpdelivery.ValidateConnectivity,
	}
}

func (s *Service) SetSMTPConnectivityValidator(validator SMTPConnectivityValidator) {
	if validator == nil {
		s.smtpValidator = smtpdelivery.ValidateConnectivity
		return
	}
	s.smtpValidator = validator
}

func (s *Service) GetSMTPConfig(ctx context.Context) (domainsecureconfig.SMTPConfigView, error) {
	record, ok, err := s.getSMTPRecord(ctx)
	if err != nil {
		return domainsecureconfig.SMTPConfigView{}, err
	}
	if !ok {
		return smtpViewFromPublic(map[string]any{}, domainsecureconfig.SecretStatus{}, 0, time.Time{}), nil
	}
	status := secretStatus(record)
	return smtpViewFromPublic(record.PublicValue, status, record.Version, record.UpdatedAt), nil
}

func (s *Service) UpdateSMTPConfig(ctx context.Context, req domainsecureconfig.UpdateSMTPConfigRequest) (domainsecureconfig.SMTPConfigView, error) {
	if s == nil || s.store == nil {
		return domainsecureconfig.SMTPConfigView{}, errs.Internal("secure config store is not available")
	}
	current, exists, err := s.getSMTPRecord(ctx)
	if err != nil {
		return domainsecureconfig.SMTPConfigView{}, err
	}
	if exists && req.Version > 0 && req.Version != current.Version {
		return domainsecureconfig.SMTPConfigView{}, errs.New(409, errs.CodeConflict, "smtp config version conflict")
	}
	public := smtpPublicValue(req)
	secrets, err := s.smtpSecretsForWrite(ctx, current, req)
	if err != nil {
		return domainsecureconfig.SMTPConfigView{}, err
	}
	if err := validateSMTPWrite(req, secrets); err != nil {
		return domainsecureconfig.SMTPConfigView{}, err
	}
	if req.Enabled {
		cfg := smtpConfigFromPublic(public)
		cfg.Password = strings.TrimSpace(fmt.Sprint(secrets["password"]))
		if err := s.validateSMTPConnectivity(ctx, cfg); err != nil {
			return domainsecureconfig.SMTPConfigView{}, errs.BadRequest("smtp connectivity validation failed: " + err.Error())
		}
	}
	encrypted, err := s.codec.EncryptJSON(secrets)
	if err != nil {
		return domainsecureconfig.SMTPConfigView{}, err
	}
	secretFields := sortedSecretFields(secrets)
	record := Record{
		Category:          smtpCategory,
		Key:               smtpKey,
		PublicValue:       public,
		SecretEncrypted:   encrypted,
		SecretFingerprint: secretcodec.Fingerprint(secrets, secretFields),
		SecretFields:      secretFields,
		Version:           1,
		UpdatedBy:         req.UpdatedBy,
	}
	if exists {
		record.Version = current.Version + 1
	}
	saved, err := s.store.Save(ctx, record)
	if err != nil {
		return domainsecureconfig.SMTPConfigView{}, err
	}
	return smtpViewFromPublic(saved.PublicValue, secretStatus(saved), saved.Version, saved.UpdatedAt), nil
}

func (s *Service) ResolveSMTPConfig(ctx context.Context) (config.SMTPConfig, bool, error) {
	record, ok, err := s.getSMTPRecord(ctx)
	if err != nil || !ok {
		return config.SMTPConfig{}, false, err
	}
	if !boolFromAny(record.PublicValue["enabled"]) {
		return config.SMTPConfig{}, false, nil
	}
	secrets, err := s.codec.DecryptJSON(record.SecretEncrypted)
	if err != nil {
		return config.SMTPConfig{}, false, err
	}
	cfg := smtpConfigFromPublic(record.PublicValue)
	cfg.Password = strings.TrimSpace(fmt.Sprint(secrets["password"]))
	return cfg, smtpConfigured(cfg), nil
}

func (s *Service) getSMTPRecord(ctx context.Context) (Record, bool, error) {
	if s == nil || s.store == nil {
		return Record{}, false, nil
	}
	return s.store.Get(ctx, smtpCategory, smtpKey)
}

func (s *Service) validateSMTPConnectivity(ctx context.Context, cfg config.SMTPConfig) error {
	validator := smtpdelivery.ValidateConnectivity
	if s != nil && s.smtpValidator != nil {
		validator = s.smtpValidator
	}
	return validator(ctx, cfg)
}

func (s *Service) smtpSecretsForWrite(ctx context.Context, current Record, req domainsecureconfig.UpdateSMTPConfigRequest) (map[string]any, error) {
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
		if strings.EqualFold(strings.TrimSpace(key), "password") {
			delete(secrets, "password")
		}
	}
	if req.Secrets != nil {
		if password, ok := req.Secrets["password"]; ok {
			password = strings.TrimSpace(password)
			if password == "" {
				return nil, errs.BadRequest("smtp password cannot be empty; use clear_secrets to remove it")
			}
			if isMaskedSecretPlaceholder(password) {
				return nil, errs.New(400, "INVALID_SECRET_PLACEHOLDER", "masked secret placeholder is not allowed")
			}
			secrets["password"] = password
		}
	}
	return secrets, nil
}

func validateSMTPWrite(req domainsecureconfig.UpdateSMTPConfigRequest, secrets map[string]any) *errs.Error {
	if !req.Enabled {
		return nil
	}
	if strings.TrimSpace(req.Host) == "" || req.Port <= 0 || req.Port > 65535 || strings.TrimSpace(req.From) == "" {
		return errs.BadRequest("smtp host, port, and from are required when smtp is enabled")
	}
	if _, err := mail.ParseAddress(req.From); err != nil {
		return errs.BadRequest("smtp from must be a valid email address")
	}
	if strings.TrimSpace(req.Username) != "" && strings.TrimSpace(fmt.Sprint(secrets["password"])) == "" {
		return errs.BadRequest("smtp password is required when username is configured")
	}
	return nil
}

func smtpPublicValue(req domainsecureconfig.UpdateSMTPConfigRequest) map[string]any {
	return map[string]any{
		"enabled":              req.Enabled,
		"host":                 strings.TrimSpace(req.Host),
		"port":                 req.Port,
		"username":             strings.TrimSpace(req.Username),
		"from":                 strings.TrimSpace(req.From),
		"starttls":             req.StartTLS,
		"insecure_skip_verify": req.InsecureSkipVerify,
	}
}

func smtpViewFromPublic(public map[string]any, status domainsecureconfig.SecretStatus, version int64, updatedAt time.Time) domainsecureconfig.SMTPConfigView {
	cfg := smtpConfigFromPublic(public)
	return domainsecureconfig.SMTPConfigView{
		Enabled:            boolFromAny(public["enabled"]),
		Host:               cfg.Host,
		Port:               cfg.Port,
		Username:           cfg.Username,
		From:               cfg.From,
		StartTLS:           cfg.StartTLS,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		SecretStatus:       status,
		Version:            version,
		UpdatedAt:          updatedAt,
	}
}

func smtpConfigFromPublic(public map[string]any) config.SMTPConfig {
	return config.SMTPConfig{
		Host:               strings.TrimSpace(fmt.Sprint(public["host"])),
		Port:               intFromAny(public["port"]),
		Username:           strings.TrimSpace(fmt.Sprint(public["username"])),
		From:               strings.TrimSpace(fmt.Sprint(public["from"])),
		StartTLS:           boolFromAny(public["starttls"]),
		InsecureSkipVerify: boolFromAny(public["insecure_skip_verify"]),
	}
}

func secretStatus(record Record) domainsecureconfig.SecretStatus {
	if strings.TrimSpace(record.SecretFingerprint) == "" {
		return domainsecureconfig.SecretStatus{HasSecret: false}
	}
	updatedAt := record.UpdatedAt
	return domainsecureconfig.SecretStatus{
		HasSecret:    true,
		Fingerprint:  record.SecretFingerprint,
		UpdatedAt:    &updatedAt,
		SecretFields: append([]string{}, record.SecretFields...),
	}
}

func sortedSecretFields(secrets map[string]any) []string {
	fields := make([]string, 0, len(secrets))
	for key, value := range secrets {
		if strings.TrimSpace(fmt.Sprint(value)) == "" {
			continue
		}
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

func smtpConfigured(cfg config.SMTPConfig) bool {
	return strings.TrimSpace(cfg.Host) != "" && cfg.Port > 0 && strings.TrimSpace(cfg.From) != ""
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func isMaskedSecretPlaceholder(value string) bool {
	if len(value) < 4 {
		return false
	}
	for _, char := range value {
		if char != '*' && char != '•' {
			return false
		}
	}
	return true
}
