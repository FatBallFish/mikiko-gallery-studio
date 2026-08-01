package cashier

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/shopspring/decimal"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/paymentintent"
)

// StripePaymentIntents is the subset of Stripe's client used by cashier flows.
type StripePaymentIntents interface {
	New(*stripe.PaymentIntentParams) (*stripe.PaymentIntent, error)
	Get(string, *stripe.PaymentIntentParams) (*stripe.PaymentIntent, error)
}

type stripePaymentIntentsFactory func(secretKey string) StripePaymentIntents

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
