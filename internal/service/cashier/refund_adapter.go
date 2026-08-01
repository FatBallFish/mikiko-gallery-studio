package cashier

import (
	"context"
	"strings"
	"sync"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

type RefundPaymentRequest struct {
	Order           OrderSnapshot
	Instance        domaincashier.ProviderInstance
	RefundTradeNo   string
	RefundAmountCNY string
	Reason          string
}

type RefundPaymentResult struct {
	ProviderType       string         `json:"provider_type"`
	ProviderInstanceID int64          `json:"provider_instance_id,omitempty"`
	RefundStatus       string         `json:"refund_status"`
	RefundTradeNo      string         `json:"refund_trade_no"`
	ChannelRefundNo    string         `json:"channel_refund_no,omitempty"`
	Message            string         `json:"message,omitempty"`
	Raw                map[string]any `json:"raw,omitempty"`
	RefundedAt         time.Time      `json:"refunded_at"`
}

type RefundPaymentBuilder func(ctx context.Context, req RefundPaymentRequest) (RefundPaymentResult, error)

type RefundProviderBuilders struct {
	AlipayDirect RefundPaymentBuilder
	WxPayDirect  RefundPaymentBuilder
	EasyPay      RefundPaymentBuilder
	JeePay       RefundPaymentBuilder
	Stripe       RefundPaymentBuilder
}

type RefundAdapterRegistry struct {
	mu       sync.RWMutex
	builders map[string]RefundPaymentBuilder
}

func NewRefundAdapterRegistry() *RefundAdapterRegistry {
	return &RefundAdapterRegistry{builders: map[string]RefundPaymentBuilder{}}
}

func NewRefundAdapterRegistryWithBuilders(builders RefundProviderBuilders) *RefundAdapterRegistry {
	registry := NewRefundAdapterRegistry()
	registry.Register("alipay_direct", builders.AlipayDirect)
	registry.Register("wxpay_direct", builders.WxPayDirect)
	registry.Register("easypay_alipay", builders.EasyPay)
	registry.Register("easypay_wxpay", builders.EasyPay)
	registry.Register("jeepay_alipay", builders.JeePay)
	registry.Register("jeepay_wxpay", builders.JeePay)
	registry.Register("stripe", builders.Stripe)
	return registry
}

func (r *RefundAdapterRegistry) Register(providerType string, builder RefundPaymentBuilder) {
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

func (r *RefundAdapterRegistry) RefundPayment(ctx context.Context, req RefundPaymentRequest) (RefundPaymentResult, bool, error) {
	if !RefundRequiresProvider(req.Order, req.Instance) {
		return RefundPaymentResult{}, false, nil
	}
	providerType := strings.ToLower(strings.TrimSpace(req.Instance.ProviderType))
	if r != nil {
		r.mu.RLock()
		builder := r.builders[providerType]
		r.mu.RUnlock()
		if builder != nil {
			result, err := builder(ctx, req)
			return result, true, err
		}
	}
	return RefundPaymentResult{}, false, nil
}

func RefundRequiresProvider(order OrderSnapshot, instance domaincashier.ProviderInstance) bool {
	status := strings.ToLower(strings.TrimSpace(order.Status))
	if status == "refunded" || (status != "completed" && status != "partially_refunded") {
		return false
	}
	providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
	return providerType != "" && providerType != "mock" && !strings.HasPrefix(providerType, "manual")
}
