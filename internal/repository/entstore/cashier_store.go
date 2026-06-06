package entstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/paymentproviderinstance"
	cashierservice "github.com/fatballfish/pic-gallery/internal/service/cashier"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type CashierStore struct {
	client              *repoent.Client
	configEncryptionKey string
}

func NewCashierStore(client *repoent.Client) *CashierStore {
	return NewCashierStoreWithConfigEncryptionKey(client, "")
}

func NewCashierStoreWithConfigEncryptionKey(client *repoent.Client, configEncryptionKey string) *CashierStore {
	return &CashierStore{client: client, configEncryptionKey: strings.TrimSpace(configEncryptionKey)}
}

func (s *CashierStore) ProviderInstances(ctx context.Context) ([]domaincashier.ProviderInstance, error) {
	if s == nil || s.client == nil {
		return nil, errs.Internal("cashier store is not available")
	}
	rows, err := s.client.PaymentProviderInstance.Query().
		Order(repoent.Asc(paymentproviderinstance.FieldSortOrder), repoent.Asc(paymentproviderinstance.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]domaincashier.ProviderInstance, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.paymentProviderInstanceFromEnt(row))
	}
	return items, nil
}

func (s *CashierStore) CreateProviderInstance(ctx context.Context, req domaincashier.ProviderInstance) (domaincashier.ProviderInstance, error) {
	if s == nil || s.client == nil {
		return domaincashier.ProviderInstance{}, errs.Internal("cashier store is not available")
	}
	normalized, err := cashierservice.NormalizeProviderInstance(req, 0, time.Now().UTC())
	if err != nil {
		return domaincashier.ProviderInstance{}, errs.BadRequest(err.Error())
	}
	configEncrypted, err := s.encryptConfig(normalized.Config)
	if err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	row, err := s.client.PaymentProviderInstance.Create().
		SetProviderType(normalized.ProviderType).
		SetName(normalized.Name).
		SetConfigEncrypted(configEncrypted).
		SetCredentialsFingerprint(cashierCredentialsFingerprint(normalized.Config, normalized.UpdatedAt)).
		SetSupportedMethods(normalized.SupportedMethods).
		SetEnabled(normalized.Enabled).
		SetSortOrder(normalized.SortOrder).
		SetSchedulerWeight(normalized.SchedulerWeight).
		SetLimits(normalized.Limits).
		SetRefundEnabled(cashierProviderRefundEnabled(normalized.ProviderType)).
		SetHealthStatus(cashierProviderHealthStatus(normalized)).
		SetLastError(normalized.LastError).
		SetMetadata(map[string]any{}).
		Save(ctx)
	if err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	return s.paymentProviderInstanceFromEnt(row), nil
}

func (s *CashierStore) UpdateProviderInstance(ctx context.Context, instanceID int64, req domaincashier.ProviderInstance) (domaincashier.ProviderInstance, error) {
	if s == nil || s.client == nil {
		return domaincashier.ProviderInstance{}, errs.Internal("cashier store is not available")
	}
	if instanceID <= 0 {
		return domaincashier.ProviderInstance{}, errs.BadRequest("payment provider instance id is required")
	}
	if _, err := s.client.PaymentProviderInstance.Get(ctx, int(instanceID)); err != nil {
		if repoent.IsNotFound(err) {
			return domaincashier.ProviderInstance{}, errs.New(404, errs.CodeNotFound, "payment provider instance not found")
		}
		return domaincashier.ProviderInstance{}, err
	}
	normalized, err := cashierservice.NormalizeProviderInstance(req, instanceID, time.Now().UTC())
	if err != nil {
		return domaincashier.ProviderInstance{}, errs.BadRequest(err.Error())
	}
	configEncrypted, err := s.encryptConfig(normalized.Config)
	if err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	row, err := s.client.PaymentProviderInstance.UpdateOneID(int(instanceID)).
		SetProviderType(normalized.ProviderType).
		SetName(normalized.Name).
		SetConfigEncrypted(configEncrypted).
		SetCredentialsFingerprint(cashierCredentialsFingerprint(normalized.Config, normalized.UpdatedAt)).
		SetSupportedMethods(normalized.SupportedMethods).
		SetEnabled(normalized.Enabled).
		SetSortOrder(normalized.SortOrder).
		SetSchedulerWeight(normalized.SchedulerWeight).
		SetLimits(normalized.Limits).
		SetRefundEnabled(cashierProviderRefundEnabled(normalized.ProviderType)).
		SetHealthStatus(cashierProviderHealthStatus(normalized)).
		SetLastError(normalized.LastError).
		Save(ctx)
	if err != nil {
		return domaincashier.ProviderInstance{}, err
	}
	return s.paymentProviderInstanceFromEnt(row), nil
}

