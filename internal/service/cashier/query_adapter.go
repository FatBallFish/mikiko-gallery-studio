package cashier

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

type OrderSnapshot struct {
	OrderNo         string
	AmountCNY       string
	TradeNo         string
	RefundTradeNo   string
	ChannelRefundNo string
	ClientToken     string
	Status          string
}

type QueryStatus struct {
	Status       string
	RiskCategory string
	ActionHint   string
	Paid         bool
	Message      string
}

type QueryOrderStatusRequest struct {
	Order    OrderSnapshot
	Instance domaincashier.ProviderInstance
}

type QueryOrderStatusResult struct {
	ProviderType       string         `json:"provider_type"`
	ProviderInstanceID int64          `json:"provider_instance_id,omitempty"`
	QueryStatus        string         `json:"query_status"`
	RiskCategory       string         `json:"risk_category,omitempty"`
	ActionHint         string         `json:"action_hint,omitempty"`
	Paid               bool           `json:"paid"`
	Completed          bool           `json:"completed"`
	TradeNo            string         `json:"trade_no,omitempty"`
	AmountCNY          string         `json:"amount_cny,omitempty"`
	Message            string         `json:"message,omitempty"`
	Raw                map[string]any `json:"raw,omitempty"`
	SyncedAt           time.Time      `json:"synced_at"`
}

type QueryOrderStatusBuilder func(ctx context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error)

type QueryProviderBuilders struct {
	AlipayDirect QueryOrderStatusBuilder
	WxPayDirect  QueryOrderStatusBuilder
	EasyPay      QueryOrderStatusBuilder
	JeePay       QueryOrderStatusBuilder
	Stripe       QueryOrderStatusBuilder
}

type QueryAdapterRegistry struct {
	mu       sync.RWMutex
	builders map[string]QueryOrderStatusBuilder
}

func NewQueryAdapterRegistry() *QueryAdapterRegistry {
	return &QueryAdapterRegistry{builders: map[string]QueryOrderStatusBuilder{}}
}

func NewQueryAdapterRegistryWithBuilders(builders QueryProviderBuilders) *QueryAdapterRegistry {
	registry := NewQueryAdapterRegistry()
	registry.Register("alipay_direct", builders.AlipayDirect)
	registry.Register("wxpay_direct", builders.WxPayDirect)
	registry.Register("easypay_alipay", builders.EasyPay)
	registry.Register("easypay_wxpay", builders.EasyPay)
	registry.Register("jeepay_alipay", builders.JeePay)
	registry.Register("jeepay_wxpay", builders.JeePay)
	registry.Register("stripe", builders.Stripe)
	return registry
}

func (r *QueryAdapterRegistry) Register(providerType string, builder QueryOrderStatusBuilder) {
	if r == nil || builder == nil {
		return
	}
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	if providerType == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builders[providerType] = builder
}

func (r *QueryAdapterRegistry) QueryOrderStatus(ctx context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
	providerType := strings.ToLower(strings.TrimSpace(req.Instance.ProviderType))
	if hasConfigQueryStatus(req.Instance) || providerType == "mock" {
		return ConfigDrivenQueryOrderStatus(req), nil
	}
	if r != nil {
		r.mu.RLock()
		builder := r.builders[providerType]
		r.mu.RUnlock()
		if builder != nil {
			return builder(ctx, req)
		}
	}
	return ConfigDrivenQueryOrderStatus(req), nil
}

func ConfigDrivenQueryOrderStatus(req QueryOrderStatusRequest) QueryOrderStatusResult {
	providerType := strings.ToLower(strings.TrimSpace(req.Instance.ProviderType))
	status := strings.ToLower(strings.TrimSpace(configString(req.Instance.Config, "query_status", "sync_status", "payment_status", "trade_status")))
	if status == "" && strings.EqualFold(strings.TrimSpace(req.Order.Status), "completed") {
		status = "paid"
	}
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(configString(req.Instance.Config, "query_trade_no", "sync_trade_no", "trade_no", "pay_order_id", "transaction_id"))
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(req.Order.TradeNo)
	}
	amountCNY := strings.TrimSpace(configString(req.Instance.Config, "query_amount_cny", "sync_amount_cny", "amount_cny", "money", "total_amount"))
	if amountCNY == "" {
		amountCNY = strings.TrimSpace(req.Order.AmountCNY)
	}
	raw := map[string]any{
		"source":        "provider_instance_config",
		"provider_type": providerType,
		"order_no":      strings.TrimSpace(req.Order.OrderNo),
		"status":        status,
	}
	if tradeNo != "" {
		raw["trade_no"] = tradeNo
	}
	if amountCNY != "" {
		raw["amount_cny"] = amountCNY
	}
	return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus(status), tradeNo, amountCNY, raw)
}

