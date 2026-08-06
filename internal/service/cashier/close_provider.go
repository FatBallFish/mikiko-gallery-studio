package cashier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/paymentintent"
)

func StandardCloseProviderBuilders() CloseProviderBuilders {
	return CloseProviderBuilders{
		AlipayDirect: AlipayClosePaymentBuilder,
		WxPayDirect:  WxPayClosePaymentBuilder,
		EasyPay:      EasyPayClosePaymentBuilder,
		JeePay:       JeePayClosePaymentBuilder,
		Stripe:       NewStripeClosePaymentBuilder(),
	}
}

func JeePayClosePaymentBuilder(ctx context.Context, req ClosePaymentRequest) (ClosePaymentResult, error) {
	instance := req.Instance
	baseURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	mchNo := strings.TrimSpace(configString(instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
	appID := strings.TrimSpace(configString(instance.Config, "app_id", "appId"))
	key := strings.TrimSpace(configString(instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if baseURL == "" || mchNo == "" || appID == "" || key == "" || strings.TrimSpace(req.Order.OrderNo) == "" {
		return BuildClosePaymentResult(instance, "invalid_config", false, false, nil), paymentProviderUnavailable()
	}
	endpoint := strings.TrimSpace(configString(instance.Config, "close_url", "closeUrl"))
	if endpoint == "" {
		closePath := strings.TrimSpace(configString(instance.Config, "close_path", "closePath"))
		if closePath == "" {
			closePath = "/api/pay/close"
		}
		if !strings.HasPrefix(closePath, "/") {
			closePath = "/" + closePath
		}
		endpoint = strings.TrimRight(trimJeePayEndpointBase(baseURL), "/") + closePath
	}
	reqTime := time.Now().UnixMilli()
	params := map[string]string{
		"mchNo": mchNo, "appId": appID, "mchOrderNo": strings.TrimSpace(req.Order.OrderNo),
		"reqTime": strconv.FormatInt(reqTime, 10), "version": "1.0", "signType": "MD5",
	}
	params["sign"] = jeepaySign(params, key)
	payload := map[string]any{
		"mchNo": mchNo, "appId": appID, "mchOrderNo": params["mchOrderNo"],
		"reqTime": reqTime, "version": "1.0", "signType": "MD5", "sign": params["sign"],
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return BuildClosePaymentResult(instance, "encode_failed", false, false, nil), paymentProviderUnavailable()
	}
	responseBody, appErr := postJSONForCashierProvider(ctx, endpoint, body, nil)
	if appErr != nil {
		return BuildClosePaymentResult(instance, "request_failed", false, false, closeDiagnostic("jeepay_close_api", req.Order.OrderNo, "request_failed")), appErr
	}
	var response map[string]any
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return BuildClosePaymentResult(instance, "invalid_response", false, false, closeDiagnostic("jeepay_close_api", req.Order.OrderNo, "invalid_response")), paymentProviderUnavailable()
	}
	data := response
	identitySources := []map[string]any{response}
	if nested, ok := response["data"].(map[string]any); ok {
		data = nested
		identitySources = append(identitySources, nested)
	}
	providerStatus := strings.ToLower(strings.TrimSpace(firstRawString(data, "state", "orderState", "status")))
	raw := closeDiagnostic("jeepay_close_api", req.Order.OrderNo, providerStatus)
	if code := strings.TrimSpace(firstRawString(response, "code")); code != "0" {
		return BuildClosePaymentResult(instance, providerStatus, false, false, raw), paymentProviderUnavailable()
	}
	if !queryResponseIdentityMatches(req.Order.OrderNo, identitySources, false, "mchOrderNo", "mch_order_no") ||
		!queryResponseIdentityMatches(mchNo, identitySources, false, "mchNo", "mch_no") ||
		!queryResponseIdentityMatches(appID, identitySources, false, "appId", "app_id") {
		return BuildClosePaymentResult(instance, providerStatus, false, false, raw), paymentProviderUnavailable()
	}
	if providerStatus == "2" || providerStatus == "paid" || providerStatus == "success" {
		return BuildClosePaymentResult(instance, providerStatus, false, true, raw), nil
	}
	if providerStatus == "4" || providerStatus == "closed" || providerStatus == "canceled" || providerStatus == "cancelled" {
		return BuildClosePaymentResult(instance, providerStatus, true, false, raw), nil
	}
	return BuildClosePaymentResult(instance, providerStatus, false, false, raw), paymentProviderUnavailable()
}

func AlipayClosePaymentBuilder(ctx context.Context, req ClosePaymentRequest) (ClosePaymentResult, error) {
	instance := req.Instance
	appID := strings.TrimSpace(configString(instance.Config, "app_id", "appId"))
	if appID == "" || strings.TrimSpace(req.Order.OrderNo) == "" {
		return BuildClosePaymentResult(instance, "invalid_config", false, false, nil), paymentProviderUnavailable()
	}
	bizContent, _ := json.Marshal(map[string]string{"out_trade_no": strings.TrimSpace(req.Order.OrderNo)})
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.close")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().UTC().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("biz_content", string(bizContent))
	sign, signErr := alipayRSA2Sign(values, configString(instance.Config, "app_private_key", "private_key", "privateKey"))
	if signErr != nil {
		return BuildClosePaymentResult(instance, "invalid_config", false, false, nil), signErr
	}
	values.Set("sign", sign)
	gatewayURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "gatewayUrl"))
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipaydev.com/gateway.do"
	}
	body, appErr := getJSONForCashierProvider(ctx, appendQuery(gatewayURL, values), nil)
	if appErr != nil {
		return BuildClosePaymentResult(instance, "request_failed", false, false, closeDiagnostic("alipay_close_api", req.Order.OrderNo, "request_failed")), appErr
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return BuildClosePaymentResult(instance, "invalid_response", false, false, closeDiagnostic("alipay_close_api", req.Order.OrderNo, "invalid_response")), paymentProviderUnavailable()
	}
	data := response
	if nested, ok := response["alipay_trade_close_response"].(map[string]any); ok {
		data = nested
	}
	code := strings.TrimSpace(firstRawString(data, "code"))
	subCode := strings.TrimSpace(firstRawString(data, "sub_code"))
	raw := closeDiagnostic("alipay_close_api", req.Order.OrderNo, defaultString(subCode, code))
	if code == "10000" && subCode == "" {
		if !queryResponseIdentityMatches(req.Order.OrderNo, []map[string]any{data}, true, "out_trade_no") {
			return BuildClosePaymentResult(instance, "identity_mismatch", false, false, raw), paymentProviderUnavailable()
		}
		return BuildClosePaymentResult(instance, "closed", true, false, raw), nil
	}
	upperSubCode := strings.ToUpper(subCode)
	if strings.Contains(upperSubCode, "TRADE_HAS_SUCCESS") || strings.Contains(upperSubCode, "TRADE_FINISHED") {
		return BuildClosePaymentResult(instance, "paid", false, true, raw), nil
	}
	return BuildClosePaymentResult(instance, defaultString(subCode, code), false, false, raw), paymentProviderUnavailable()
}

