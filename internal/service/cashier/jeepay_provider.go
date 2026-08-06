package cashier

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

var jeepayHTTPClient = &http.Client{Timeout: 15 * time.Second}

func NewJeePayPaymentDisplayBuilder(callbacks CallbackURLConfig) PaymentDisplayBuilder {
	return func(ctx context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
		return BuildJeePayPaymentDisplay(ctx, callbacks, req, display)
	}
}

func BuildJeePayPaymentDisplay(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
	payment, err := buildJeePayAPIPayment(ctx, callbacks, req)
	if err != nil {
		return PaymentDisplayResult{}, err
	}
	result := PaymentDisplayResult{Display: display, PaymentURL: payment.PaymentURL, QRCode: payment.QRCode}
	display["type"] = "redirect"
	if payment.FormHTML != "" {
		display["type"] = "form"
		display["form_html"] = payment.FormHTML
	} else if payment.QRCode != "" {
		display["type"] = "qr_code"
		display["qr_code"] = payment.QRCode
	}
	if payment.PaymentURL != "" {
		display["payment_url"] = payment.PaymentURL
	}
	display["prepay_mode"] = "api"
	display["way_code"] = payment.WayCode
	if payment.ChannelTradeNo != "" {
		display["channel_trade_no"] = payment.ChannelTradeNo
	}
	return result, nil
}

func BuildJeePayAPIPayment(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, string, string, string, string, error) {
	payment, err := buildJeePayAPIPayment(ctx, callbacks, req)
	return payment.PaymentURL, payment.QRCode, payment.Sign, payment.WayCode, payment.ChannelTradeNo, err
}

type jeepayAPIPayment struct {
	PaymentURL     string
	QRCode         string
	FormHTML       string
	Sign           string
	WayCode        string
	ChannelTradeNo string
}

type jeepayUnifiedOrderRequest struct {
	MchNo        string `json:"mchNo"`
	AppID        string `json:"appId"`
	WayCode      string `json:"wayCode"`
	MchOrderNo   string `json:"mchOrderNo"`
	Amount       int64  `json:"amount"`
	Currency     string `json:"currency"`
	Subject      string `json:"subject"`
	Body         string `json:"body"`
	NotifyURL    string `json:"notifyUrl,omitempty"`
	ReturnURL    string `json:"returnUrl,omitempty"`
	ClientIP     string `json:"clientIp,omitempty"`
	ReqTime      int64  `json:"reqTime"`
	SignType     string `json:"signType"`
	Version      string `json:"version"`
	ChannelExtra string `json:"channelExtra,omitempty"`
	Sign         string `json:"sign"`
}

type jeepayUnifiedOrderResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data jeepayUnifiedOrderData `json:"data"`
}

type jeepayUnifiedOrderData struct {
	PayOrderID  string `json:"payOrderId"`
	PayDataType string `json:"payDataType"`
	PayData     string `json:"payData"`
	PayURL      string `json:"payUrl"`
	CodeURL     string `json:"codeUrl"`
	QRCode      string `json:"qrCode"`
}

