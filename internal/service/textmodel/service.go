package textmodel

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	textopenai "github.com/fatballfish/pic-gallery/internal/provider/text/openai"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/service/secretcodec"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	store   Store
	codec   *secretcodec.Codec
	factory OptimizerFactory
}

type OptimizerFactory func(account domaintextmodel.AccountRecord, apiKey string) (textprovider.Optimizer, error)

type ConnectionTestResult struct {
	Status    string `json:"status"`
	ModelID   int64  `json:"model_id"`
	ModelCode string `json:"model_code"`
	APIStyle  string `json:"api_style"`
	LatencyMS int64  `json:"latency_ms"`
}

func NewService(store Store, encryptionKey string) *Service {
	return NewServiceWithOptimizerFactory(store, encryptionKey, nil)

}

func NewServiceWithOptimizerFactory(store Store, encryptionKey string, factory OptimizerFactory) *Service {
	if factory == nil {
		factory = func(account domaintextmodel.AccountRecord, apiKey string) (textprovider.Optimizer, error) {
			return textopenai.NewClient(textopenai.Config{BaseURL: account.BaseURL, APIKey: apiKey, APIStyle: account.APIStyle})
		}
	}
	return &Service{store: store, codec: secretcodec.New(encryptionKey), factory: factory}
}

func (s *Service) ListAccounts(ctx context.Context) ([]domaintextmodel.Account, error) {
	records, err := s.store.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domaintextmodel.Account, 0, len(records))
	for _, record := range records {
		result = append(result, accountView(record))
	}
	return result, nil
}

func (s *Service) ListModels(ctx context.Context, accountID int64) ([]domaintextmodel.Model, error) {
	return s.store.ListModels(ctx, accountID)
}

func (s *Service) CreateAccount(ctx context.Context, req domaintextmodel.AccountWriteRequest) (domaintextmodel.Account, error) {
	record, err := s.accountRecordForWrite(ctx, domaintextmodel.AccountRecord{}, req)
	if err != nil {
		return domaintextmodel.Account{}, err
	}
	record.Version = 1
	saved, err := s.store.CreateAccount(ctx, record)
	if err != nil {
		return domaintextmodel.Account{}, mapStoreError(err)
	}
	return accountView(saved), nil
}

func (s *Service) UpdateAccount(ctx context.Context, accountID int64, req domaintextmodel.AccountWriteRequest) (domaintextmodel.Account, error) {
	current, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return domaintextmodel.Account{}, mapStoreError(err)
	}
	if req.Version != current.Version {
		return domaintextmodel.Account{}, errs.New(409, errs.CodeConflict, "text model account version conflict")
	}
	record, err := s.accountRecordForWrite(ctx, current, req)
	if err != nil {
		return domaintextmodel.Account{}, err
	}
	record.ID, record.Version = current.ID, current.Version+1
	saved, err := s.store.UpdateAccount(ctx, record)
	if err != nil {
		return domaintextmodel.Account{}, mapStoreError(err)
	}
	return accountView(saved), nil
}

func (s *Service) DeleteAccount(ctx context.Context, accountID int64) error {
	return mapStoreError(s.store.DeleteAccount(ctx, accountID))
}

func (s *Service) CreateModel(ctx context.Context, req domaintextmodel.ModelWriteRequest) (domaintextmodel.Model, error) {
	model, err := modelForWrite(domaintextmodel.Model{}, req)
	if err != nil {
		return domaintextmodel.Model{}, err
	}
	if _, err := s.store.GetAccount(ctx, model.AccountID); err != nil {
		return domaintextmodel.Model{}, mapStoreError(err)
	}
	model.Version = 1
	created, err := s.store.CreateModel(ctx, model)
	if err != nil {
		return domaintextmodel.Model{}, mapStoreError(err)
	}
	return created, nil
}

