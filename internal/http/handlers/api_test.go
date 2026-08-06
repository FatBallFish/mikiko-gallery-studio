package handlers

import (
	"encoding/base64"
	"strings"
	"testing"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	cashierservice "github.com/fatballfish/pic-gallery/internal/service/cashier"
)

func TestPublicGalleryDetailPreservesReusableCreationConfiguration(t *testing.T) {
	item := domainimagetask.GalleryImage{
		ID: "image-a", Prompt: "full prompt", TaskType: "image_edit", RouteModelCode: "plus",
		SizeMode: "pixel", RequestedSize: "1536x1024", BaseResolution: "2k", AspectRatio: "3:2",
		Quality: "high", OutputFormat: "webp", OutputCompression: 72, Moderation: "low", OutputImageCount: 4,
	}
	detail := publicGalleryDetailItem(item)
	if detail.SizeMode != item.SizeMode || detail.RequestedSize != item.RequestedSize || detail.OutputFormat != item.OutputFormat || detail.OutputCompression != item.OutputCompression || detail.Moderation != item.Moderation || detail.OutputImageCount != item.OutputImageCount {
		t.Fatalf("public detail lost reusable creation configuration: %#v", detail)
	}
}

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

func TestOpenAPIRequestBodyLimitAccountsForUploadEncoding(t *testing.T) {
	const imageBytes int64 = 20 * 1024 * 1024
	jsonLimit := openAPIRequestBodyLimit("/api/open/image/v1/reference-assets/uploads", "application/json", imageBytes)
	wantJSONMinimum := int64(base64.StdEncoding.EncodedLen(int(imageBytes))) + referenceAssetMultipartOverheadBytes
	if jsonLimit < wantJSONMinimum {
		t.Fatalf("JSON upload limit %d cannot carry 20 MB base64 payload; want at least %d", jsonLimit, wantJSONMinimum)
	}
	multipartLimit := openAPIRequestBodyLimit("/api/open/image/v1/reference-assets", "multipart/form-data; boundary=test", imageBytes)
	if multipartLimit != imageBytes+referenceAssetMultipartOverheadBytes {
		t.Fatalf("multipart upload limit = %d, want %d", multipartLimit, imageBytes+referenceAssetMultipartOverheadBytes)
	}
}

func TestOpenAPIRequestBodyLimitClampsPersistedImagePolicy(t *testing.T) {
	const unsafePersistedLimit = int64(10 * 1024 * 1024 * 1024)
	got := openAPIRequestBodyLimit("/api/open/image/v1/reference-assets/uploads", "application/json", unsafePersistedLimit)
	want := int64(base64.StdEncoding.EncodedLen(assetservice.MaxImageAttachmentSizeMB*1024*1024)) + referenceAssetMultipartOverheadBytes
	if got != want {
		t.Fatalf("OpenAPI body limit = %d, want hard-clamped %d", got, want)
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
		name             string
		status           string
		wantStatus       string
		wantPaid         bool
		wantMessage      string
		wantRiskCategory string
		wantActionHint   string
	}{
		{name: "alipay trade success", status: "TRADE_SUCCESS", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付", wantRiskCategory: "paid", wantActionHint: "核对本地到账"},
		{name: "alipay waiting buyer pay", status: "WAIT_BUYER_PAY", wantStatus: "pending", wantPaid: false, wantMessage: "渠道订单未支付或仍在处理中", wantRiskCategory: "pending", wantActionHint: "稍后可再次查单"},
		{name: "alipay trade closed", status: "TRADE_CLOSED", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭", wantRiskCategory: "closed", wantActionHint: "重新创建订单"},
		{name: "wxpay revoked", status: "REVOKED", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭", wantRiskCategory: "closed", wantActionHint: "重新创建订单"},
		{name: "wxpay refund", status: "REFUND", wantStatus: "refunded", wantPaid: false, wantMessage: "渠道订单已退款", wantRiskCategory: "refunded", wantActionHint: "本地退款流水"},
		{name: "wxpay pay error", status: "PAYERROR", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单支付失败", wantRiskCategory: "channel_error", wantActionHint: "商户后台"},
		{name: "easypay paid", status: "1", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付", wantRiskCategory: "paid", wantActionHint: "核对本地到账"},
		{name: "easypay pending", status: "0", wantStatus: "pending", wantPaid: false, wantMessage: "渠道订单未支付或仍在处理中", wantRiskCategory: "pending", wantActionHint: "稍后可再次查单"},
		{name: "jeepay success", status: "2", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付", wantRiskCategory: "paid", wantActionHint: "核对本地到账"},
		{name: "jeepay closed", status: "3", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭", wantRiskCategory: "closed", wantActionHint: "重新创建订单"},
		{name: "jeepay failed", status: "4", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单支付失败", wantRiskCategory: "channel_error", wantActionHint: "商户后台"},
		{name: "provider risk", status: "risk_control", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单被风控拦截", wantRiskCategory: "risk_control", wantActionHint: "更换支付渠道"},
		{name: "provider limited", status: "limited", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单触发限额限制", wantRiskCategory: "channel_limited", wantActionHint: "切换备用渠道"},
		{name: "provider signature", status: "sign_error", wantStatus: "failed", wantPaid: false, wantMessage: "渠道验签或签名配置异常", wantRiskCategory: "signature_error", wantActionHint: "检查商户密钥"},
		{name: "provider amount mismatch", status: "amount_mismatch", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单金额与本地订单不一致", wantRiskCategory: "amount_mismatch", wantActionHint: "暂停到账"},
		{name: "provider account abnormal", status: "merchant_disabled", wantStatus: "failed", wantPaid: false, wantMessage: "渠道商户账号状态异常", wantRiskCategory: "account_abnormal", wantActionHint: "切换备用账号"},
		{name: "provider timeout", status: "timeout", wantStatus: "failed", wantPaid: false, wantMessage: "渠道查单超时或网络异常", wantRiskCategory: "channel_timeout", wantActionHint: "稍后重试"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cashierservice.NormalizeQueryStatus(tt.status)
			if got.Status != tt.wantStatus || got.Paid != tt.wantPaid || got.Message != tt.wantMessage || got.RiskCategory != tt.wantRiskCategory || !strings.Contains(got.ActionHint, tt.wantActionHint) {
				t.Fatalf("NormalizeQueryStatus(%q)=%#v, want status=%q paid=%v message=%q risk_category=%q action_hint containing %q", tt.status, got, tt.wantStatus, tt.wantPaid, tt.wantMessage, tt.wantRiskCategory, tt.wantActionHint)
			}
		})
	}
}
