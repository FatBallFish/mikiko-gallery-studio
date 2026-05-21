package entstore

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/redeemcode"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	_ "github.com/mattn/go-sqlite3"
)

func TestBillingStoreRedeemCodeCreditsPointsOnce(t *testing.T) {
	ctx := context.Background()
	client := openRedeemTestClient(t, "file:redeem-valid?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	now := time.Now().UTC()
	code := client.RedeemCode.Create().
		SetCode("WELCOME100").
		SetStatus("available").
		SetRewardType("points").
		SetRewardValue("12.50000").
		SetValidFrom(now.Add(-time.Hour)).
		SetValidUntil(now.Add(time.Hour)).
		SetMaxRedemptions(2).
		SaveX(ctx)

	store := NewBillingStore(client, 5)
	req := billingservice.RedeemCodeRequest{UserID: 42, Code: "welcome100", IdempotencyKey: "redeem-request-1"}
	balance, err := store.RedeemCode(ctx, req)
	if err != nil {
		t.Fatalf("RedeemCode: %v", err)
	}
	if balance.AvailablePoints != "12.50000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected balance after redeem %#v", balance)
	}

	replay, err := store.RedeemCode(ctx, req)
	if err != nil {
		t.Fatalf("RedeemCode replay: %v", err)
	}
	if replay.AvailablePoints != "12.50000" || replay.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected balance after replay %#v", replay)
	}

	ledgerCount := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(42), pointledger.RedeemCodeIDEQ(int64(code.ID))).
		CountX(ctx)
	if ledgerCount != 1 {
		t.Fatalf("expected one redeem ledger entry, got %d", ledgerCount)
	}
	reloaded := client.RedeemCode.Query().Where(redeemcode.CodeEQ("WELCOME100")).OnlyX(ctx)
	if reloaded.RedeemedCount != 1 || reloaded.LastRedeemedBy == nil || *reloaded.LastRedeemedBy != 42 {
		t.Fatalf("unexpected redeem code counters: %#v", reloaded)
	}
}

func TestBillingStoreRedeemCodeRejectsUnknownCode(t *testing.T) {
	ctx := context.Background()
	client := openRedeemTestClient(t, "file:redeem-unknown?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	store := NewBillingStore(client, 5)
	if _, err := store.RedeemCode(ctx, billingservice.RedeemCodeRequest{UserID: 42, Code: "missing", IdempotencyKey: "redeem-request-1"}); err == nil {
		t.Fatal("expected unknown redeem code to be rejected")
	}
	balance, err := store.GetBalance(ctx, 42)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "0.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("unknown code should not credit balance, got %#v", balance)
	}
}

func openRedeemTestClient(t *testing.T, dsn string) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	if err := client.Schema.Create(context.Background()); err != nil {
		client.Close()
		t.Fatalf("create schema: %v", err)
	}
	return client
}