func (s *Service) UpdateModel(ctx context.Context, modelID int64, req domaintextmodel.ModelWriteRequest) (domaintextmodel.Model, error) {
	current, err := s.store.GetModel(ctx, modelID)
	if err != nil {
		return domaintextmodel.Model{}, mapStoreError(err)
	}
	if req.Version != current.Version {
		return domaintextmodel.Model{}, errs.New(409, errs.CodeConflict, "text model version conflict")
	}
	model, err := modelForWrite(current, req)
	if err != nil {
		return domaintextmodel.Model{}, err
	}
	if _, err := s.store.GetAccount(ctx, model.AccountID); err != nil {
		return domaintextmodel.Model{}, mapStoreError(err)
	}
	model.ID = current.ID
	model.IsDefault = current.IsDefault && model.Enabled
	model.Version = current.Version + 1
	updated, err := s.store.UpdateModel(ctx, model)
	if err != nil {
		return domaintextmodel.Model{}, mapStoreError(err)
	}
	return updated, nil
}

func (s *Service) DeleteModel(ctx context.Context, modelID int64) error {
	return mapStoreError(s.store.DeleteModel(ctx, modelID))
}

func (s *Service) SetDefaultModel(ctx context.Context, modelID int64) (domaintextmodel.Model, error) {
	model, err := s.store.SetDefaultModel(ctx, modelID)
	if err != nil {
		return domaintextmodel.Model{}, mapStoreError(err)
	}
	return model, nil
}

func (s *Service) ResolveDefaultModel(ctx context.Context) (domaintextmodel.AccountRecord, domaintextmodel.Model, string, error) {
	account, model, err := s.store.GetDefaultModel(ctx)
	if err != nil {
		return domaintextmodel.AccountRecord{}, domaintextmodel.Model{}, "", mapStoreError(err)
	}
	apiKey, err := s.decryptAPIKey(account)
	if err != nil {
		return domaintextmodel.AccountRecord{}, domaintextmodel.Model{}, "", err
	}
	return account, model, apiKey, nil
}

func (s *Service) TestModelConnection(ctx context.Context, modelID int64) (ConnectionTestResult, error) {
	model, err := s.store.GetModel(ctx, modelID)
	if err != nil {
		return ConnectionTestResult{}, mapStoreError(err)
	}
	account, err := s.store.GetAccount(ctx, model.AccountID)
	if err != nil {
		return ConnectionTestResult{}, mapStoreError(err)
	}
	if !account.Enabled || !model.Enabled {
		return ConnectionTestResult{}, errs.New(409, errs.CodeConflict, "text model and account must be enabled before testing")
	}
	apiKey, err := s.decryptAPIKey(account)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	optimizer, err := s.factory(account, apiKey)
	if err != nil {
		return ConnectionTestResult{}, errs.New(409, errs.CodeConflict, "text model connection configuration is invalid")
	}
	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	startedAt := time.Now()
	response, err := optimizer.Optimize(testCtx, textprovider.OptimizeRequest{
		Model: model.ModelCode, SystemPrompt: "Return only the word OK.", Prompt: "Connection test", MaxOutputTokens: 8,
	})
	if err != nil || strings.TrimSpace(response.Text) == "" {
		return ConnectionTestResult{}, errs.New(502, errs.CodeUpstreamUnavailable, "text model connection test failed")
	}
	return ConnectionTestResult{
		Status: "success", ModelID: model.ID, ModelCode: model.ModelCode, APIStyle: account.APIStyle,
		LatencyMS: time.Since(startedAt).Milliseconds(),
	}, nil
}

func (s *Service) decryptAPIKey(account domaintextmodel.AccountRecord) (string, error) {
	secrets, err := s.codec.DecryptJSON(account.SecretEncrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt text model credential: %w", err)
	}
	apiKey := strings.TrimSpace(fmt.Sprint(secrets["api_key"]))
	if apiKey == "" {
		return "", errs.New(409, errs.CodeConflict, "text model credential is not configured")
	}
	return apiKey, nil
}

