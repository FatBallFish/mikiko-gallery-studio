package cashier

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func NewWxPayPaymentDisplayBuilder(callbacks CallbackURLConfig) PaymentDisplayBuilder {
	return func(ctx context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
		return BuildWxPayPaymentDisplay(ctx, callbacks, req, display)
	}
}

func BuildWxPayPaymentDisplay(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
	instance := req.Instance
	result := PaymentDisplayResult{
		Display:     display,
		QRCode:      strings.TrimSpace(configString(instance.Config, "qr_code", "qrcode", "code_url")),
		PaymentURL:  strings.TrimSpace(configString(instance.Config, "payment_url", "pay_url", "h5_url")),
		ClientToken: strings.TrimSpace(configString(instance.Config, "client_token")),
	}
	prepayMode := strings.ToLower(strings.TrimSpace(configString(instance.Config, "payment_mode", "prepay_mode", "trade_type")))
	if result.ClientToken == "" && prepayMode == "jsapi" {
		clientToken, err := BuildWxPayJSAPIClientToken(ctx, callbacks, req)
		if err != nil {
			return PaymentDisplayResult{}, err
		}
		result.ClientToken = clientToken
		display["type"] = "jsapi"
		display["prepay_mode"] = "jsapi"
	}
	if result.PaymentURL == "" && prepayMode == "h5" {
		paymentURL, err := BuildWxPayH5PaymentURL(ctx, callbacks, req)
		if err != nil {
			return PaymentDisplayResult{}, err
		}
		result.PaymentURL = paymentURL
		display["prepay_mode"] = "h5"
	}
	if result.QRCode == "" && result.PaymentURL == "" && result.ClientToken == "" {
		codeURL, err := BuildWxPayNativeCodeURL(ctx, callbacks, req)
		if err != nil {
			return PaymentDisplayResult{}, err
		}
		result.QRCode = codeURL
		display["prepay_mode"] = "native"
	}
	if result.QRCode == "" && result.PaymentURL == "" && result.ClientToken == "" {
		return PaymentDisplayResult{}, fmt.Errorf("%w: wxpay_direct", ErrPaymentProviderNotImplemented)
	}
	if result.QRCode != "" {
		display["type"] = "qr_code"
		display["qr_code"] = result.QRCode
	}
	if result.PaymentURL != "" {
		display["payment_url"] = result.PaymentURL
	}
	if result.ClientToken != "" {
		display["client_token"] = result.ClientToken
	}
	return result, nil
}

