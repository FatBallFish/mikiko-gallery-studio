package entstore_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainredeem "github.com/fatballfish/pic-gallery/internal/domain/redeem"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	redeemservice "github.com/fatballfish/pic-gallery/internal/service/redeem"
	_ "github.com/mattn/go-sqlite3"
)

func TestRedeemAdminStoreCreateListStatusAndRedemptions(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:redeemadminstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	adminStore := entstore.NewRedeemAdminStore(client)
	svc := redeemservice.NewServiceWithStore(adminStore)
	validUntil := time.Now().UTC().Add(24 * time.Hour)
	manual, err := svc.CreateCode(ctx, domainredeem.CreateRequest{
		Code:           "manual_copy_1",
		BatchID:        99,
		Status:         "available",
		RewardType:     "points",
		RewardValue:    "8.00000",
		ValidUntil:     validUntil,
		MaxRedemptions: 1,
	})
	if err != nil {
		t.Fatalf("CreateCode manual: %v", err)
	}
	if manual.Code != "MANUAL_COPY_1" {
		t.Fatalf("expected manual code normalized to uppercase, got %q", manual.Code)
	}
	generated, err := svc.CreateCode(ctx, domainredeem.CreateRequest{
		BatchID:        99,
		Status:         "available",
		RewardType:     "points",
		RewardValue:    "5.00000",
		ValidUntil:     validUntil,
		MaxRedemptions: 1,
	})
	if err != nil {
		t.Fatalf("CreateCode generated: %v", err)
	}
	if ok := regexp.MustCompile(`^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{12}$`).MatchString(generated.Code); !ok {
		t.Fatalf("expected generated uppercase safe code, got %q", generated.Code)
	}
	if generated.Code == manual.Code {
		t.Fatal("expected generated code to be unique")
	}

	list, err := adminStore.ListCodes(ctx, domainredeem.ListRequest{Page: 1, PageSize: 10, BatchID: 99, Status: "available", Code: "manual"})
	if err != nil {
		t.Fatalf("ListCodes: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != manual.ID {
		t.Fatalf("unexpected filtered list %#v", list)
	}

	updated, err := adminStore.UpdateStatus(ctx, generated.ID, "disabled")
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if updated.Status != "disabled" {
		t.Fatalf("expected disabled status, got %#v", updated)
	}

	billingStore := entstore.NewBillingStore(client, 5)
	if _, err := billingStore.RedeemCode(ctx, billingservice.RedeemCodeRequest{UserID: 7, Code: manual.Code, IdempotencyKey: "redeem-manual-once"}); err != nil {
		t.Fatalf("RedeemCode: %v", err)
	}
	redemptions, err := adminStore.ListRedemptions(ctx, manual.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListRedemptions: %v", err)
	}
	if redemptions.Total != 1 || len(redemptions.Items) != 1 || redemptions.Items[0].LedgerType != "redeem" || redemptions.Items[0].ChangePoints != "8.00000" {
		t.Fatalf("unexpected redemption ledger %#v", redemptions)
	}
}

func TestRedeemAdminBatchCreateUsesSharedBatchID(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:redeemadminbatch?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	svc := redeemservice.NewServiceWithStore(entstore.NewRedeemAdminStore(client))
	result, err := svc.BatchCreate(ctx, domainredeem.BatchCreateRequest{
		Count:          3,
		Status:         "available",
		RewardType:     "points",
		RewardValue:    "2.50000",
		ValidUntil:     time.Now().UTC().Add(24 * time.Hour),
		MaxRedemptions: 1,
	})
	if err != nil {
		t.Fatalf("BatchCreate: %v", err)
	}
	if result.Count != 3 || len(result.Items) != 3 || result.BatchID == 0 {
		t.Fatalf("unexpected batch result %#v", result)
	}
	seen := map[string]bool{}
	for _, item := range result.Items {
		if item.BatchID != result.BatchID {
			t.Fatalf("expected shared batch id %d, got %#v", result.BatchID, item)
		}
		if seen[item.Code] {
			t.Fatalf("duplicate generated code %q", item.Code)
		}
		seen[item.Code] = true
	}
}
