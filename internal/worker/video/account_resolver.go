package video

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/provider/video/minimax"
	"github.com/fatballfish/pic-gallery/internal/provider/video/seedance"
)

type ExecutionAccount struct {
	RouteCandidateID     int64
	AccountModelID       int64
	ModelAccountID       int64
	ProviderCode         string
	BaseURL              string
	APIKey               string
	ModelCode            string
	CallbackURL          string
	CallbackSecret       string
	Timeout              time.Duration
	ArtifactAllowedHosts []string
}

type ExecutionAccountStore interface {
	GetExecutionAccount(context.Context, ProviderRef) (ExecutionAccount, error)
}

type ExecutionAccountResolver struct {
	store       ExecutionAccountStore
	newProvider func(ExecutionAccount) (providervideo.Provider, error)
}

func NewExecutionAccountResolver(store ExecutionAccountStore) *ExecutionAccountResolver {
	resolver := &ExecutionAccountResolver{store: store}
	resolver.newProvider = buildExecutionProvider
	return resolver
}

func (r *ExecutionAccountResolver) Resolve(ctx context.Context, ref ProviderRef) (ResolvedExecution, error) {
	if r == nil || r.store == nil || ref.RouteCandidateID <= 0 || ref.AccountModelID <= 0 || ref.ModelAccountID <= 0 {
		return ResolvedExecution{}, errors.New("video execution account reference is incomplete")
	}
	account, err := r.store.GetExecutionAccount(ctx, ref)
	if err != nil {
		return ResolvedExecution{}, fmt.Errorf("resolve video execution account: %w", err)
	}
	if account.RouteCandidateID != ref.RouteCandidateID || account.AccountModelID != ref.AccountModelID || account.ModelAccountID != ref.ModelAccountID ||
		!strings.EqualFold(account.ProviderCode, ref.ProviderCode) || account.ModelCode != ref.ModelCode {
		return ResolvedExecution{}, errors.New("video execution account does not match the immutable attempt snapshot")
	}
	provider, err := r.newProvider(account)
	if err != nil {
		return ResolvedExecution{}, err
	}
	return ResolvedExecution{Provider: provider, ArtifactAllowedHosts: append([]string(nil), account.ArtifactAllowedHosts...)}, nil
}

func buildExecutionProvider(account ExecutionAccount) (providervideo.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(account.ProviderCode)) {
	case "seedance":
		return seedance.NewClient(seedance.Config{
			BaseURL: account.BaseURL, APIKey: account.APIKey, ModelCode: account.ModelCode, CallbackURL: account.CallbackURL,
			CallbackSecret: account.CallbackSecret, Timeout: account.Timeout, Verified: true,
		})
	case "minimax":
		return minimax.NewClient(minimax.Config{
			BaseURL: account.BaseURL, APIKey: account.APIKey, ModelCode: account.ModelCode, CallbackURL: account.CallbackURL,
			CallbackSecret: account.CallbackSecret, Timeout: account.Timeout, Verified: true,
		})
	default:
		return nil, fmt.Errorf("unsupported video provider %q", account.ProviderCode)
	}
}
