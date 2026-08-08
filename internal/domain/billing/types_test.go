package billing

import (
	"strings"
	"testing"
	"time"
)

func TestPopulateLedgerDisplayFieldsFormatsExpireLedger(t *testing.T) {
	entry := PopulateLedgerDisplayFields(LedgerEntry{
		LedgerType:    "expire",
		BalanceBucket: "trial",
		ChangePoints:  "-9.00000",
		BalanceAfter:  "50.00000",
		CreatedAt:     time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
		Reason:        "expired trial grant",
	})

	if entry.Title != "额度过期" {
		t.Fatalf("expected expire title, got %q", entry.Title)
	}
	if entry.BucketType != "trial" || entry.SourceType != "system" {
		t.Fatalf("expected trial/system display fields, got bucket=%q source=%q", entry.BucketType, entry.SourceType)
	}
	if entry.Type != "debit" || entry.Amount != "-9.00000" {
		t.Fatalf("expected debit amount, got type=%q amount=%q", entry.Type, entry.Amount)
	}
	if !strings.Contains(entry.Detail, "体验额度") || !strings.Contains(entry.Detail, "系统") || !strings.Contains(entry.Detail, "expired trial grant") {
		t.Fatalf("expected readable expire detail, got %q", entry.Detail)
	}
}

func TestPopulateLedgerDisplayFieldsLabelsGiftOrderLedger(t *testing.T) {
	entry := PopulateLedgerDisplayFields(LedgerEntry{
		LedgerType: "order_paid", BalanceBucket: "gift", SourceType: "payment_order",
		OrderID: 42, ChangePoints: "5.00000", CreatedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	})
	if entry.Amount != "+5.00000" || entry.BucketType != "gift" || !strings.Contains(entry.Detail, "赠送积分") || !strings.Contains(entry.Detail, "支付订单") {
		t.Fatalf("gift order ledger must expose readable amount and detail, got %#v", entry)
	}
}
