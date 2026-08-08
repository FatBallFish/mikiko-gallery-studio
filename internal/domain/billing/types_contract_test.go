package billing

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRemediationBillingTypesExposeCreditExpiryJSONContract(t *testing.T) {
	validDays := 30
	creditedAt := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	expiresAt := creditedAt.Add(30 * 24 * time.Hour)

	assertJSONKeys(t, SubscriptionPlan{CreditExpiryEnabled: true}, "credit_expiry_enabled")
	assertJSONKeys(t, CreateSubscriptionPlanRequest{CreditExpiryEnabled: true}, "credit_expiry_enabled")
	assertJSONKeys(t, UpdateSubscriptionPlanRequest{CreditExpiryEnabled: true}, "credit_expiry_enabled")
	assertJSONKeys(t, PaymentOrder{
		CreditExpiryEnabled: true,
		CreditValidDays:     &validDays,
		CreditedAt:          &creditedAt,
		CreditExpiresAt:     &expiresAt,
	}, "credit_expiry_enabled", "credit_valid_days", "credited_at", "credit_expires_at")
}

func TestPaymentOrderNullableCreditSnapshotsAreOmittedWhenAbsent(t *testing.T) {
	payload := marshalJSONObject(t, PaymentOrder{})
	for _, key := range []string{"credit_valid_days", "credited_at", "credit_expires_at"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("PaymentOrder JSON unexpectedly contains absent nullable field %q", key)
		}
	}
}

func TestPermanentFixedPackageOmitsDurationDays(t *testing.T) {
	planPayload := marshalJSONObject(t, SubscriptionPlan{CreditExpiryEnabled: false, DurationDays: nil})
	if _, ok := planPayload["duration_days"]; ok {
		t.Fatalf("permanent SubscriptionPlan must omit duration_days: %#v", planPayload)
	}
	requestPayload := marshalJSONObject(t, CreateSubscriptionPlanRequest{CreditExpiryEnabled: false, DurationDays: nil})
	if _, ok := requestPayload["duration_days"]; ok {
		t.Fatalf("permanent create request must omit duration_days: %#v", requestPayload)
	}
	updatePayload := marshalJSONObject(t, UpdateSubscriptionPlanRequest{CreditExpiryEnabled: false, DurationDays: nil})
	if _, ok := updatePayload["duration_days"]; ok {
		t.Fatalf("permanent update request must omit duration_days: %#v", updatePayload)
	}
}

func TestExpiringFixedPackageIncludesDurationDays(t *testing.T) {
	days := 30
	payload := marshalJSONObject(t, SubscriptionPlan{CreditExpiryEnabled: true, DurationDays: &days})
	if payload["duration_days"] != float64(days) {
		t.Fatalf("expiring SubscriptionPlan duration_days = %#v, want %d", payload["duration_days"], days)
	}
}

func assertJSONKeys(t *testing.T, value any, keys ...string) {
	t.Helper()
	payload := marshalJSONObject(t, value)
	for _, key := range keys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("%T JSON is missing key %q: %#v", value, key, payload)
		}
	}
}

func marshalJSONObject(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode %T JSON: %v", value, err)
	}
	return payload
}
