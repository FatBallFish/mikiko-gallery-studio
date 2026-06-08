package cashier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func StandardQueryProviderBuilders() QueryProviderBuilders {
	return QueryProviderBuilders{
		AlipayDirect: AlipayOrderStatusQueryBuilder,
		WxPayDirect:  WxPayOrderStatusQueryBuilder,
		EasyPay:      EasyPayOrderStatusQueryBuilder,
		JeePay:       JeePayOrderStatusQueryBuilder,
	}
}

func AlipayOrderStatusQueryBuilder(ctx context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
	order := req.Order
	instance := req.Instance
	gatewayURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "gatewayUrl"))
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipaydev.com/gateway.do"
	}
	appID := strings.TrimSpace(configString(instance.Config, "app_id", "appId"))
	if appID == "" {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	bizContent, _ := json.Marshal(map[string]string{
		"out_trade_no": strings.TrimSpace(order.OrderNo),
	})
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.query")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().UTC().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("biz_content", string(bizContent))
	sign, signErr := alipayRSA2Sign(values, configString(instance.Config, "app_private_key", "private_key", "privateKey"))
	if signErr != nil {
		return QueryOrderStatusResult{}, signErr
	}
	values.Set("sign", sign)
	body, appErr := getJSONForCashierProvider(ctx, appendQuery(gatewayURL, values), nil)
	if appErr != nil {
		return QueryOrderStatusResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	data := raw
	if nested, ok := raw["alipay_trade_query_response"].(map[string]any); ok {
		data = nested
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(data, "trade_status", "status")))
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(firstRawString(data, "trade_no"))
	amountCNY := strings.TrimSpace(firstRawString(data, "total_amount", "receipt_amount", "buyer_pay_amount"))
	raw["source"] = "alipay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return BuildQueryOrderStatusResult(instance, NormalizeQueryStatus(status), tradeNo, amountCNY, raw), nil
}

func WxPayOrderStatusQueryBuilder(ctx context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
	order := req.Order
	instance := req.Instance
	_, mchID, serial, privateKeyRaw, err := wxPayRequiredMerchantConfig(instance.Config)
	if err != nil {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	gatewayURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(strings.TrimSpace(order.OrderNo))
	values := url.Values{}
	values.Set("mchid", mchID)
	requestURI := path + "?" + values.Encode()
	endpoint := strings.TrimRight(gatewayURL, "/") + requestURI
	auth, signErr := WxPayBuildAuthorization(http.MethodGet, requestURI, "", mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return QueryOrderStatusResult{}, signErr
	}
	body, appErr := getJSONForCashierProvider(ctx, endpoint, map[string]string{"Authorization": auth})
	if appErr != nil {
		return QueryOrderStatusResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(raw, "trade_state", "status")))
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(firstRawString(raw, "transaction_id", "trade_no"))
	amountCNY := ""
	if amountRaw, ok := raw["amount"].(map[string]any); ok {
		totalFen := firstRawString(amountRaw, "total", "payer_total")
		if totalFen != "" {
			if amountFen, err := strconv.ParseInt(strings.TrimSpace(totalFen), 10, 64); err == nil {
				amountCNY = wxPayAmountCNYFromFen(amountFen)
			}
		}
	}
	raw["source"] = "wxpay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return BuildQueryOrderStatusResult(instance, NormalizeQueryStatus(status), tradeNo, amountCNY, raw), nil
}