func BuildWxPayJSAPIClientToken(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, error) {
	instance := req.Instance
	appID, mchID, serial, privateKeyRaw, err := wxPayRequiredMerchantConfig(instance.Config)
	if err != nil {
		return "", err
	}
	openID := strings.TrimSpace(configString(instance.Config, "openid", "open_id", "payer_openid", "payerOpenID"))
	if openID == "" {
		return "", errs.BadRequest("wxpay jsapi openid is required")
	}
	totalFen, err := WxPayAmountFenFromCNY(req.AmountCNY)
	if err != nil {
		return "", err
	}
	notifyURL, _ := cashierCallbackURLs(callbacks, instance.Config, "wxpay_direct", req.ClientReturnURL)
	payload := map[string]any{
		"appid":        appID,
		"mchid":        mchID,
		"description":  defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"amount": map[string]any{
			"total":    totalFen,
			"currency": "CNY",
		},
		"payer": map[string]any{
			"openid": openID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", errs.Internal("failed to build wxpay jsapi request")
	}
	respBody, err := postWxPayJSON(ctx, instance.Config, "/v3/pay/transactions/jsapi", body, mchID, serial, privateKeyRaw)
	if err != nil {
		return "", err
	}
	var parsed struct {
		PrepayID string `json:"prepay_id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || strings.TrimSpace(parsed.PrepayID) == "" {
		return "", paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	return WxPayBuildJSAPIClientToken(appID, strings.TrimSpace(parsed.PrepayID), privateKeyRaw)
}

func BuildWxPayH5PaymentURL(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, error) {
	instance := req.Instance
	appID, mchID, serial, privateKeyRaw, err := wxPayRequiredMerchantConfig(instance.Config)
	if err != nil {
		return "", err
	}
	totalFen, err := WxPayAmountFenFromCNY(req.AmountCNY)
	if err != nil {
		return "", err
	}
	notifyURL, _ := cashierCallbackURLs(callbacks, instance.Config, "wxpay_direct", req.ClientReturnURL)
	payload := map[string]any{
		"appid":        appID,
		"mchid":        mchID,
		"description":  defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"amount": map[string]any{
			"total":    totalFen,
			"currency": "CNY",
		},
		"scene_info": map[string]any{
			"payer_client_ip": defaultString(strings.TrimSpace(configString(instance.Config, "client_ip", "payer_client_ip", "payerClientIP")), "127.0.0.1"),
			"h5_info": map[string]any{
				"type": defaultString(strings.TrimSpace(configString(instance.Config, "h5_type", "h5InfoType")), "Wap"),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", errs.Internal("failed to build wxpay h5 request")
	}
	respBody, err := postWxPayJSON(ctx, instance.Config, "/v3/pay/transactions/h5", body, mchID, serial, privateKeyRaw)
	if err != nil {
		return "", err
	}
	var parsed struct {
		H5URL string `json:"h5_url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || strings.TrimSpace(parsed.H5URL) == "" {
		return "", paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	return strings.TrimSpace(parsed.H5URL), nil
}

func BuildWxPayNativeCodeURL(ctx context.Context, callbacks CallbackURLConfig, req PaymentDisplayRequest) (string, error) {
	instance := req.Instance
	appID, mchID, serial, privateKeyRaw, err := wxPayRequiredMerchantConfig(instance.Config)
	if err != nil {
		return "", err
	}
	totalFen, err := WxPayAmountFenFromCNY(req.AmountCNY)
	if err != nil {
		return "", err
	}
	notifyURL, _ := cashierCallbackURLs(callbacks, instance.Config, "wxpay_direct", req.ClientReturnURL)
	payload := map[string]any{
		"appid":        appID,
		"mchid":        mchID,
		"description":  defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"amount": map[string]any{
			"total":    totalFen,
			"currency": "CNY",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", errs.Internal("failed to build wxpay native request")
	}
	respBody, err := postWxPayJSON(ctx, instance.Config, "/v3/pay/transactions/native", body, mchID, serial, privateKeyRaw)
	if err != nil {
		return "", err
	}
	var parsed struct {
		CodeURL string `json:"code_url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || strings.TrimSpace(parsed.CodeURL) == "" {
		return "", paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	return strings.TrimSpace(parsed.CodeURL), nil
}

func wxPayRequiredMerchantConfig(config map[string]any) (string, string, string, string, error) {
	appID := strings.TrimSpace(configString(config, "app_id", "appId"))
	mchID := strings.TrimSpace(configString(config, "mch_id", "mchId", "merchant_id", "merchantId"))
	serial := strings.TrimSpace(configString(config, "merchant_certificate_serial", "merchantCertificateSerial", "merchant_serial_no", "serial_no"))
	privateKeyRaw := strings.TrimSpace(configString(config, "merchant_private_key", "private_key", "privateKey"))
	if appID == "" || mchID == "" || serial == "" || privateKeyRaw == "" {
		return "", "", "", "", errs.BadRequest("wxpay app_id, mch_id, merchant_certificate_serial, and merchant_private_key are required")
	}
	return appID, mchID, serial, privateKeyRaw, nil
}

func postWxPayJSON(ctx context.Context, config map[string]any, requestURI string, body []byte, mchID string, serial string, privateKeyRaw string) ([]byte, error) {
	gatewayURL := strings.TrimSpace(configString(config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	endpoint := strings.TrimRight(gatewayURL, "/") + requestURI
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errs.BadRequest("invalid wxpay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	auth, err := WxPayBuildAuthorization(http.MethodPost, httpReq.URL.RequestURI(), string(body), mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", auth)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := cashierProviderHTTPClient.Do(httpReq)
	if err != nil {
		return nil, paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	defer resp.Body.Close()
	respBody, err := readCashierProviderResponse(resp.Body)
	if err != nil {
		return nil, paymentInitializationOutcomeUncertain(errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		providerErr := errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
		if resp.StatusCode >= 500 {
			return nil, paymentInitializationOutcomeUncertain(providerErr)
		}
		return nil, providerErr
	}
	return respBody, nil
}

func WxPayBuildAuthorization(method string, requestURI string, body string, mchID string, serial string, privateKeyPEM string, timestamp int64, nonce string) (string, error) {
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", errs.BadRequest("wxpay merchant private key is invalid")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	requestURI = strings.TrimSpace(requestURI)
	nonce = strings.TrimSpace(nonce)
	if method == "" || requestURI == "" || strings.TrimSpace(mchID) == "" || strings.TrimSpace(serial) == "" || nonce == "" {
		return "", errs.BadRequest("wxpay authorization fields are required")
	}
	message := fmt.Sprintf("%s\n%s\n%d\n%s\n%s\n", method, requestURI, timestamp, nonce, body)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", errs.Internal("failed to sign wxpay request")
	}
	return fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%d",serial_no="%s"`,
		strings.TrimSpace(mchID),
		nonce,
		base64.StdEncoding.EncodeToString(signature),
		timestamp,
		strings.TrimSpace(serial),
	), nil
}

func WxPayBuildJSAPIClientToken(appID string, prepayID string, privateKeyPEM string) (string, error) {
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", errs.BadRequest("wxpay merchant private key is invalid")
	}
	appID = strings.TrimSpace(appID)
	prepayID = strings.TrimSpace(prepayID)
	if appID == "" || prepayID == "" {
		return "", errs.BadRequest("wxpay jsapi app_id and prepay_id are required")
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := uuid.NewString()
	packageValue := "prepay_id=" + prepayID
	message := fmt.Sprintf("%s\n%s\n%s\n%s\n", appID, timestamp, nonce, packageValue)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", errs.Internal("failed to sign wxpay jsapi token")
	}
	token := map[string]string{
		"appId":     appID,
		"timeStamp": timestamp,
		"nonceStr":  nonce,
		"package":   packageValue,
		"signType":  "RSA",
		"paySign":   base64.StdEncoding.EncodeToString(signature),
	}
	body, err := json.Marshal(token)
	if err != nil {
		return "", errs.Internal("failed to build wxpay jsapi token")
	}
	return string(body), nil
}

func WxPayAmountFenFromCNY(amountCNY string) (int64, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountCNY))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return 0, errs.BadRequest("amount_cny is invalid")
	}
	scaled := amount.Mul(decimal.NewFromInt(100))
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, errs.BadRequest("amount_cny must not contain fractional fen")
	}
	return scaled.IntPart(), nil
}
