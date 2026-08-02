package cashier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	apperrs "github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/paymentintent"
	striperefund "github.com/stripe/stripe-go/v85/refund"
	"github.com/stripe/stripe-go/v85/webhook"
)

var ErrStripeWebhookSignatureInvalid = errors.New("Stripe webhook signature is invalid")

type StripeWebhookEvent struct {
	EventID         string
	Type            string
	PaymentIntentID string
	OrderNo         string
	AmountCNY       string
	Currency        string
}

// StripePaymentIntents is the subset of Stripe's client used by cashier flows.
type StripePaymentIntents interface {
	New(*stripe.PaymentIntentParams) (*stripe.PaymentIntent, error)
	Get(string, *stripe.PaymentIntentParams) (*stripe.PaymentIntent, error)
}

type stripePaymentIntentsFactory func(secretKey string) StripePaymentIntents

type StripeRefunds interface {
	New(*stripe.RefundParams) (*stripe.Refund, error)
	Get(string, *stripe.RefundParams) (*stripe.Refund, error)
}

type stripeRefundsFactory func(secretKey string) StripeRefunds

func ConfigureStripeAPIBackend(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("Stripe API base URL must be a loopback HTTP origin")
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	ip := net.ParseIP(hostname)
	if hostname != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("Stripe API base URL must be a loopback HTTP origin")
	}
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL:               stripe.String(strings.TrimRight(rawURL, "/")),
		MaxNetworkRetries: stripe.Int64(0),
	}))
	return nil
}

func NewStripePaymentDisplayBuilder() PaymentDisplayBuilder {
	return newStripePaymentDisplayBuilder(func(secretKey string) StripePaymentIntents {
		return &paymentintent.Client{B: stripe.GetBackend(stripe.APIBackend), Key: secretKey}
	})
}

func newStripePaymentDisplayBuilder(clientFactory stripePaymentIntentsFactory) PaymentDisplayBuilder {
	return func(ctx context.Context, req PaymentDisplayRequest, _ map[string]any) (PaymentDisplayResult, error) {
		publishableKey := configString(req.Instance.Config, "publishable_key")
		secretKey := configString(req.Instance.Config, "secret_key")
		if publishableKey == "" || secretKey == "" {
			return PaymentDisplayResult{}, fmt.Errorf("Stripe payment credentials are incomplete")
		}
		amountFen, err := StripeAmountFenFromCNY(req.AmountCNY)
		if err != nil {
			return PaymentDisplayResult{}, err
		}
		params := &stripe.PaymentIntentParams{
			Amount:      stripe.Int64(amountFen),
			Currency:    stripe.String(string(stripe.CurrencyCNY)),
			Description: stripe.String(strings.TrimSpace(req.Subject)),
			Metadata:    map[string]string{"order_no": strings.TrimSpace(req.OrderNo)},
			AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
				Enabled: stripe.Bool(true),
			},
		}
		params.Context = ctx
		params.SetIdempotencyKey(strings.TrimSpace(req.OrderNo))
		intent, err := clientFactory(secretKey).New(params)
		if err != nil {
			return PaymentDisplayResult{}, fmt.Errorf("create Stripe PaymentIntent: %w", err)
		}
		if intent == nil || strings.TrimSpace(intent.ID) == "" || strings.TrimSpace(intent.ClientSecret) == "" {
			return PaymentDisplayResult{}, fmt.Errorf("Stripe PaymentIntent response is incomplete")
		}
		return PaymentDisplayResult{
			Display: map[string]any{
				"type":            "stripe_payment_element",
				"client_secret":   strings.TrimSpace(intent.ClientSecret),
				"publishable_key": publishableKey,
			},
			ClientToken: strings.TrimSpace(intent.ID),
		}, nil
	}
}

func StripeAmountFenFromCNY(amountCNY string) (int64, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountCNY))
	if err != nil || !amount.IsPositive() {
		return 0, fmt.Errorf("invalid Stripe payment amount")
	}
	scaled := amount.Shift(2)
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, fmt.Errorf("Stripe payment amount has fractional fen")
	}
	amountFen := scaled.BigInt()
	if amountFen == nil || !amountFen.IsInt64() || amountFen.Cmp(big.NewInt(0)) <= 0 {
		return 0, fmt.Errorf("Stripe payment amount is out of range")
	}
	return amountFen.Int64(), nil
}

func ParseStripeWebhookEvent(payload []byte, signature, webhookSecret string) (StripeWebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, strings.TrimSpace(signature), strings.TrimSpace(webhookSecret))
	if err != nil {
		return StripeWebhookEvent{}, ErrStripeWebhookSignatureInvalid
	}
	parsed := StripeWebhookEvent{EventID: strings.TrimSpace(event.ID), Type: string(event.Type)}
	if !strings.HasPrefix(parsed.Type, "payment_intent.") {
		return parsed, nil
	}
	var intent stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &intent); err != nil {
		return StripeWebhookEvent{}, fmt.Errorf("decode Stripe PaymentIntent event: %w", err)
	}
	parsed.PaymentIntentID = strings.TrimSpace(intent.ID)
	parsed.OrderNo = strings.TrimSpace(intent.Metadata["order_no"])
	parsed.AmountCNY = decimal.NewFromInt(intent.Amount).Shift(-2).StringFixed(2)
	parsed.Currency = strings.ToLower(strings.TrimSpace(string(intent.Currency)))
	return parsed, nil
}