func EasyPayOrderStatusQueryBuilder(ctx context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
	order := req.Order
	instance := req.Instance
	baseURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	baseURL = trimEasyPayEndpointBase(baseURL)
	pid := strings.TrimSpace(configString(instance.Config, "pid", "merchant_id", "merchantId"))
	key := strings.TrimSpace(configString(instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if pid == "" || key == "" {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	endpoint := strings.TrimSpace(configString(instance.Config, "query_url", "queryUrl"))
	if endpoint == "" {
		queryPath := strings.TrimSpace(configString(instance.Config, "query_path", "queryPath"))
		if queryPath == "" {
			queryPath = "/api.php"
		}
		if !strings.HasPrefix(queryPath, "/") {
			queryPath = "/" + queryPath
		}
		endpoint = strings.TrimRight(baseURL, "/") + queryPath
	}
	values := url.Values{}
	values.Set("act", "order")
	values.Set("pid", pid)
	values.Set("key", key)
	values.Set("out_trade_no", strings.TrimSpace(order.OrderNo))
	body, appErr := postFormForCashierProvider(ctx, endpoint, values)
	if appErr != nil {
		return QueryOrderStatusResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	status := strings.ToLower(strings.TrimSpace(rawString(raw["status"])))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(rawString(raw["trade_status"])))
	}
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(rawString(raw["trade_no"]))
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(rawString(raw["api_trade_no"]))
	}
	amountCNY := strings.TrimSpace(rawString(raw["money"]))
	if amountCNY == "" {
		amountCNY = strings.TrimSpace(rawString(raw["amount_cny"]))
	}
	raw["source"] = "easypay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return BuildQueryOrderStatusResult(instance, NormalizeQueryStatus(status), tradeNo, amountCNY, raw), nil
}

func JeePayOrderStatusQueryBuilder(ctx context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
	order := req.Order
	instance := req.Instance
	baseURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	baseURL = trimJeePayEndpointBase(baseURL)
	mchNo := strings.TrimSpace(configString(instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
	appID := strings.TrimSpace(configString(instance.Config, "app_id", "appId"))
	key := strings.TrimSpace(configString(instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if mchNo == "" || appID == "" || key == "" {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	endpoint := strings.TrimSpace(configString(instance.Config, "query_url", "queryUrl"))
	if endpoint == "" {
		queryPath := strings.TrimSpace(configString(instance.Config, "query_path", "queryPath"))
		if queryPath == "" {
			queryPath = "/api/pay/query"
		}
		if !strings.HasPrefix(queryPath, "/") {
			queryPath = "/" + queryPath
		}
		endpoint = strings.TrimRight(baseURL, "/") + queryPath
	}
	params := map[string]string{
		"mchNo":      mchNo,
		"appId":      appID,
		"mchOrderNo": strings.TrimSpace(order.OrderNo),
		"signType":   "MD5",
	}
	params["sign"] = jeepaySign(params, key)
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	body, appErr := postFormForCashierProvider(ctx, endpoint, values)
	if appErr != nil {
		return QueryOrderStatusResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return QueryOrderStatusResult{}, paymentProviderUnavailable()
	}
	data := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		data = nested
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(data, "state", "status", "trade_state", "tradeStatus")))
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(firstRawString(data, "payOrderId", "channelOrderNo", "trade_no", "tradeNo"))
	amountCNY := strings.TrimSpace(firstRawString(data, "amount_cny", "amountCNY", "money", "total_amount", "totalAmount"))
	if amountCNY == "" {
		amountCNY = JeePayAmountCNYFromFen(firstRawString(data, "amount"))
	}
	raw["source"] = "jeepay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return BuildQueryOrderStatusResult(instance, NormalizeQueryStatus(status), tradeNo, amountCNY, raw), nil
}

func postFormForCashierProvider(ctx context.Context, endpoint string, values url.Values) ([]byte, error) {
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if reqErr != nil {
		return nil, paymentProviderUnavailable()
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return nil, paymentProviderUnavailable()
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, paymentProviderUnavailable()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, paymentProviderUnavailable()
	}
	return body, nil
}

func getJSONForCashierProvider(ctx context.Context, endpoint string, headers map[string]string) ([]byte, error) {
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if reqErr != nil {
		return nil, paymentProviderUnavailable()
	}
	httpReq.Header.Set("Accept", "application/json")
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return nil, paymentProviderUnavailable()
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, paymentProviderUnavailable()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, paymentProviderUnavailable()
	}
	return body, nil
}

func paymentProviderUnavailable() error {
	return errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
}

func wxPayAmountCNYFromFen(amountFen int64) string {
	return decimal.NewFromInt(amountFen).Div(decimal.NewFromInt(100)).Round(5).StringFixed(5)
}

func JeePayAmountCNYFromFen(amountFen string) string {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountFen))
	if err != nil {
		return ""
	}
	return amount.Div(decimal.NewFromInt(100)).Round(5).StringFixed(5)
}