func buildJeePayAPIPayment(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest) (jeepayAPIPayment, error) {
	baseURL, params, sign, wayCode, err := BuildJeePayPaymentParams(callbacks, req)
	if err != nil {
		return jeepayAPIPayment{}, err
	}
	amount, amountErr := strconv.ParseInt(params["amount"], 10, 64)
	reqTime, reqTimeErr := strconv.ParseInt(params["reqTime"], 10, 64)
	if amountErr != nil || reqTimeErr != nil || amount <= 0 || reqTime <= 0 {
		return jeepayAPIPayment{}, errs.BadRequest("invalid jeepay amount or request time")
	}
	payload := jeepayUnifiedOrderRequest{
		MchNo: params["mchNo"], AppID: params["appId"], WayCode: params["wayCode"], MchOrderNo: params["mchOrderNo"],
		Amount: amount, Currency: params["currency"], Subject: params["subject"], Body: params["body"],
		NotifyURL: params["notifyUrl"], ReturnURL: params["returnUrl"], ClientIP: params["clientIp"],
		ReqTime: reqTime, SignType: params["signType"], Version: params["version"], ChannelExtra: params["channelExtra"], Sign: sign,
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return jeepayAPIPayment{}, errs.Internal("encode jeepay unified order request")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/pay/unifiedOrder"
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if reqErr != nil {
		return jeepayAPIPayment{}, errs.BadRequest("invalid jeepay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := jeepayHTTPClient.Do(httpReq)
	if doErr != nil {
		logJeePayFailure(ctx, req, "request", 0, "", "request failed")
		return jeepayAPIPayment{}, paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	defer resp.Body.Close()
	respBody, readErr := readCashierProviderResponse(resp.Body)
	if readErr != nil {
		logJeePayFailure(ctx, req, "read_response", resp.StatusCode, "", "response could not be read")
		return jeepayAPIPayment{}, paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, message := jeepayResponseDiagnostic(respBody)
		logJeePayFailure(ctx, req, "http_response", resp.StatusCode, code, message)
		providerErr := errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
		if resp.StatusCode >= 500 {
			return jeepayAPIPayment{}, paymentInitializationOutcomeUncertain(providerErr)
		}
		return jeepayAPIPayment{}, providerErr
	}
	var parsed jeepayUnifiedOrderResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		logJeePayFailure(ctx, req, "decode_response", resp.StatusCode, "", "invalid JSON response")
		return jeepayAPIPayment{}, paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	code := strconv.Itoa(parsed.Code)
	if parsed.Code != 0 {
		logJeePayFailure(ctx, req, "provider_response", resp.StatusCode, code, parsed.Msg)
		return jeepayAPIPayment{}, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	result := jeepayAPIPayment{
		PaymentURL: strings.TrimSpace(parsed.Data.PayURL),
		QRCode:     strings.TrimSpace(defaultString(parsed.Data.CodeURL, parsed.Data.QRCode)),
		Sign:       sign, WayCode: wayCode, ChannelTradeNo: strings.TrimSpace(parsed.Data.PayOrderID),
	}
	payData := strings.TrimSpace(parsed.Data.PayData)
	switch strings.ToLower(strings.TrimSpace(parsed.Data.PayDataType)) {
	case "payurl":
		result.PaymentURL = payData
	case "form":
		result.FormHTML = payData
	case "codeurl", "codeimgurl", "code_img_url":
		result.QRCode = payData
	case "none":
		// No browser-actionable payment data is available for checkout.
	case "":
		if result.PaymentURL == "" && result.QRCode == "" && payData != "" {
			if parsedURL, parseErr := url.Parse(payData); parseErr == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") && parsedURL.Host != "" {
				result.PaymentURL = payData
			} else {
				result.QRCode = payData
			}
		}
	}
	if result.PaymentURL == "" && result.QRCode == "" && result.FormHTML == "" {
		logJeePayFailure(ctx, req, "payment_payload", resp.StatusCode, code, parsed.Msg)
		return jeepayAPIPayment{}, paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	return result, nil
}

func jeepayResponseDiagnostic(body []byte) (string, string) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "upstream returned a non-JSON response"
	}
	return strings.TrimSpace(rawString(raw["code"])), strings.TrimSpace(firstRawString(raw, "msg", "message"))
}

func logJeePayFailure(ctx context.Context, req PaymentDisplayRequest, stage string, status int, code, message string) {
	slog.WarnContext(ctx, "jeepay unified order failed",
		"stage", stage,
		"provider_type", strings.TrimSpace(req.Instance.ProviderType),
		"http_status", status,
		"provider_code", strings.TrimSpace(code),
		"message", sanitizeJeePayDiagnostic(message, req.Instance.Config),
	)
}

func sanitizeJeePayDiagnostic(message string, config map[string]any) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	for _, key := range []string{"key", "api_key", "apiKey", "merchant_key", "merchantKey"} {
		secret := strings.TrimSpace(configString(config, key))
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	parts := strings.Fields(message)
	for index, part := range parts {
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
			if parsed, err := url.Parse(part); err == nil && parsed.Host != "" {
				parsed.RawQuery = ""
				parsed.Fragment = ""
				parts[index] = parsed.String()
			}
		case strings.Contains(lower, "sign="), strings.Contains(lower, "signature="), strings.Contains(lower, "token="), strings.Contains(lower, "key="), strings.Contains(lower, "secret="):
			parts[index] = "[redacted]"
		}
	}
	message = strings.Join(parts, " ")
	runes := []rune(message)
	if len(runes) > 160 {
		message = string(runes[:160])
	}
	if message == "" {
		return "upstream request failed"
	}
	return message
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
	amountFen, amountErr := jeepayAmountFenFromCNYExact(req.AmountCNY)
	if amountErr != nil {
		return "", nil, "", "", amountErr
	}
	params := map[string]string{
		"mchNo":      mchNo,
		"appId":      appID,
		"wayCode":    wayCode,
		"mchOrderNo": strings.TrimSpace(req.OrderNo),
		"amount":     amountFen,
		"currency":   "cny",
		"subject":    defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"body":       defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"notifyUrl":  notifyURL,
		"returnUrl":  returnURL,
		"clientIp":   defaultString(strings.TrimSpace(configString(req.Instance.Config, "client_ip", "clientIp", "payer_client_ip", "payerClientIP")), "127.0.0.1"),
		"reqTime":    strconv.FormatInt(time.Now().UnixMilli(), 10),
		"signType":   "MD5",
		"version":    "1.0",
	}
	channelExtra, channelExtraErr := jsonOrStringConfig(req.Instance.Config, "channel_extra", "channelExtra", "channel_extra_json", "channelExtraJSON")
	if channelExtraErr != nil {
		return "", nil, "", "", channelExtraErr
	}
	if channelExtra != "" {
		params["channelExtra"] = channelExtra
	} else if defaultChannelExtra := defaultJeePayChannelExtra(wayCode); defaultChannelExtra != "" {
		params["channelExtra"] = defaultChannelExtra
	}
	sign := jeepaySign(params, key)
	params["sign"] = sign
	return baseURL, params, sign, wayCode, nil
}

func defaultJeePayChannelExtra(wayCode string) string {
	normalized := strings.ToUpper(strings.TrimSpace(wayCode))
	switch {
	case strings.Contains(normalized, "NATIVE"), strings.Contains(normalized, "QR"):
		return `{"payDataType":"codeUrl"}`
	case strings.Contains(normalized, "PC"), strings.Contains(normalized, "WEB"), strings.Contains(normalized, "WAP"), strings.Contains(normalized, "H5"):
		return `{"payDataType":"payUrl"}`
	default:
		return ""
	}
}

func trimJeePayEndpointBase(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		path := strings.TrimRight(parsed.Path, "/")
		lower := strings.ToLower(path)
		for _, endpoint := range []string{"/api/pay/unifiedorder", "/api/pay/query", "/api/pay/close", "/api/refund/refundorder", "/api/pay/notify", "/api/pay"} {
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
		if strings.EqualFold(name, "sign") || strings.TrimSpace(value) == "" {
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
	amountFen, err := jeepayAmountFenFromCNYExact(amountCNY)
	if err != nil {
		return "0"
	}
	return amountFen
}

func jeepayAmountFenFromCNYExact(amountCNY string) (string, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountCNY))
	if err != nil || !amount.IsPositive() {
		return "", errs.BadRequest("amount_cny is invalid")
	}
	scaled := amount.Mul(decimal.NewFromInt(100))
	if !scaled.Equal(scaled.Truncate(0)) {
		return "", errs.BadRequest("amount_cny must not contain fractional fen")
	}
	return strconv.FormatInt(scaled.IntPart(), 10), nil
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