func (s *CashierStore) DeleteProviderInstance(ctx context.Context, instanceID int64) (domaincashier.ProviderInstance, error) {
	if s == nil || s.client == nil {
		return domaincashier.ProviderInstance{}, errs.Internal("cashier store is not available")
	}
	if instanceID <= 0 {
		return domaincashier.ProviderInstance{}, errs.BadRequest("payment provider instance id is required")
	}
	row, err := s.client.PaymentProviderInstance.Get(ctx, int(instanceID))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domaincashier.ProviderInstance{}, errs.New(404, errs.CodeNotFound, "payment provider instance not found")
		}
		return domaincashier.ProviderInstance{}, err
	}
	deleted := s.paymentProviderInstanceFromEnt(row)
	if err := s.client.PaymentProviderInstance.DeleteOneID(int(instanceID)).Exec(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return domaincashier.ProviderInstance{}, errs.New(404, errs.CodeNotFound, "payment provider instance not found")
		}
		return domaincashier.ProviderInstance{}, err
	}
	return deleted, nil
}

func (s *CashierStore) paymentProviderInstanceFromEnt(row *repoent.PaymentProviderInstance) domaincashier.ProviderInstance {
	if row == nil {
		return domaincashier.ProviderInstance{}
	}
	return domaincashier.ProviderInstance{
		ID:               int64(row.ID),
		ProviderType:     row.ProviderType,
		Name:             row.Name,
		Enabled:          row.Enabled,
		SupportedMethods: append([]string{}, row.SupportedMethods...),
		SortOrder:        row.SortOrder,
		SchedulerWeight:  row.SchedulerWeight,
		Limits:           normalizeCashierMap(row.Limits),
		Config:           normalizeCashierMap(s.decryptConfig(row.ConfigEncrypted)),
		ConfigStatus:     configStatusFromProviderRow(row),
		LastError:        row.LastError,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func configStatusFromProviderRow(row *repoent.PaymentProviderInstance) string {
	if row.ProviderType == "mock" || len(row.ConfigEncrypted) > 0 {
		return "configured"
	}
	return "missing"
}

func cashierProviderHealthStatus(instance domaincashier.ProviderInstance) string {
	if strings.TrimSpace(instance.LastError) != "" {
		return "error"
	}
	if instance.ConfigStatus == "configured" {
		return "unknown"
	}
	return "missing_config"
}

func cashierProviderRefundEnabled(providerType string) bool {
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	return providerType != "" && providerType != "mock" && !strings.HasPrefix(providerType, "manual")
}

func cashierCredentialsFingerprint(config map[string]any, updatedAt time.Time) string {
	status := cashierservice.CredentialsStatus(config, updatedAt)
	if fingerprint, ok := status["fingerprint"].(string); ok {
		return fingerprint
	}
	return ""
}

func (s *CashierStore) encryptConfig(config map[string]any) (map[string]any, error) {
	config = normalizeCashierMap(config)
	if len(config) == 0 || strings.TrimSpace(s.configEncryptionKey) == "" {
		return config, nil
	}
	plain, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal cashier provider config: %w", err)
	}
	aead, err := cashierConfigAEAD(s.configEncryptionKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate cashier provider config nonce: %w", err)
	}
	ciphertext := aead.Seal(nonce, nonce, plain, nil)
	return map[string]any{
		"ciphertext": "v1:" + base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}

func (s *CashierStore) decryptConfig(config map[string]any) map[string]any {
	config = normalizeCashierMap(config)
	ciphertext, _ := config["ciphertext"].(string)
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		return config
	}
	plaintext, ok := decryptCashierConfigCiphertext(ciphertext, s.configEncryptionKey)
	if !ok {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(plaintext), &decoded); err != nil {
		return map[string]any{}
	}
	return normalizeCashierMap(decoded)
}

func decryptCashierConfigCiphertext(ciphertext, keyMaterial string) (string, bool) {
	encoded := strings.TrimSpace(ciphertext)
	if !strings.HasPrefix(encoded, "v1:") || strings.TrimSpace(keyMaterial) == "" {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "v1:"))
	if err != nil {
		return "", false
	}
	aead, err := cashierConfigAEAD(keyMaterial)
	if err != nil || len(payload) < aead.NonceSize() {
		return "", false
	}
	nonce := payload[:aead.NonceSize()]
	sealed := payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", false
	}
	return string(plaintext), true
}

func cashierConfigAEAD(keyMaterial string) (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(strings.TrimSpace(keyMaterial)))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("create cashier provider config cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func normalizeCashierMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	for key, raw := range value {
		cloned[key] = raw
	}
	return cloned
}
