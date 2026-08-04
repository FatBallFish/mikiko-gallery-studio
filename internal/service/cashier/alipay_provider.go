package cashier

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type CallbackURLConfig struct {
	SiteBaseURL string
}

func NewAlipayPaymentDisplayBuilder(callbacks CallbackURLConfig) PaymentDisplayBuilder {
	return func(_ context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
		paymentURL, signed, err := BuildAlipayPaymentURL(callbacks, req)
		if err != nil {
			return PaymentDisplayResult{}, err
		}
		display["type"] = "redirect"
		display["payment_url"] = paymentURL
		display["signed"] = signed
		display["sign_type"] = "RSA2"
		return PaymentDisplayResult{Display: display, PaymentURL: paymentURL}, nil
	}
}

func BuildAlipayPaymentURL(callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, bool, error) {
	instance := req.Instance
	gatewayURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "gatewayUrl"))
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipaydev.com/gateway.do"
	}
	appID := strings.TrimSpace(configString(instance.Config, "app_id", "appId"))
	if appID == "" {
		return "", false, errs.BadRequest("alipay app_id is required")
	}
	amountCNY, amountErr := cashierAmountCNYWithExactFen(req.AmountCNY)
	if amountErr != nil {
		return "", false, amountErr
	}
	notifyURL, returnURL := cashierCallbackURLs(callbacks, instance.Config, "alipay_direct", req.ClientReturnURL)
	bizContent, _ := json.Marshal(map[string]string{
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"total_amount": amountCNY,
		"subject":      defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"product_code": "FAST_INSTANT_TRADE_PAY",
	})
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.page.pay")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().UTC().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("notify_url", notifyURL)
	values.Set("return_url", returnURL)
	values.Set("biz_content", string(bizContent))
	sign, signErr := alipayRSA2Sign(values, configString(instance.Config, "app_private_key", "private_key", "privateKey"))
	if signErr != nil {
		return "", false, signErr
	}
	values.Set("sign", sign)
	return appendQuery(gatewayURL, values), true, nil
}

func cashierCallbackURLs(callbacks CallbackURLConfig, config map[string]any, providerType string, clientReturnURL string) (string, string) {
	notifyURL := strings.TrimSpace(configString(config, "notify_url", "notifyUrl"))
	returnURL := strings.TrimSpace(configString(config, "return_url", "returnUrl"))
	if trimmed := strings.TrimSpace(clientReturnURL); trimmed != "" {
		returnURL = trimmed
	}
	baseURL := strings.TrimRight(strings.TrimSpace(callbacks.SiteBaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if notifyURL == "" {
		notifyURL = baseURL + "/api/open/image/v1/payments/webhooks/" + strings.ToLower(strings.TrimSpace(providerType))
	}
	if returnURL == "" {
		returnURL = baseURL + "/#/checkout"
	}
	return notifyURL, returnURL
}

func alipayRSA2Sign(values url.Values, privateKeyPEM string) (string, error) {
	privateKeyPEM = strings.TrimSpace(privateKeyPEM)
	if privateKeyPEM == "" {
		return "", errs.BadRequest("alipay private key is required")
	}
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", errs.BadRequest("alipay private key is invalid")
	}
	signContent := alipaySignContent(values)
	digest := sha256.Sum256([]byte(signContent))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", errs.Internal("failed to sign alipay payment")
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func alipaySignContent(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "sign" || key == "sign_type" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(values.Get(key))
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "&")
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	if !strings.Contains(raw, "-----BEGIN") {
		raw = "-----BEGIN RSA PRIVATE KEY-----\n" + raw + "\n-----END RSA PRIVATE KEY-----"
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("invalid pem private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not rsa")
	}
	return key, nil
}

func appendQuery(raw string, values url.Values) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if strings.Contains(raw, "?") {
			return raw + "&" + values.Encode()
		}
		return raw + "?" + values.Encode()
	}
	query := parsed.Query()
	for key, items := range values {
		for _, item := range items {
			query.Set(key, item)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
