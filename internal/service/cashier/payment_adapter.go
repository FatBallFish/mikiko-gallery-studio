package cashier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
)

var ErrPaymentProviderNotImplemented = errors.New("payment provider adapter is not implemented")

type paymentInitializationError struct {
	err              error
	outcomeUncertain bool
}

func (e *paymentInitializationError) Error() string { return e.err.Error() }
func (e *paymentInitializationError) Unwrap() error { return e.err }

func paymentInitializationOutcomeUncertain(err error) error {
	if err == nil {
		return nil
	}
	return &paymentInitializationError{err: err, outcomeUncertain: true}
}

func PaymentInitializationOutcomeUncertain(err error) bool {
	var initializationErr *paymentInitializationError
	return errors.As(err, &initializationErr) && initializationErr.outcomeUncertain
}

type PaymentDisplayRequest struct {
	Method          domaincashier.VisibleMethod
	Instance        domaincashier.ProviderInstance
	OrderNo         string
	AmountCNY       string
	Subject         string
	ClientReturnURL string
}

type PaymentDisplayResult struct {
	Display     map[string]any
	PaymentURL  string
	QRCode      string
	ClientToken string
}

type PaymentDisplayBuilder func(ctx context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error)

type PaymentProviderBuilders struct {
	AlipayDirect PaymentDisplayBuilder
	WxPayDirect  PaymentDisplayBuilder
	EasyPay      PaymentDisplayBuilder
	JeePay       PaymentDisplayBuilder
	Stripe       PaymentDisplayBuilder
}

type PaymentAdapterRegistry struct {
	mu       sync.RWMutex
	builders map[string]PaymentDisplayBuilder
}

func NewPaymentAdapterRegistry() *PaymentAdapterRegistry {
	registry := &PaymentAdapterRegistry{builders: map[string]PaymentDisplayBuilder{}}
	registry.Register("mock", mockPaymentDisplayBuilder)
	return registry
}

func NewPaymentAdapterRegistryWithBuilders(builders PaymentProviderBuilders) *PaymentAdapterRegistry {
	registry := NewPaymentAdapterRegistry()
	registry.Register("alipay_direct", builders.AlipayDirect)
	registry.Register("wxpay_direct", builders.WxPayDirect)
	registry.Register("easypay_alipay", builders.EasyPay)
	registry.Register("easypay_wxpay", builders.EasyPay)
	registry.Register("jeepay_alipay", builders.JeePay)
	registry.Register("jeepay_wxpay", builders.JeePay)
	registry.Register("stripe", builders.Stripe)
	return registry
}

func (r *PaymentAdapterRegistry) Register(providerType string, builder PaymentDisplayBuilder) {
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

func (r *PaymentAdapterRegistry) BuildPaymentDisplay(ctx context.Context, req PaymentDisplayRequest) (PaymentDisplayResult, error) {
	providerType := strings.ToLower(strings.TrimSpace(req.Instance.ProviderType))
	display := BasePaymentDisplay(req, providerType)
	if r == nil {
		return PaymentDisplayResult{}, fmt.Errorf("%w: %s", ErrPaymentProviderNotImplemented, providerType)
	}

	r.mu.RLock()
	builder := r.builders[providerType]
	r.mu.RUnlock()
	if builder == nil {
		return PaymentDisplayResult{}, fmt.Errorf("%w: %s", ErrPaymentProviderNotImplemented, providerType)
	}
	return builder(ctx, req, display)
}

func BasePaymentDisplay(req PaymentDisplayRequest, providerType string) map[string]any {
	return map[string]any{
		"type":                 "redirect",
		"visible_method":       strings.ToLower(strings.TrimSpace(req.Method.Method)),
		"provider_type":        strings.ToLower(strings.TrimSpace(providerType)),
		"provider_instance_id": req.Instance.ID,
		"order_no":             strings.TrimSpace(req.OrderNo),
		"amount_cny":           strings.TrimSpace(req.AmountCNY),
	}
}

func mockPaymentDisplayBuilder(_ context.Context, _ PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
	display["type"] = "mock"
	return PaymentDisplayResult{Display: display}, nil
}

func cashierAmountCNYWithExactFen(raw string) (string, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !amount.IsPositive() {
		return "", errs.BadRequest("amount_cny is invalid")
	}
	amountFen := amount.Mul(decimal.NewFromInt(100))
	if !amountFen.Equal(amountFen.Truncate(0)) {
		return "", errs.BadRequest("amount_cny must not contain fractional fen")
	}
	return strings.TrimSpace(raw), nil
}