func NewStripeOrderStatusQueryBuilder() QueryOrderStatusBuilder {
	return newStripeOrderStatusQueryBuilder(func(secretKey string) StripePaymentIntents {
		return &paymentintent.Client{B: stripe.GetBackend(stripe.APIBackend), Key: secretKey}
	})
}

func newStripeOrderStatusQueryBuilder(clientFactory stripePaymentIntentsFactory) QueryOrderStatusBuilder {
	return func(ctx context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
		secretKey := configString(req.Instance.Config, "secret_key")
		intentID := strings.TrimSpace(req.Order.TradeNo)
		if intentID == "" {
			intentID = strings.TrimSpace(req.Order.ClientToken)
		}
		if secretKey == "" || intentID == "" {
			return QueryOrderStatusResult{}, paymentProviderUnavailable()
		}
		params := &stripe.PaymentIntentParams{}
		params.Context = ctx
		intent, err := clientFactory(secretKey).Get(intentID, params)
		if err != nil || intent == nil {
			return QueryOrderStatusResult{}, paymentProviderUnavailable()
		}
		if intent.Currency != stripe.CurrencyCNY {
			return QueryOrderStatusResult{}, stripePaymentAmountMismatch()
		}
		queryStatus := stripePaymentIntentQueryStatus(intent)
		amountCNY := decimal.NewFromInt(intent.Amount).Shift(-2).StringFixed(2)
		raw := map[string]any{
			"source":            "stripe_payment_intent",
			"payment_intent_id": strings.TrimSpace(intent.ID),
			"status":            string(intent.Status),
			"currency":          strings.ToLower(string(intent.Currency)),
			"amount_fen":        intent.Amount,
		}
		return BuildQueryOrderStatusResult(req.Instance, queryStatus, intent.ID, amountCNY, raw), nil
	}
}

func stripePaymentIntentQueryStatus(intent *stripe.PaymentIntent) QueryStatus {
	if intent == nil {
		return NormalizeQueryStatus("failed")
	}
	switch intent.Status {
	case stripe.PaymentIntentStatusSucceeded:
		return NormalizeQueryStatus("succeeded")
	case stripe.PaymentIntentStatusCanceled:
		return NormalizeQueryStatus("failed")
	case stripe.PaymentIntentStatusRequiresPaymentMethod:
		if intent.LastPaymentError != nil {
			return NormalizeQueryStatus("failed")
		}
		return NormalizeQueryStatus("pending")
	default:
		return NormalizeQueryStatus("pending")
	}
}

func NewStripeRefundPaymentBuilder() RefundPaymentBuilder {
	return newStripeRefundPaymentBuilder(func(secretKey string) StripeRefunds {
		return &striperefund.Client{B: stripe.GetBackend(stripe.APIBackend), Key: secretKey}
	})
}

func newStripeRefundPaymentBuilder(clientFactory stripeRefundsFactory) RefundPaymentBuilder {
	return func(ctx context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
		secretKey := configString(req.Instance.Config, "secret_key")
		intentID := strings.TrimSpace(req.Order.TradeNo)
		refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
		if secretKey == "" || intentID == "" || refundTradeNo == "" {
			return RefundPaymentResult{}, paymentRefundProviderUnavailable()
		}
		amountCNY := strings.TrimSpace(req.RefundAmountCNY)
		if amountCNY == "" {
			amountCNY = strings.TrimSpace(req.Order.AmountCNY)
		}
		amountFen, err := StripeAmountFenFromCNY(amountCNY)
		if err != nil {
			return RefundPaymentResult{}, err
		}
		params := &stripe.RefundParams{
			Amount:        stripe.Int64(amountFen),
			PaymentIntent: stripe.String(intentID),
			Metadata: map[string]string{
				"order_no":        strings.TrimSpace(req.Order.OrderNo),
				"refund_trade_no": refundTradeNo,
			},
		}
		params.Context = ctx
		params.SetIdempotencyKey(refundTradeNo)
		if strings.TrimSpace(req.Reason) != "" {
			params.Reason = stripe.String(string(stripe.RefundReasonRequestedByCustomer))
		}
		refundResult, err := clientFactory(secretKey).New(params)
		if err != nil || refundResult == nil || strings.TrimSpace(refundResult.ID) == "" {
			return RefundPaymentResult{}, paymentRefundProviderUnavailable()
		}
		if refundResult.Amount != amountFen || refundResult.Currency != stripe.CurrencyCNY {
			return RefundPaymentResult{}, stripePaymentAmountMismatch()
		}
		status := strings.ToLower(strings.TrimSpace(string(refundResult.Status)))
		if status == "" {
			status = "pending"
		}
		return RefundPaymentResult{
			ProviderType:       "stripe",
			ProviderInstanceID: req.Instance.ID,
			RefundStatus:       status,
			RefundTradeNo:      refundTradeNo,
			ChannelRefundNo:    strings.TrimSpace(refundResult.ID),
			Raw: map[string]any{
				"source":            "stripe_refund",
				"refund_id":         strings.TrimSpace(refundResult.ID),
				"status":            status,
				"amount_fen":        refundResult.Amount,
				"currency":          strings.ToLower(string(refundResult.Currency)),
				"payment_intent_id": intentID,
			},
			RefundedAt: time.Now().UTC(),
		}, nil
	}
}

func stripePaymentAmountMismatch() error {
	return apperrs.New(http.StatusConflict, apperrs.CodePaymentAmountMismatch, "payment amount does not match order")
}
