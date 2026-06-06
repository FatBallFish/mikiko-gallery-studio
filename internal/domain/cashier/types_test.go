package cashier

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestRandomProviderInstanceWithReaderSelectsCandidateDeterministically(t *testing.T) {
	if got := RandomProviderInstanceWithReader(strings.NewReader("\x00"), nil); got.ID != 0 {
		t.Fatalf("empty candidates should return zero value, got %#v", got)
	}

	only := []ProviderInstance{{ID: 11, Name: "only"}}
	if got := RandomProviderInstanceWithReader(strings.NewReader("\x00"), only); got.ID != 11 {
		t.Fatalf("single candidate should be selected, got %#v", got)
	}

	candidates := []ProviderInstance{{ID: 101}, {ID: 102}, {ID: 103}}
	if got := RandomProviderInstanceWithReader(strings.NewReader("\x00\x00\x00\x00\x00\x00\x00\x02"), candidates); got.ID != 103 {
		t.Fatalf("reader byte should choose deterministic candidate, got %#v", got)
	}

	if got := RandomProviderInstanceWithReader(failingReader{}, candidates); got.ID != 101 {
		t.Fatalf("reader failure should fall back to first candidate, got %#v", got)
	}
}

func TestProviderInstanceAmountAllowedHonorsMinAndMaxLimits(t *testing.T) {
	instance := ProviderInstance{Limits: map[string]any{
		"min_amount_cny": "5.00000",
		"max_amount_cny": "500.00000",
	}}

	for _, amount := range []string{"5.00000", "10.00000", "500.00000"} {
		if !ProviderInstanceAmountAllowed(instance, decimal.RequireFromString(amount)) {
			t.Fatalf("expected amount %s to be allowed", amount)
		}
	}

	for _, amount := range []string{"4.99999", "500.00001"} {
		if ProviderInstanceAmountAllowed(instance, decimal.RequireFromString(amount)) {
			t.Fatalf("expected amount %s to be rejected", amount)
		}
	}
}

func TestProviderInstanceAmountAllowedRejectsInvalidLimitValues(t *testing.T) {
	instance := ProviderInstance{Limits: map[string]any{
		"min_amount_cny": "not-a-decimal",
	}}

	if ProviderInstanceAmountAllowed(instance, decimal.RequireFromString("1.00000")) {
		t.Fatalf("invalid limits should block payment scheduling")
	}
}