func WxPayClosePaymentBuilder(ctx context.Context, req ClosePaymentRequest) (ClosePaymentResult, error) {
	instance := req.Instance
	_, mchID, serial, privateKeyRaw, configErr := wxPayRequiredMerchantConfig(instance.Config)
	if configErr != nil || strings.TrimSpace(req.Order.OrderNo) == "" {
		return BuildClosePaymentResult(instance, "invalid_config", false, false, nil), paymentProviderUnavailable()
	}
	body, _ := json.Marshal(map[string]string{"mchid": mchID})
	requestURI := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(strings.TrimSpace(req.Order.OrderNo)) + "/close"
	auth, signErr := WxPayBuildAuthorization(http.MethodPost, requestURI, string(body), mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return BuildClosePaymentResult(instance, "invalid_config", false, false, nil), signErr
	}
	gatewayURL := strings.TrimSpace(configString(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	_, statusCode, appErr := postJSONForCashierProviderWithStatus(ctx, strings.TrimRight(gatewayURL, "/")+requestURI, body, map[string]string{"Authorization": auth})
	if appErr != nil {
		return BuildClosePaymentResult(instance, "request_failed", false, false, closeDiagnostic("wxpay_close_api", req.Order.OrderNo, "request_failed")), appErr
	}
	if statusCode != http.StatusNoContent {
		return BuildClosePaymentResult(instance, "unexpected_response", false, false, closeDiagnostic("wxpay_close_api", req.Order.OrderNo, "unexpected_response")), paymentProviderUnavailable()
	}
	return BuildClosePaymentResult(instance, "closed", true, false, closeDiagnostic("wxpay_close_api", req.Order.OrderNo, "closed")), nil
}

func EasyPayClosePaymentBuilder(ctx context.Context, req ClosePaymentRequest) (ClosePaymentResult, error) {
	instance := req.Instance
	endpoint := strings.TrimSpace(configString(instance.Config, "close_url", "closeUrl"))
	if endpoint == "" {
		return UnsupportedClosePaymentResult(instance), fmt.Errorf("%w: %s", ErrPaymentCloseUnsupported, instance.ProviderType)
	}
	pid := strings.TrimSpace(configString(instance.Config, "pid", "merchant_id", "merchantId"))
	key := strings.TrimSpace(configString(instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if pid == "" || key == "" || strings.TrimSpace(req.Order.OrderNo) == "" {
		return BuildClosePaymentResult(instance, "invalid_config", false, false, nil), paymentProviderUnavailable()
	}
	values := url.Values{}
	values.Set("act", defaultString(strings.TrimSpace(configString(instance.Config, "close_action", "closeAction")), "close"))
	values.Set("pid", pid)
	values.Set("key", key)
	values.Set("out_trade_no", strings.TrimSpace(req.Order.OrderNo))
	body, appErr := postFormForCashierProvider(ctx, endpoint, values)
	if appErr != nil {
		return BuildClosePaymentResult(instance, "request_failed", false, false, closeDiagnostic("easypay_close_api", req.Order.OrderNo, "request_failed")), appErr
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return BuildClosePaymentResult(instance, "invalid_response", false, false, closeDiagnostic("easypay_close_api", req.Order.OrderNo, "invalid_response")), paymentProviderUnavailable()
	}
	status := strings.ToLower(strings.TrimSpace(firstRawString(response, "status", "trade_status", "state")))
	raw := closeDiagnostic("easypay_close_api", req.Order.OrderNo, status)
	if code := strings.TrimSpace(firstRawString(response, "code")); code != "1" && !strings.EqualFold(code, "success") {
		return BuildClosePaymentResult(instance, status, false, false, raw), paymentProviderUnavailable()
	}
	if !queryResponseIdentityMatches(req.Order.OrderNo, []map[string]any{response}, true, "out_trade_no", "mch_order_no") ||
		!queryResponseIdentityMatches(pid, []map[string]any{response}, true, "pid", "merchant_id") {
		return BuildClosePaymentResult(instance, status, false, false, raw), paymentProviderUnavailable()
	}
	if status == "paid" || status == "success" || status == "trade_success" {
		return BuildClosePaymentResult(instance, status, false, true, raw), nil
	}
	if status == "closed" || status == "canceled" || status == "cancelled" || status == "trade_closed" {
		return BuildClosePaymentResult(instance, status, true, false, raw), nil
	}
	return BuildClosePaymentResult(instance, status, false, false, raw), paymentProviderUnavailable()
}

type StripePaymentIntentCloser interface {
	Cancel(string, *stripe.PaymentIntentCancelParams) (*stripe.PaymentIntent, error)
}

type stripePaymentIntentCloserFactory func(secretKey string) StripePaymentIntentCloser

func NewStripeClosePaymentBuilder() ClosePaymentBuilder {
	return newStripeClosePaymentBuilder(func(secretKey string) StripePaymentIntentCloser {
		return &paymentintent.Client{B: stripe.GetBackend(stripe.APIBackend), Key: secretKey}
	})
}

func newStripeClosePaymentBuilder(clientFactory stripePaymentIntentCloserFactory) ClosePaymentBuilder {
	return func(ctx context.Context, req ClosePaymentRequest) (ClosePaymentResult, error) {
		instance := req.Instance
		secretKey := strings.TrimSpace(configString(instance.Config, "secret_key"))
		intentID := strings.TrimSpace(req.Order.ClientToken)
		if secretKey == "" || intentID == "" {
			return BuildClosePaymentResult(instance, "invalid_config", false, false, nil), paymentProviderUnavailable()
		}
		params := &stripe.PaymentIntentCancelParams{CancellationReason: stripe.String(string(stripe.PaymentIntentCancellationReasonRequestedByCustomer))}
		params.Context = ctx
		intent, err := clientFactory(secretKey).Cancel(intentID, params)
		if err != nil || intent == nil {
			return BuildClosePaymentResult(instance, "request_failed", false, false, closeDiagnostic("stripe_payment_intent", req.Order.OrderNo, "request_failed")), paymentProviderUnavailable()
		}
		status := strings.ToLower(strings.TrimSpace(string(intent.Status)))
		raw := closeDiagnostic("stripe_payment_intent", req.Order.OrderNo, status)
		raw["payment_intent_id"] = strings.TrimSpace(intent.ID)
		if strings.TrimSpace(intent.ID) != intentID || strings.TrimSpace(intent.Metadata["order_no"]) != strings.TrimSpace(req.Order.OrderNo) {
			return BuildClosePaymentResult(instance, "identity_mismatch", false, false, raw), paymentProviderUnavailable()
		}
		if intent.Status == stripe.PaymentIntentStatusSucceeded {
			return BuildClosePaymentResult(instance, status, false, true, raw), nil
		}
		if intent.Status == stripe.PaymentIntentStatusCanceled {
			return BuildClosePaymentResult(instance, status, true, false, raw), nil
		}
		return BuildClosePaymentResult(instance, status, false, false, raw), paymentProviderUnavailable()
	}
}

func closeDiagnostic(source, orderNo, status string) map[string]any {
	return map[string]any{
		"source":   source,
		"order_no": strings.TrimSpace(orderNo),
		"status":   strings.ToLower(strings.TrimSpace(status)),
	}
}
