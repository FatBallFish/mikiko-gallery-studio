package cashier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

var ErrPaymentCloseUnsupported = errors.New("payment provider does not support safe order close")

type ClosePaymentRequest struct {
	Order    OrderSnapshot
	Instance domaincashier.ProviderInstance
}

type ClosePaymentResult struct {
	ProviderType       string         `json:"provider_type"`
	ProviderInstanceID int64          `json:"provider_instance_id,omitempty"`
	CloseStatus        string         `json:"close_status"`
	ProviderStatus     string         `json:"provider_status,omitempty"`
	Closed             bool           `json:"closed"`
	AlreadyPaid        bool           `json:"already_paid"`
	Unsupported        bool           `json:"unsupported"`
	OutcomeUncertain   bool           `json:"outcome_uncertain"`
	Message            string         `json:"message,omitempty"`
	Raw                map[string]any `json:"raw,omitempty"`
	ClosedAt           time.Time      `json:"closed_at"`
}

type ClosePaymentBuilder func(ctx context.Context, req ClosePaymentRequest) (ClosePaymentResult, error)

type CloseProviderBuilders struct {
	AlipayDirect ClosePaymentBuilder
	WxPayDirect  ClosePaymentBuilder
	EasyPay      ClosePaymentBuilder
	JeePay       ClosePaymentBuilder
	Stripe       ClosePaymentBuilder
}

type CloseAdapterRegistry struct {
	mu       sync.RWMutex
	builders map[string]ClosePaymentBuilder
}

func NewCloseAdapterRegistry() *CloseAdapterRegistry {
	return &CloseAdapterRegistry{builders: map[string]ClosePaymentBuilder{}}
}

func NewCloseAdapterRegistryWithBuilders(builders CloseProviderBuilders) *CloseAdapterRegistry {
	registry := NewCloseAdapterRegistry()
	registry.Register("alipay_direct", builders.AlipayDirect)
	registry.Register("wxpay_direct", builders.WxPayDirect)
	registry.Register("easypay_alipay", builders.EasyPay)
	registry.Register("easypay_wxpay", builders.EasyPay)
	registry.Register("jeepay_alipay", builders.JeePay)
	registry.Register("jeepay_wxpay", builders.JeePay)
	registry.Register("stripe", builders.Stripe)
	return registry
}

func (r *CloseAdapterRegistry) Register(providerType string, builder ClosePaymentBuilder) {
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

func (r *CloseAdapterRegistry) ClosePayment(ctx context.Context, req ClosePaymentRequest) (ClosePaymentResult, error) {
	providerType := strings.ToLower(strings.TrimSpace(req.Instance.ProviderType))
	if r != nil {
		r.mu.RLock()
		builder := r.builders[providerType]
		r.mu.RUnlock()
		if builder != nil {
			return builder(ctx, req)
		}
	}
	return UnsupportedClosePaymentResult(req.Instance), fmt.Errorf("%w: %s", ErrPaymentCloseUnsupported, providerType)
}

func BuildClosePaymentResult(instance domaincashier.ProviderInstance, providerStatus string, closed, alreadyPaid bool, raw map[string]any) ClosePaymentResult {
	status := "uncertain"
	message := "payment provider close outcome is uncertain"
	if closed {
		status = "closed"
		message = "payment provider order is closed"
	} else if alreadyPaid {
		status = "paid"
		message = "payment provider order is already paid"
	}
	return ClosePaymentResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		CloseStatus:        status,
		ProviderStatus:     strings.ToLower(strings.TrimSpace(providerStatus)),
		Closed:             closed,
		AlreadyPaid:        alreadyPaid,
		OutcomeUncertain:   !closed && !alreadyPaid,
		Message:            message,
		Raw:                raw,
		ClosedAt:           time.Now().UTC(),
	}
}

func UnsupportedClosePaymentResult(instance domaincashier.ProviderInstance) ClosePaymentResult {
	result := BuildClosePaymentResult(instance, "unsupported", false, false, nil)
	result.CloseStatus = "unsupported"
	result.Unsupported = true
	result.OutcomeUncertain = false
	result.Message = "payment provider does not support safe order close"
	return result
}
