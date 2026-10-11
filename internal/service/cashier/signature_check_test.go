package cashier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckAlipaySignatureReportsGatewayVerdicts(t *testing.T) {
	privateKey := alipayTestPrivateKeyPEM(t)

	t.Run("signature accepted then business rejects unknown trade", func(t *testing.T) {
		var signedForm string
		gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			signedForm = r.PostForm.Encode()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"alipay_trade_query_response":{"code":"40004","msg":"Business Failed","sub_code":"ACQ.TRADE_NOT_EXIST","sub_msg":"交易不存在"}}`))
		}))
		defer gateway.Close()

		result, err := CheckAlipaySignature(context.Background(), AlipaySignatureCheckRequest{
			GatewayURL: gateway.URL, AppID: "app-123", AppPrivateKey: privateKey,
		})
		if err != nil {
			t.Fatalf("CheckAlipaySignature: %v", err)
		}
		if !result.SignatureOK || result.Code != "40004" || result.SubCode != "ACQ.TRADE_NOT_EXIST" {
			t.Fatalf("expected signature ok with ACQ.TRADE_NOT_EXIST, got %#v", result)
		}
		if !strings.Contains(signedForm, "method=alipay.trade.query") || !strings.Contains(signedForm, "sign_type=RSA2") {
			t.Fatalf("expected signed alipay.trade.query form, got %s", signedForm)
		}
		hint := AlipaySignatureCheckHint(result)
		if !strings.Contains(hint, "签名验证通过") {
			t.Fatalf("expected pass hint, got %q", hint)
		}
	})

	t.Run("signature rejected by gateway", func(t *testing.T) {
		gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"alipay_trade_query_response":{"code":"40002","msg":"Signature Check Failed","sub_code":"isv.invalid-signature","sub_msg":"验签出错"}}`))
		}))
		defer gateway.Close()

		result, err := CheckAlipaySignature(context.Background(), AlipaySignatureCheckRequest{
			GatewayURL: gateway.URL, AppID: "app-123", AppPrivateKey: privateKey,
		})
		if err != nil {
			t.Fatalf("CheckAlipaySignature: %v", err)
		}
		if result.SignatureOK {
			t.Fatalf("expected signature failure, got %#v", result)
		}
		if hint := AlipaySignatureCheckHint(result); !strings.Contains(hint, "签名验证失败") {
			t.Fatalf("expected failure hint, got %q", hint)
		}
	})

	t.Run("network failure is not reported as key mismatch", func(t *testing.T) {
		result, err := CheckAlipaySignature(context.Background(), AlipaySignatureCheckRequest{
			GatewayURL: "http://127.0.0.1:1/gateway.do", AppID: "app-123", AppPrivateKey: privateKey,
		})
		if err != nil {
			t.Fatalf("CheckAlipaySignature: %v", err)
		}
		if result.SignatureOK || result.NetworkError == "" {
			t.Fatalf("expected network failure, got %#v", result)
		}
		if hint := AlipaySignatureCheckHint(result); !strings.Contains(hint, "无法连接") {
			t.Fatalf("expected network hint, got %q", hint)
		}
	})

	t.Run("missing app id is rejected", func(t *testing.T) {
		if _, err := CheckAlipaySignature(context.Background(), AlipaySignatureCheckRequest{AppPrivateKey: privateKey}); err == nil {
			t.Fatal("expected error for missing app_id")
		}
	})
}
