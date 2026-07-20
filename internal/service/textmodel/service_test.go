package textmodel_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	textmodelservice "github.com/fatballfish/pic-gallery/internal/service/textmodel"
)

type connectionOptimizer struct {
	request textprovider.OptimizeRequest
}

func (o *connectionOptimizer) Optimize(_ context.Context, req textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
	o.request = req
	return textprovider.OptimizeResponse{Text: "OK", RequestID: "connection-request"}, nil
}

func TestServiceEncryptsSecretsAndResolvesEnabledDefault(t *testing.T) {
	ctx := context.Background()
	store := textmodelservice.NewMemoryStore()
	svc := textmodelservice.NewService(store, "text-model-encryption-key")
	account, err := svc.CreateAccount(ctx, domaintextmodel.AccountWriteRequest{
		Name: "Primary", PlatformType: domaintextmodel.PlatformOpenAICompatible,
		APIStyle: domaintextmodel.APIStyleResponses, BaseURL: "https://text.example.com",
		Enabled: true, Secrets: map[string]string{"api_key": "plain-secret"},
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if !account.SecretStatus.HasSecret || account.SecretStatus.Fingerprint == "" {
		t.Fatalf("expected redacted secret status, got %#v", account.SecretStatus)
	}
	record, err := store.GetAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetAccount record: %v", err)
	}
	raw, _ := json.Marshal(record.SecretEncrypted)
	if strings.Contains(string(raw), "plain-secret") {
		t.Fatalf("secret persisted in plaintext: %s", raw)
	}

	model, err := svc.CreateModel(ctx, domaintextmodel.ModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-test", DisplayName: "GPT Test",
		InputPricePerMTok: "1.250000", OutputPricePerMTok: "10.000000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	if _, err := svc.SetDefaultModel(ctx, model.ID); err != nil {
		t.Fatalf("SetDefaultModel: %v", err)
	}
	resolvedAccount, resolvedModel, apiKey, err := svc.ResolveDefaultModel(ctx)
	if err != nil {
		t.Fatalf("ResolveDefaultModel: %v", err)
	}
	if resolvedAccount.ID != account.ID || resolvedModel.ID != model.ID || apiKey != "plain-secret" {
		t.Fatalf("unexpected resolved default: %#v %#v key=%q", resolvedAccount, resolvedModel, apiKey)
	}
}

func TestServiceRejectsVersionConflictsAndInvalidPrices(t *testing.T) {
	ctx := context.Background()
	svc := textmodelservice.NewService(textmodelservice.NewMemoryStore(), "text-model-encryption-key")
	account, err := svc.CreateAccount(ctx, domaintextmodel.AccountWriteRequest{
		Name: "Primary", PlatformType: domaintextmodel.PlatformOpenAICompatible,
		APIStyle: domaintextmodel.APIStyleChatCompletions, BaseURL: "https://text.example.com",
		Enabled: true, Secrets: map[string]string{"api_key": "plain-secret"},
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := svc.UpdateAccount(ctx, account.ID, domaintextmodel.AccountWriteRequest{
		Version: account.Version + 1, Name: account.Name, PlatformType: account.PlatformType,
		APIStyle: account.APIStyle, BaseURL: account.BaseURL, Enabled: true,
	}); err == nil {
		t.Fatal("expected account version conflict")
	}
	if _, err := svc.CreateModel(ctx, domaintextmodel.ModelWriteRequest{
		AccountID: account.ID, ModelCode: "bad-price", DisplayName: "Bad Price",
		InputPricePerMTok: "-1.000000", OutputPricePerMTok: "not-a-price", Currency: "USD", Enabled: true,
	}); err == nil {
		t.Fatal("expected invalid model prices to be rejected")
	}
}

func TestServiceRejectsDefaultModelWhenAccountIsDisabled(t *testing.T) {
	ctx := context.Background()
	svc := textmodelservice.NewService(textmodelservice.NewMemoryStore(), "text-model-encryption-key")
	account, err := svc.CreateAccount(ctx, domaintextmodel.AccountWriteRequest{
		Name: "Disabled", PlatformType: domaintextmodel.PlatformOpenAICompatible,
		APIStyle: domaintextmodel.APIStyleResponses, BaseURL: "https://text.example.com", Enabled: false,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	model, err := svc.CreateModel(ctx, domaintextmodel.ModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-test", DisplayName: "GPT Test",
		InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	if _, err := svc.SetDefaultModel(ctx, model.ID); err == nil {
		t.Fatal("expected disabled account default selection to fail")
	}
}

func TestServiceListsUpdatesAndDeletesTextConfiguration(t *testing.T) {
	ctx := context.Background()
	svc := textmodelservice.NewService(textmodelservice.NewMemoryStore(), "text-model-encryption-key")
	account, err := svc.CreateAccount(ctx, domaintextmodel.AccountWriteRequest{
		Name: "Primary", PlatformType: domaintextmodel.PlatformOpenAICompatible,
		APIStyle: domaintextmodel.APIStyleResponses, BaseURL: "https://text.example.com",
		Enabled: true, Secrets: map[string]string{"api_key": "plain-secret"},
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	model, err := svc.CreateModel(ctx, domaintextmodel.ModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-test", DisplayName: "GPT Test",
		InputPricePerMTok: "1", OutputPricePerMTok: "2", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	accounts, err := svc.ListAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].SecretStatus.Fingerprint == "" {
		t.Fatalf("unexpected accounts: %#v err=%v", accounts, err)
	}
	models, err := svc.ListModels(ctx, account.ID)
	if err != nil || len(models) != 1 {
		t.Fatalf("unexpected models: %#v err=%v", models, err)
	}
	updated, err := svc.UpdateModel(ctx, model.ID, domaintextmodel.ModelWriteRequest{
		Version: model.Version, AccountID: account.ID, ModelCode: model.ModelCode, DisplayName: "Renamed",
		InputPricePerMTok: "3.5", OutputPricePerMTok: "12", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateModel: %v", err)
	}
	if updated.DisplayName != "Renamed" || updated.InputPricePerMTok != "3.500000" || updated.Version != model.Version+1 {
		t.Fatalf("unexpected updated model %#v", updated)
	}
	if err := svc.DeleteAccount(ctx, account.ID); err == nil {
		t.Fatal("account with models must not be deleted")
	}
	if err := svc.DeleteModel(ctx, model.ID); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}
	if err := svc.DeleteAccount(ctx, account.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
}

func TestServiceTestsConfiguredModelConnectionWithoutExposingSecret(t *testing.T) {
	ctx := context.Background()
	store := textmodelservice.NewMemoryStore()
	optimizer := &connectionOptimizer{}
	var receivedKey string
	svc := textmodelservice.NewServiceWithOptimizerFactory(store, "text-model-encryption-key", func(account domaintextmodel.AccountRecord, apiKey string) (textprovider.Optimizer, error) {
		receivedKey = apiKey
		return optimizer, nil
	})
	account, err := svc.CreateAccount(ctx, domaintextmodel.AccountWriteRequest{
		Name: "Primary", PlatformType: domaintextmodel.PlatformOpenAICompatible,
		APIStyle: domaintextmodel.APIStyleResponses, BaseURL: "https://text.example.com",
		Enabled: true, Secrets: map[string]string{"api_key": "connection-secret"},
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	model, err := svc.CreateModel(ctx, domaintextmodel.ModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-test", DisplayName: "GPT Test",
		InputPricePerMTok: "0", OutputPricePerMTok: "0", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	result, err := svc.TestModelConnection(ctx, model.ID)
	if err != nil {
		t.Fatalf("TestModelConnection: %v", err)
	}
	if result.Status != "success" || result.ModelID != model.ID || result.ModelCode != model.ModelCode || result.APIStyle != account.APIStyle {
		t.Fatalf("unexpected connection result %#v", result)
	}
	if receivedKey != "connection-secret" || optimizer.request.Model != model.ModelCode {
		t.Fatalf("connection test used wrong model or credential")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), receivedKey) {
		t.Fatalf("connection result leaked credential: %s", raw)
	}
}
