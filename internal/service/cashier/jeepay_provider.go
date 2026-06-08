package cashier

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func NewJeePayPaymentDisplayBuilder(callbacks CallbackURLConfig) PaymentDisplayBuilder {
	return func(ctx context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
		return BuildJeePayPaymentDisplay(ctx, callbacks, req, display)
	}
}

func BuildJeePayPaymentDisplay(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
	prepayMode := strings.ToLower(strings.TrimSpace(configString(req.Instance.Config, "payment_mode", "prepay_mode", "trade_type")))
	if prepayMode == "api" || prepayMode == "qrcode" || prepayMode == "qr_code" {
		paymentURL, qrCode, sign, wayCode, channelTradeNo, err := BuildJeePayAPIPayment(ctx, callbacks, req)
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
		display["way_code"] = wayCode
		if channelTradeNo != "" {
			display["channel_trade_no"] = channelTradeNo
		}
		return result, nil
	}
	paymentURL, sign, wayCode, err := BuildJeePayPaymentURL(callbacks, req)
	if err != nil {
		return PaymentDisplayResult{}, err
	}
	display["type"] = "redirect"
	display["payment_url"] = paymentURL
	display["sign"] = sign
	display["sign_type"] = "MD5"
	display["way_code"] = wayCode
	return PaymentDisplayResult{Display: display, PaymentURL: paymentURL}, nil
}

func BuildJeePayPaymentURL(callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, string, string, error) {
	baseURL, params, sign, wayCode, err := BuildJeePayPaymentParams(callbacks, req)
	if err != nil {
		return "", "", "", err
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return strings.TrimRight(baseURL, "/") + "/api/pay/unifiedOrder?" + values.Encode(), sign, wayCode, nil
}

func BuildJeePayAPIPayment(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, string, string, string, string, error) {
	baseURL, params, sign, wayCode, err := BuildJeePayPaymentParams(callbacks, req)
	if err != nil {
		return "", "", "", "", "", err
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/pay/unifiedOrder"
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if reqErr != nil {
		return "", "", "", "", "", errs.BadRequest("invalid jeepay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	var raw map[string]any
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	code := strings.TrimSpace(rawString(raw["code"]))
	if code != "0" && code != "1" && !strings.EqualFold(code, "success") {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	data := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		data = nested
	}
	paymentURL := strings.TrimSpace(firstRawString(data, "payUrl", "pay_url", "payurl", "payURL", "cashierUrl", "cashier_url"))
	qrCode := strings.TrimSpace(firstRawString(data, "codeUrl", "code_url", "qrCode", "qr_code", "qrcode", "payData", "pay_data"))
	channelTradeNo := strings.TrimSpace(firstRawString(data, "payOrderId", "pay_order_id", "trade_no", "tradeNo", "channelOrderNo"))
	if paymentURL == "" && qrCode == "" {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return paymentURL, qrCode, sign, wayCode, channelTradeNo, nil
}

func BuildJeePayPaymentParams(callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, map[string]string, string, string, error) {
	baseURL := strings.TrimSpace(configString(req.Instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return "", nil, "", "", errs.BadRequest("jeepay gateway_url is required")
	}
	baseURL = trimJeePayEndpointBase(baseURL)
	mchNo := strings.TrimSpace(configString(req.Instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
	appID := strings.TrimSpace(configString(req.Instance.Config, "app_id", "appId"))
	key := strings.TrimSpace(configString(req.Instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if mchNo == "" || appID == "" || key == "" {
		return "", nil, "", "", errs.BadRequest("jeepay mch_no, app_id, and key are required")
	}
	providerType := strings.ToLower(strings.TrimSpace(req.Instance.ProviderType))
	wayCode := strings.TrimSpace(configString(req.Instance.Config, "way_code", "wayCode"))
	if wayCode == "" {
		if providerType == "jeepay_wxpay" {
			wayCode = "WX_NATIVE"
		} else {
			wayCode = "ALI_PC"
		}
	}
	notifyURL, returnURL := cashierCallbackURLs(callbacks, req.Instance.Config, providerType, req.ClientReturnURL)
	params := map[string]string{
		"mchNo":      mchNo,
		"appId":      appID,
		"wayCode":    wayCode,
		"mchOrderNo": strings.TrimSpace(req.OrderNo),
		"amount":     jeepayAmountFenFromCNY(req.AmountCNY),
		"currency":   "cny",
		"subject":    defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"body":       defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"notifyUrl":  notifyURL,
		"returnUrl":  returnURL,
		"clientIp":   defaultString(strings.TrimSpace(configString(req.Instance.Config, "client_ip", "clientIp", "payer_client_ip", "payerClientIP")), "127.0.0.1"),
		"signType":   "MD5",
	}
	channelExtra, channelExtraErr := jsonOrStringConfig(req.Instance.Config, "channel_extra", "channelExtra", "channel_extra_json", "channelExtraJSON")
	if channelExtraErr != nil {
		return "", nil, "", "", channelExtraErr
	}
	if channelExtra != "" {
		params["channelExtra"] = channelExtra
	}
	sign := jeepaySign(params, key)
	params["sign"] = sign
	return baseURL, params, sign, wayCode, nil
}

func trimJeePayEndpointBase(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		path := strings.TrimRight(parsed.Path, "/")
		lower := strings.ToLower(path)
		for _, endpoint := range []string{"/api/pay/unifiedorder", "/api/pay/notify", "/api/pay"} {
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

func jeepaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if strings.EqualFold(name, "sign") || strings.EqualFold(name, "signType") || strings.TrimSpace(value) == "" {
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
	_, _ = builder.WriteString("&key=" + strings.TrimSpace(key))
	sum := md5.Sum([]byte(builder.String()))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func jeepayAmountFenFromCNY(amountCNY string) string {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountCNY))
	if err != nil {
		return "0"
	}
	return strconv.FormatInt(amount.Mul(decimal.NewFromInt(100)).Round(0).IntPart(), 10)
}

func jsonOrStringConfig(values map[string]any, keys ...string) (string, error) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		if value, ok := raw.(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value, nil
			}
			continue
		}
		body, err := json.Marshal(raw)
		if err != nil {
			return "", errs.BadRequest(key + " must be a JSON string or object")
		}
		value := strings.TrimSpace(string(body))
		if value != "" && value != "null" {
			return value, nil
		}
	}
	return "", nil
}

func firstRawString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(rawString(raw[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func rawString(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