func (s *Service) accountRecordForWrite(_ context.Context, current domaintextmodel.AccountRecord, req domaintextmodel.AccountWriteRequest) (domaintextmodel.AccountRecord, error) {
	name := strings.TrimSpace(req.Name)
	platformType := strings.ToLower(strings.TrimSpace(req.PlatformType))
	apiStyle := strings.ToLower(strings.TrimSpace(req.APIStyle))
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	parsed, parseErr := url.Parse(baseURL)
	if name == "" || platformType != domaintextmodel.PlatformOpenAICompatible || (apiStyle != domaintextmodel.APIStyleChatCompletions && apiStyle != domaintextmodel.APIStyleResponses) {
		return domaintextmodel.AccountRecord{}, errs.BadRequest("invalid text model account configuration")
	}
	if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
		return domaintextmodel.AccountRecord{}, errs.BadRequest("invalid text model base_url")
	}
	secrets := map[string]any{}
	if len(current.SecretEncrypted) > 0 {
		decoded, err := s.codec.DecryptJSON(current.SecretEncrypted)
		if err != nil {
			return domaintextmodel.AccountRecord{}, err
		}
		for key, value := range decoded {
			secrets[key] = value
		}
	}
	for _, key := range req.ClearSecrets {
		if strings.EqualFold(strings.TrimSpace(key), "api_key") {
			delete(secrets, "api_key")
		}
	}
	if value, ok := req.Secrets["api_key"]; ok {
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, "***") || strings.Contains(value, "••") {
			return domaintextmodel.AccountRecord{}, errs.BadRequest("invalid text model api_key")
		}
		secrets["api_key"] = value
	}
	if req.Enabled && strings.TrimSpace(fmt.Sprint(secrets["api_key"])) == "" {
		return domaintextmodel.AccountRecord{}, errs.BadRequest("api_key is required when text model account is enabled")
	}
	encrypted, err := s.codec.EncryptJSON(secrets)
	if err != nil {
		return domaintextmodel.AccountRecord{}, err
	}
	return domaintextmodel.AccountRecord{
		Name: name, PlatformType: platformType, APIStyle: apiStyle, BaseURL: baseURL,
		SecretEncrypted: encrypted, SecretFingerprint: secretcodec.Fingerprint(secrets, []string{"api_key"}), Enabled: req.Enabled,
	}, nil
}

func modelForWrite(current domaintextmodel.Model, req domaintextmodel.ModelWriteRequest) (domaintextmodel.Model, error) {
	accountID := req.AccountID
	if accountID == 0 {
		accountID = current.AccountID
	}
	modelCode := strings.TrimSpace(req.ModelCode)
	displayName := strings.TrimSpace(req.DisplayName)
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if accountID <= 0 || modelCode == "" {
		return domaintextmodel.Model{}, errs.BadRequest("account_id and model_code are required")
	}
	if displayName == "" {
		displayName = modelCode
	}
	if currency == "" {
		currency = "USD"
	}
	inputPrice, err := normalizePrice(req.InputPricePerMTok)
	if err != nil {
		return domaintextmodel.Model{}, err
	}
	outputPrice, err := normalizePrice(req.OutputPricePerMTok)
	if err != nil {
		return domaintextmodel.Model{}, err
	}
	return domaintextmodel.Model{
		AccountID: accountID, ModelCode: modelCode, DisplayName: displayName,
		InputPricePerMTok: inputPrice, OutputPricePerMTok: outputPrice, Currency: currency, Enabled: req.Enabled,
	}, nil
}

func normalizePrice(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "0"
	}
	value, err := decimal.NewFromString(raw)
	if err != nil || value.IsNegative() {
		return "", errs.BadRequest("text model token prices must be non-negative decimals")
	}
	return value.Round(6).StringFixed(6), nil
}

func accountView(record domaintextmodel.AccountRecord) domaintextmodel.Account {
	status := domaintextmodel.SecretStatus{HasSecret: record.SecretFingerprint != "", Fingerprint: record.SecretFingerprint}
	if status.HasSecret && !record.UpdatedAt.IsZero() {
		updatedAt := record.UpdatedAt
		status.UpdatedAt = &updatedAt
	}
	return domaintextmodel.Account{
		ID: record.ID, Name: record.Name, PlatformType: record.PlatformType, APIStyle: record.APIStyle,
		BaseURL: record.BaseURL, Enabled: record.Enabled, SecretStatus: status, Version: record.Version,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func mapStoreError(err error) error {
	switch {
	case errors.Is(err, repoerr.ErrNotFound):
		return errs.New(404, errs.CodeNotFound, "text model configuration not found")
	case errors.Is(err, repoerr.ErrConflict):
		return errs.New(409, errs.CodeConflict, "text model configuration conflict")
	default:
		return err
	}
}
