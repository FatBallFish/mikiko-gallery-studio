package cashier

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func StandardRefundProviderBuilders() RefundProviderBuilders {
	return RefundProviderBuilders{
		AlipayDirect: AlipayRefundPaymentBuilder,
		WxPayDirect:  WxPayRefundPaymentBuilder,
		EasyPay:      EasyPayRefundPaymentBuilder,
		JeePay:       JeePayRefundPaymentBuilder,
		Stripe:       NewStripeRefundPaymentBuilder(),
	}
}

func AlipayRefundPaymentBuilder(ctx context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
	order := req.Order
	instance := req.Instance
	gatewayURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "gatewayUrl"))
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipaydev.com/gateway.do"
	}
	appID := strings.TrimSpace(configString(instance.Config, "app_id", "appId"))
	if appID == "" {
		return RefundPaymentResult{}, paymentProviderUnavailable()
	}
	refundAmountCNY, amountErr := cashierAmountCNYWithExactFen(defaultString(strings.TrimSpace(req.RefundAmountCNY), strings.TrimSpace(order.AmountCNY)))
	if amountErr != nil {
		return RefundPaymentResult{}, amountErr
	}
	bizContent, _ := json.Marshal(map[string]string{
		"out_trade_no":   strings.TrimSpace(order.OrderNo),
		"refund_amount":  refundAmountCNY,
		"refund_reason":  defaultString(strings.TrimSpace(req.Reason), "cashier order refund"),
		"out_request_no": strings.TrimSpace(req.RefundTradeNo),
	})
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.refund")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().UTC().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("biz_content", string(bizContent))
	sign, signErr := alipayRSA2Sign(values, configString(instance.Config, "app_private_key", "private_key", "privateKey"))
	if signErr != nil {
		return RefundPaymentResult{}, signErr
	}
	values.Set("sign", sign)
	body, appErr := getJSONForCashierProvider(ctx, appendQuery(gatewayURL, values), nil)
	if appErr != nil {
		return RefundPaymentResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	data := raw
	if nested, ok := raw["alipay_trade_refund_response"].(map[string]any); ok {
		data = nested
	}
	code := strings.TrimSpace(firstRawString(data, "code"))
	if code != "10000" {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	if subCode := strings.TrimSpace(firstRawString(data, "sub_code")); subCode != "" {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(data, "fund_change", "status")))
	if status == "" {
		status = "accepted"
	}
	raw["source"] = "alipay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return RefundPaymentResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(req.RefundTradeNo),
		ChannelRefundNo:    strings.TrimSpace(firstRawString(data, "trade_no", "out_trade_no")),
		Message:            strings.TrimSpace(firstRawString(data, "msg", "sub_msg")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func WxPayRefundPaymentBuilder(ctx context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
	order := req.Order
	instance := req.Instance
	_, mchID, serial, privateKeyRaw, err := wxPayRequiredMerchantConfig(instance.Config)
	if err != nil {
		return RefundPaymentResult{}, paymentProviderUnavailable()
	}
	totalFen, amountErr := WxPayAmountFenFromCNY(order.AmountCNY)
	if amountErr != nil {
		return RefundPaymentResult{}, amountErr
	}
	refundFen, refundAmountErr := WxPayAmountFenFromCNY(defaultString(strings.TrimSpace(req.RefundAmountCNY), strings.TrimSpace(order.AmountCNY)))
	if refundAmountErr != nil {
		return RefundPaymentResult{}, refundAmountErr
	}
	payload := map[string]any{
		"out_trade_no":  strings.TrimSpace(order.OrderNo),
		"out_refund_no": strings.TrimSpace(req.RefundTradeNo),
		"reason":        defaultString(strings.TrimSpace(req.Reason), "cashier order refund"),
		"amount": map[string]any{
			"refund":   refundFen,
			"total":    totalFen,
			"currency": "CNY",
		},
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return RefundPaymentResult{}, errs.Internal("failed to build wxpay refund request")
	}
	gatewayURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	requestURI := "/v3/refund/domestic/refunds"
	auth, signErr := WxPayBuildAuthorization(http.MethodPost, requestURI, string(body), mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return RefundPaymentResult{}, signErr
	}
	respBody, appErr := postJSONForCashierProvider(ctx, strings.TrimRight(gatewayURL, "/")+requestURI, body, map[string]string{"Authorization": auth})
	if appErr != nil {
		return RefundPaymentResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(raw, "status", "refund_status")))
	if status == "abnormal" || status == "closed" {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	if status == "" {
		status = "accepted"
	}
	raw["source"] = "wxpay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return RefundPaymentResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(req.RefundTradeNo),
		ChannelRefundNo:    strings.TrimSpace(firstRawString(raw, "refund_id", "channel_refund_no")),
		Message:            strings.TrimSpace(firstRawString(raw, "message")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func EasyPayRefundPaymentBuilder(ctx context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
	order := req.Order
	instance := req.Instance
	baseURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return RefundPaymentResult{}, paymentProviderUnavailable()
	}
	baseURL = trimEasyPayEndpointBase(baseURL)
	pid := strings.TrimSpace(configString(instance.Config, "pid", "merchant_id", "merchantId"))
	key := strings.TrimSpace(configString(instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if pid == "" || key == "" {
		return RefundPaymentResult{}, paymentProviderUnavailable()
	}
	refundAmountCNY, amountErr := cashierAmountCNYWithExactFen(defaultString(strings.TrimSpace(req.RefundAmountCNY), strings.TrimSpace(order.AmountCNY)))
	if amountErr != nil {
		return RefundPaymentResult{}, amountErr
	}
	endpoint := strings.TrimSpace(configString(instance.Config, "refund_url", "refundUrl"))
	if endpoint == "" {
		endpoint = strings.TrimRight(baseURL, "/") + "/api.php"
	}
	values := url.Values{}
	values.Set("act", "refund")
	values.Set("pid", pid)
	values.Set("key", key)
	values.Set("money", refundAmountCNY)
	values.Set("out_trade_no", strings.TrimSpace(order.OrderNo))
	raw, appErr := postEasyPayRefundForm(ctx, endpoint, values)
	if appErr != nil && strings.TrimSpace(order.TradeNo) != "" && easyPayRefundShouldRetryByTradeNo(raw) {
		values.Del("out_trade_no")
		values.Set("trade_no", strings.TrimSpace(order.TradeNo))
		raw, appErr = postEasyPayRefundForm(ctx, endpoint, values)
	}
	if appErr != nil {
		return RefundPaymentResult{}, appErr
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(raw, "status", "trade_status")))
	if status == "" {
		status = "accepted"
	}
	raw["source"] = "easypay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	raw["refund_trade_no"] = strings.TrimSpace(req.RefundTradeNo)
	return RefundPaymentResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(req.RefundTradeNo),
		ChannelRefundNo:    strings.TrimSpace(firstRawString(raw, "refund_no", "trade_no", "api_trade_no")),
		Message:            strings.TrimSpace(firstRawString(raw, "msg", "message")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func JeePayRefundPaymentBuilder(ctx context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
	order := req.Order
	instance := req.Instance
	baseURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return RefundPaymentResult{}, paymentProviderUnavailable()
	}
	baseURL = trimJeePayEndpointBase(baseURL)
	mchNo := strings.TrimSpace(configString(instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
	appID := strings.TrimSpace(configString(instance.Config, "app_id", "appId"))
	key := strings.TrimSpace(configString(instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if mchNo == "" || appID == "" || key == "" {
		return RefundPaymentResult{}, paymentProviderUnavailable()
	}
	refundAmount, amountErr := jeepayAmountFenFromCNYExact(defaultString(strings.TrimSpace(req.RefundAmountCNY), strings.TrimSpace(order.AmountCNY)))
	if amountErr != nil {
		return RefundPaymentResult{}, amountErr
	}
	endpoint := strings.TrimSpace(configString(instance.Config, "refund_url", "refundUrl"))
	if endpoint == "" {
		refundPath := strings.TrimSpace(configString(instance.Config, "refund_path", "refundPath"))
		if refundPath == "" {
			refundPath = "/api/refund/refundOrder"
		}
		if !strings.HasPrefix(refundPath, "/") {
			refundPath = "/" + refundPath
		}
		endpoint = strings.TrimRight(baseURL, "/") + refundPath
	}
	reqTime := time.Now().UnixMilli()
	params := map[string]string{
		"mchNo":        mchNo,
		"appId":        appID,
		"mchOrderNo":   strings.TrimSpace(order.OrderNo),
		"mchRefundNo":  strings.TrimSpace(req.RefundTradeNo),
		"refundAmount": refundAmount,
		"currency":     "cny",
		"refundReason": defaultString(strings.TrimSpace(req.Reason), "cashier order refund"),
		"reqTime":      strconv.FormatInt(reqTime, 10),
		"version":      "1.0",
		"signType":     "MD5",
	}
	if tradeNo := strings.TrimSpace(order.TradeNo); tradeNo != "" {
		params["payOrderId"] = tradeNo
	}
	if clientIP := strings.TrimSpace(configString(instance.Config, "client_ip", "clientIp", "payer_client_ip", "payerClientIP")); clientIP != "" {
		params["clientIp"] = clientIP
	}
	if notifyURL := strings.TrimSpace(configString(instance.Config, "refund_notify_url", "refundNotifyUrl")); notifyURL != "" {
		params["notifyUrl"] = notifyURL
	}
	params["sign"] = jeepaySign(params, key)
	refundAmountFen, parseAmountErr := strconv.ParseInt(refundAmount, 10, 64)
	if parseAmountErr != nil {
		return RefundPaymentResult{}, errs.BadRequest("invalid jeepay refund amount")
	}
	payload := make(map[string]any, len(params))
	for name, value := range params {
		payload[name] = value
	}
	payload["reqTime"] = reqTime
	payload["refundAmount"] = refundAmountFen
	requestBody, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return RefundPaymentResult{}, errs.Internal("encode jeepay refund request")
	}
	body, appErr := postJSONForCashierProvider(ctx, endpoint, requestBody, nil)
	if appErr != nil {
		return RefundPaymentResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	code := strings.TrimSpace(firstRawString(raw, "code"))
	if code != "0" {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	data := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		data = nested
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(data, "state", "status", "refundState", "refund_status")))
	if status == "3" || status == "failed" || status == "fail" || status == "closed" {
		return RefundPaymentResult{}, paymentRefundProviderUnavailable()
	}
	if status == "" {
		status = "accepted"
	}
	raw["source"] = "jeepay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return RefundPaymentResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(req.RefundTradeNo),
		ChannelRefundNo:    strings.TrimSpace(firstRawString(data, "refundOrderId", "channelOrderNo", "refund_no", "refundNo")),
		Message:            strings.TrimSpace(firstRawString(raw, "msg", "message")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func postEasyPayRefundForm(ctx context.Context, endpoint string, values url.Values) (map[string]any, error) {
	body, appErr := postFormForCashierProvider(ctx, endpoint, values)
	if appErr != nil {
		return nil, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, paymentRefundProviderUnavailable()
	}
	code := strings.TrimSpace(firstRawString(raw, "code"))
	if code != "1" {
		return raw, paymentRefundProviderUnavailable()
	}
	return raw, nil
}

func easyPayRefundShouldRetryByTradeNo(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(firstRawString(raw, "msg", "message", "error")))
	return strings.Contains(message, "not found") ||
		strings.Contains(message, "no order") ||
		strings.Contains(message, "不存在") ||
		strings.Contains(message, "未找到")
}

func postJSONForCashierProvider(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	respBody, _, err := postJSONForCashierProviderWithStatus(ctx, endpoint, body, headers)
	return respBody, err
}

func postJSONForCashierProviderWithStatus(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, int, error) {
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if reqErr != nil {
		return nil, 0, paymentProviderUnavailable()
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	resp, doErr := cashierProviderHTTPClient.Do(httpReq)
	if doErr != nil {
		return nil, 0, paymentProviderUnavailable()
	}
	defer resp.Body.Close()
	respBody, readErr := readCashierProviderResponse(resp.Body)
	if readErr != nil {
		return nil, resp.StatusCode, paymentProviderUnavailable()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, paymentProviderUnavailable()
	}
	return respBody, resp.StatusCode, nil
}

func paymentRefundProviderUnavailable() error {
	return errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
}
