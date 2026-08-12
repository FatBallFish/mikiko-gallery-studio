package videocallback

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/provider/video/minimax"
	"github.com/fatballfish/pic-gallery/internal/provider/video/seedance"
)

type Account struct {
	ModelAccountID int64
	AdapterType    string
	BaseURL        string
	Credentials    map[string]string
	TimeoutMS      int
	Extra          map[string]any
}

type AccountStore interface {
	GetCallbackAccount(context.Context, uuid.UUID, string) (Account, error)
}

type AccountResolver struct{ store AccountStore }

func NewAccountResolver(store AccountStore) *AccountResolver { return &AccountResolver{store: store} }

func (r *AccountResolver) ResolveProvider(ctx context.Context, providerCode string, publicID uuid.UUID) (ResolvedProvider, error) {
	if r == nil || r.store == nil {
		return ResolvedProvider{}, fmt.Errorf("video callback account resolver is unavailable")
	}
	account, err := r.store.GetCallbackAccount(ctx, publicID, providerCode)
	if err != nil {
		return ResolvedProvider{}, err
	}
	timeout := time.Duration(account.TimeoutMS) * time.Millisecond
	apiKey := strings.TrimSpace(account.Credentials["api_key"])
	callbackSecret := strings.TrimSpace(account.Credentials["callback_secret"])
	modelCode := strings.TrimSpace(stringOption(account.Extra, "video_callback_model_code"))
	if modelCode == "" {
		modelCode = "callback-verification"
	}
	switch strings.ToLower(account.AdapterType) {
	case "seedance":
		provider, err := seedance.NewClient(seedance.Config{BaseURL: account.BaseURL, APIKey: apiKey, ModelCode: modelCode, CallbackSecret: callbackSecret, Timeout: timeout, Verified: true})
		if err != nil {
			return ResolvedProvider{}, fmt.Errorf("build seedance callback verifier: %w", err)
		}
		return ResolvedProvider{ModelAccountID: account.ModelAccountID, Provider: provider}, nil
	case "minimax":
		provider, err := minimax.NewClient(minimax.Config{BaseURL: account.BaseURL, APIKey: apiKey, ModelCode: modelCode, CallbackSecret: callbackSecret, Timeout: timeout, Verified: true})
		if err != nil {
			return ResolvedProvider{}, fmt.Errorf("build minimax callback verifier: %w", err)
		}
		return ResolvedProvider{ModelAccountID: account.ModelAccountID, Provider: provider}, nil
	default:
		return ResolvedProvider{}, fmt.Errorf("unsupported video callback provider %q", account.AdapterType)
	}
}

func stringOption(options map[string]any, key string) string {
	value, _ := options[key].(string)
	return value
}
