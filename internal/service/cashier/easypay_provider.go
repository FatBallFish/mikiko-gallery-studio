package cashier

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func NewEasyPayPaymentDisplayBuilder(callbacks CallbackURLConfig) PaymentDisplayBuilder {
	return func(ctx context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
		return BuildEasyPayPaymentDisplay(ctx, callbacks, req, display)
	}
}

func BuildEasyPayPaymentDisplay(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
	providerType := strings.ToLower(strings.TrimSpace(req.Instance.ProviderType))
	paymentType := "alipay"
	if providerType == "easypay_wxpay" {
		paymentType = "wxpay"
	}
	prepayMode := strings.ToLower(strings.TrimSpace(configString(req.Instance.Config, "payment_mode", "prepay_mode", "trade_type")))
	if prepayMode == "api" || prepayMode == "qrcode" || prepayMode == "qr_code" {
		paymentURL, qrCode, sign, err := BuildEasyPayAPIPayment(ctx, callbacks, req, paymentType)
		if err != nil {
			return PaymentDisplayResult{}, err
		}
		result := PaymentDisplayResult{Display: display, PaymentURL: paymentURL, QRCode: qrCode}
		display["type"] = "redirect"
		if qrCode != "" {
			display["type"] = "qr_code"
			display["qr_code"] = qrCode
		}
		if paymentURL != "" {
			display["payment_url"] = paymentURL
		}
		display["prepay_mode"] = "api"
		display["sign"] = sign
		display["sign_type"] = "MD5"
		return result, nil
	}
	paymentURL, sign, err := BuildEasyPayPaymentURL(callbacks, req, paymentType)
	if err != nil {
		return PaymentDisplayResult{}, err
	}
	display["type"] = "redirect"
	display["payment_url"] = paymentURL
	display["sign"] = sign
	display["sign_type"] = "MD5"
	return PaymentDisplayResult{Display: display, PaymentURL: paymentURL}, nil
}

func BuildEasyPayPaymentURL(callbacks CallbackURLConfig, req PaymentDisplayRequest, paymentType string) (string, string, error) {
	baseURL, params, sign, _, err := BuildEasyPayPaymentParams(callbacks, req, paymentType)
	if err != nil {
		return "", "", err
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return strings.TrimRight(baseURL, "/") + "/submit.php?" + values.Encode(), sign, nil
}

func BuildEasyPayAPIPayment(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest, paymentType string) (string, string, string, error) {
	baseURL, params, sign, key, err := BuildEasyPayPaymentParams(callbacks, req, paymentType)
	if err != nil {
		return "", "", "", err
	}
	params["clientip"] = defaultString(strings.TrimSpace(configString(req.Instance.Config, "client_ip", "clientip", "payer_client_ip", "payerClientIP")), "127.0.0.1")
	if device := strings.TrimSpace(configString(req.Instance.Config, "device")); device != "" {
		params["device"] = device
	}
	sign = easyPaySign(params, key)
	params["sign"] = sign
	params["sign_type"] = "MD5"
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/mapi.php"
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if reqErr != nil {
		return "", "", "", errs.BadRequest("invalid easypay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	var parsed struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		TradeNo string `json:"trade_no"`
		PayURL  string `json:"payurl"`
		PayURL2 string `json:"payurl2"`
		QRCode  string `json:"qrcode"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.Code != 1 {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	paymentURL := strings.TrimSpace(parsed.PayURL)
	if paymentURL == "" {
		paymentURL = strings.TrimSpace(parsed.PayURL2)
	}
	qrCode := strings.TrimSpace(parsed.QRCode)
	if paymentURL == "" && qrCode == "" {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return paymentURL, qrCode, sign, nil
}

func BuildEasyPayPaymentParams(callbacks CallbackURLConfig, req PaymentDisplayRequest, paymentType string) (string, map[string]string, string, string, error) {
	baseURL := strings.TrimSpace(configString(req.Instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return "", nil, "", "", errs.BadRequest("easypay gateway_url is required")
	}
	baseURL = trimEasyPayEndpointBase(baseURL)
	pid := strings.TrimSpace(configString(req.Instance.Config, "pid", "merchant_id", "merchantId"))
	key := strings.TrimSpace(configString(req.Instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if pid == "" || key == "" {
		return "", nil, "", "", errs.BadRequest("easypay pid and key are required")
	}
	notifyURL, returnURL := cashierCallbackURLs(callbacks, req.Instance.Config, strings.ToLower(strings.TrimSpace(req.Instance.ProviderType)), req.ClientReturnURL)
	params := map[string]string{
		"pid":          pid,
		"type":         paymentType,
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"money":        strings.TrimSpace(req.AmountCNY),
	}
	if cid := strings.TrimSpace(configString(req.Instance.Config, "cid")); cid != "" {
		params["cid"] = cid
	}
	sign := easyPaySign(params, key)
	params["sign"] = sign
	params["sign_type"] = "MD5"
	return baseURL, params, sign, key, nil
}

func trimEasyPayEndpointBase(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		path := strings.TrimRight(parsed.Path, "/")
		lower := strings.ToLower(path)
		for _, endpoint := range []string{"/submit.php", "/mapi.php", "/api.php"} {
			if strings.HasSuffix(lower, endpoint) {
				path = strings.TrimRight(path[:len(path)-len(endpoint)], "/")
				break
			}
		}
		parsed.Path = path
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func easyPaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if name == "sign" || name == "sign_type" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, name := range keys {
		if index > 0 {
			_ = builder.WriteByte('&')
		}
		_, _ = builder.WriteString(name + "=" + params[name])
	}
	_, _ = builder.WriteString(strings.TrimSpace(key))
	sum := md5.Sum([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}
