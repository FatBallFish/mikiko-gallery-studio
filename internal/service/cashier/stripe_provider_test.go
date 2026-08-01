package cashier

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	stripe "github.com/stripe/stripe-go/v85"
)

type recordingStripePaymentIntents struct {
	newParams *stripe.PaymentIntentParams
	intent    *stripe.PaymentIntent
}

func (c *recordingStripePaymentIntents) New(params *stripe.PaymentIntentParams) (*stripe.PaymentIntent, error) {
	c.newParams = params
	return c.intent, nil
}

func (c *recordingStripePaymentIntents) Get(string, *stripe.PaymentIntentParams) (*stripe.PaymentIntent, error) {
	return nil, nil
}

func TestStripePaymentIntentBuilderCreatesExactIdempotentIntent(t *testing.T) {
	client := &recordingStripePaymentIntents{intent: &stripe.PaymentIntent{
		ID:           "pi_test_123",
		ClientSecret: "pi_test_123_secret_client",
	}}
	requestedSecretKey := ""
	builder := newStripePaymentDisplayBuilder(func(secretKey string) StripePaymentIntents {
		requestedSecretKey = secretKey
		return client
	})
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "stripe"},
		Instance: domaincashier.ProviderInstance{
			ID:           7,
			ProviderType: "stripe",
			Config: map[string]any{
				"publishable_key": "pk_test_public",
				"secret_key":      "sk_test_private",
				"webhook_secret":  "whsec_private",
			},
		},
		OrderNo:   "PGO-STRIPE-001",
		AmountCNY: "10.25",
		Subject:   "1025 points",
	}

	result, err := builder(context.Background(), req, BasePaymentDisplay(req, "stripe"))
	if err != nil {
		t.Fatalf("build Stripe payment display: %v", err)
	}
	if requestedSecretKey != "sk_test_private" {
		t.Fatalf("expected instance secret key, got %q", requestedSecretKey)
	}
	if client.newParams == nil || client.newParams.Amount == nil || *client.newParams.Amount != 1025 {
		t.Fatalf("expected exact 1025 fen amount, got %#v", client.newParams)
	}
	if client.newParams.Currency == nil || *client.newParams.Currency != string(stripe.CurrencyCNY) {
		t.Fatalf("expected cny currency, got %#v", client.newParams.Currency)
	}
	if client.newParams.Metadata["order_no"] != req.OrderNo {
		t.Fatalf("expected order metadata, got %#v", client.newParams.Metadata)
	}
	if client.newParams.IdempotencyKey == nil || *client.newParams.IdempotencyKey != req.OrderNo {
		t.Fatalf("expected order number as idempotency key, got %#v", client.newParams.IdempotencyKey)
	}
	wantDisplay := map[string]any{
		"type":            "stripe_payment_element",
		"client_secret":   "pi_test_123_secret_client",
		"publishable_key": "pk_test_public",
	}
	if encoded, _ := json.Marshal(result.Display); string(encoded) != `{"client_secret":"pi_test_123_secret_client","publishable_key":"pk_test_public","type":"stripe_payment_element"}` {
		t.Fatalf("unexpected narrow display: got %s want %#v", encoded, wantDisplay)
	}
	if result.ClientToken != "pi_test_123" {
		t.Fatalf("expected PaymentIntent ID transport, got %q", result.ClientToken)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode result: %v", err)
	}
	for _, secret := range []string{"sk_test_private", "whsec_private"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("payment response leaked provider secret %q", secret)
		}
	}
}

func TestStripeAmountFenFromCNYRejectsFractionalFen(t *testing.T) {
	if _, err := StripeAmountFenFromCNY("10.251"); err == nil {
		t.Fatal("expected fractional fen amount to be rejected")
	}
}
