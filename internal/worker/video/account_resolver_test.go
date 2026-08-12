package video

import (
	"context"
	"testing"
	"time"

	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
)

func TestExecutionAccountResolverUsesExactAttemptAccountAndModel(t *testing.T) {
	store := &executionAccountStoreStub{account: ExecutionAccount{
		RouteCandidateID: 41, AccountModelID: 42, ModelAccountID: 43, ProviderCode: "seedance", BaseURL: "https://ark.example.test",
		APIKey: "secret", ModelCode: "seedance-2-5", CallbackURL: "https://app.example.test/callback", CallbackSecret: "callback-secret", Timeout: 45 * time.Second,
	}}
	provider := &providerStub{}
	resolver := NewExecutionAccountResolver(store)
	resolver.newProvider = func(account ExecutionAccount) (providervideo.Provider, error) {
		if account.ModelAccountID != 43 || account.AccountModelID != 42 || account.ModelCode != "seedance-2-5" {
			t.Fatalf("provider account = %#v", account)
		}
		return provider, nil
	}
	ref := ProviderRef{RouteCandidateID: 41, AccountModelID: 42, ModelAccountID: 43, ProviderCode: "seedance", ModelCode: "seedance-2-5"}

	got, err := resolver.Resolve(t.Context(), ref)
	if err != nil || got.Provider != provider || store.last != ref {
		t.Fatalf("Resolve() execution=%#v ref=%#v err=%v", got, store.last, err)
	}
}

func TestExecutionAccountResolverRejectsMismatchedAccountSnapshot(t *testing.T) {
	store := &executionAccountStoreStub{account: ExecutionAccount{RouteCandidateID: 1, AccountModelID: 2, ModelAccountID: 999, ProviderCode: "seedance", ModelCode: "seedance-2-5"}}
	resolver := NewExecutionAccountResolver(store)
	_, err := resolver.Resolve(t.Context(), ProviderRef{RouteCandidateID: 1, AccountModelID: 2, ModelAccountID: 3, ProviderCode: "seedance", ModelCode: "seedance-2-5"})
	if err == nil {
		t.Fatal("expected mismatched account snapshot rejection")
	}
}

type executionAccountStoreStub struct {
	account ExecutionAccount
	err     error
	last    ProviderRef
}

func (s *executionAccountStoreStub) GetExecutionAccount(_ context.Context, ref ProviderRef) (ExecutionAccount, error) {
	s.last = ref
	if s.err != nil {
		return ExecutionAccount{}, s.err
	}
	return s.account, nil
}
