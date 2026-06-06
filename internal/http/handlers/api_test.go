package handlers

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadBoundedBodyRejectsOversizedUnsignedBody(t *testing.T) {
	if _, err := readBoundedBody(strings.NewReader(strings.Repeat("x", 3<<20)), 1); err == nil {
		t.Fatal("expected oversized signed body to be rejected")
	}
	body, err := readBoundedBody(strings.NewReader("small"), 1)
	if err != nil {
		t.Fatalf("expected small body to pass: %v", err)
	}
	if string(body) != "small" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestPromptExcerptNeverReturnsFullPrompt(t *testing.T) {
	for _, prompt := range []string{
		"short prompt",
		"Generate a downloadable banner",
		"生成一张适合电商首页的明亮横幅图",
	} {
		excerpt := promptExcerpt(prompt, 24)
		if excerpt == "" {
			t.Fatalf("expected excerpt for %q", prompt)
		}
		if excerpt == prompt {
			t.Fatalf("excerpt should not expose full prompt %q", prompt)
		}
		if len([]rune(excerpt)) > 24 {
			t.Fatalf("excerpt should be capped at 24 runes, got %d for %q", len([]rune(excerpt)), excerpt)
		}
	}
}

func TestNormalizeCashierQueryStatusMapsProviderTerminalStates(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		wantStatus  string
		wantPaid    bool
		wantMessage string
	}{
		{name: "alipay trade success", status: "TRADE_SUCCESS", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付"},
		{name: "alipay waiting buyer pay", status: "WAIT_BUYER_PAY", wantStatus: "pending", wantPaid: false, wantMessage: "渠道订单未支付或仍在处理中"},
		{name: "alipay trade closed", status: "TRADE_CLOSED", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭"},
		{name: "wxpay revoked", status: "REVOKED", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭"},
		{name: "wxpay refund", status: "REFUND", wantStatus: "refunded", wantPaid: false, wantMessage: "渠道订单已退款"},
		{name: "wxpay pay error", status: "PAYERROR", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单支付失败"},
		{name: "easypay paid", status: "1", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付"},
		{name: "easypay pending", status: "0", wantStatus: "pending", wantPaid: false, wantMessage: "渠道订单未支付或仍在处理中"},
		{name: "jeepay success", status: "2", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付"},
		{name: "jeepay closed", status: "3", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭"},
		{name: "jeepay failed", status: "4", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单支付失败"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCashierQueryStatus(tt.status)
			if got.Status != tt.wantStatus || got.Paid != tt.wantPaid || got.Message != tt.wantMessage {
				t.Fatalf("normalizeCashierQueryStatus(%q)=%#v, want status=%q paid=%v message=%q", tt.status, got, tt.wantStatus, tt.wantPaid, tt.wantMessage)
			}
		})
	}
}

func TestRandomCashierProviderInstanceWithReader(t *testing.T) {
	if got := randomCashierProviderInstanceWithReader(strings.NewReader("\x00"), nil); got.ID != 0 {
		t.Fatalf("expected empty candidates to return zero value, got %#v", got)
	}

	only := []cashierProviderInstance{{ID: 11, Name: "only"}}
	if got := randomCashierProviderInstanceWithReader(strings.NewReader("\x00"), only); got.ID != 11 {
		t.Fatalf("expected single candidate, got %#v", got)
	}

	candidates := []cashierProviderInstance{{ID: 101}, {ID: 102}, {ID: 103}}
	if got := randomCashierProviderInstanceWithReader(strings.NewReader("\x02"), candidates); got.ID != 103 {
		t.Fatalf("expected deterministic reader byte to select third candidate, got %#v", got)
	}

	if got := randomCashierProviderInstanceWithReader(failingReader{}, candidates); got.ID != 101 {
		t.Fatalf("expected failed random source to fall back to first candidate, got %#v", got)
	}
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, errRandomReaderFailed
}

var errRandomReaderFailed = errors.New("random reader failed")

var _ io.Reader = failingReader{}