func BuildQueryOrderStatusResult(instance domaincashier.ProviderInstance, queryStatus QueryStatus, tradeNo, amountCNY string, raw map[string]any) QueryOrderStatusResult {
	return QueryOrderStatusResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		QueryStatus:        queryStatus.Status,
		RiskCategory:       queryStatus.RiskCategory,
		ActionHint:         queryStatus.ActionHint,
		Paid:               queryStatus.Paid,
		TradeNo:            strings.TrimSpace(tradeNo),
		AmountCNY:          strings.TrimSpace(amountCNY),
		Message:            queryStatus.Message,
		Raw:                raw,
		SyncedAt:           time.Now().UTC(),
	}
}

func NormalizeQueryStatus(status string) QueryStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "success", "succeeded", "completed", "complete", "trade_success", "trade_finished", "1", "2":
		return QueryStatus{Status: "paid", RiskCategory: "paid", ActionHint: "渠道已确认支付，可核对本地到账状态。", Paid: true, Message: "渠道订单已支付"}
	case "pending", "processing", "process", "wait", "waiting", "created", "new", "0", "wait_buyer_pay", "userpaying", "notpay":
		return QueryStatus{Status: "pending", RiskCategory: "pending", ActionHint: "渠道仍未确认支付，稍后可再次查单。", Message: "渠道订单未支付或仍在处理中"}
	case "closed", "close", "canceled", "cancelled", "cancel", "expired", "trade_closed", "revoked", "3":
		return QueryStatus{Status: "closed", RiskCategory: "closed", ActionHint: "渠道订单已关闭，建议取消当前订单并让用户重新创建订单。", Message: "渠道订单已关闭"}
	case "limited", "limit", "quota_limited", "amount_limited", "frequency_limited", "rate_limited", "exceed_limit", "over_limit":
		return QueryStatus{Status: "failed", RiskCategory: "channel_limited", ActionHint: "渠道订单触发限额限制，建议切换备用渠道、降低单笔金额或调整渠道实例限额后再重试。", Message: "渠道订单触发限额限制"}
	case "sign_error", "signature_error", "invalid_sign", "verify_failed", "signature_invalid", "bad_signature", "sign_invalid":
		return QueryStatus{Status: "failed", RiskCategory: "signature_error", ActionHint: "渠道验签或签名配置异常，请检查商户密钥、证书、公钥、回调地址和签名算法配置。", Message: "渠道验签或签名配置异常"}
	case "amount_mismatch", "money_mismatch", "total_amount_mismatch", "fee_mismatch", "price_mismatch":
		return QueryStatus{Status: "failed", RiskCategory: "amount_mismatch", ActionHint: "渠道订单金额与本地订单不一致，请暂停到账并核对订单金额、汇率、渠道费率和回调原文。", Message: "渠道订单金额与本地订单不一致"}
	case "merchant_disabled", "mch_disabled", "account_disabled", "merchant_abnormal", "account_abnormal", "merchant_closed", "account_closed":
		return QueryStatus{Status: "failed", RiskCategory: "account_abnormal", ActionHint: "渠道商户账号状态异常，建议切换备用账号并登录渠道后台确认商户状态和产品权限。", Message: "渠道商户账号状态异常"}
	case "timeout", "timed_out", "query_timeout", "network_timeout", "gateway_timeout", "request_timeout":
		return QueryStatus{Status: "failed", RiskCategory: "channel_timeout", ActionHint: "渠道查单超时或网络异常，建议稍后重试；连续失败时检查网关地址、网络出口和渠道可用性。", Message: "渠道查单超时或网络异常"}
	case "failed", "failure", "fail", "error", "payerror", "pay_error", "trade_failed", "4":
		return QueryStatus{Status: "failed", RiskCategory: "channel_error", ActionHint: "渠道返回异常状态，请结合原始响应、商户后台和回调事件继续排查。", Message: "渠道订单支付失败"}
	case "risk", "risk_control", "fraud", "intercepted", "security", "blocked":
		return QueryStatus{Status: "failed", RiskCategory: "risk_control", ActionHint: "渠道侧风控或安全策略拦截，建议让用户更换支付渠道或重新创建订单后再支付。", Message: "渠道订单被风控拦截"}
	case "refunded", "refund", "partially_refunded", "partial_refund", "trade_refund":
		return QueryStatus{Status: "refunded", RiskCategory: "refunded", ActionHint: "渠道显示已退款，请核对本地退款流水和用户充值余额是否一致。", Message: "渠道订单已退款"}
	default:
		return QueryStatus{Status: "pending", RiskCategory: "pending", ActionHint: "渠道仍未确认支付，稍后可再次查单。", Message: "渠道订单未支付或仍在处理中"}
	}
}

func hasConfigQueryStatus(instance domaincashier.ProviderInstance) bool {
	return strings.TrimSpace(configString(instance.Config, "query_status", "sync_status", "payment_status", "trade_status")) != ""
}

func configString(config map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := config[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
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
	return ""
}
