package cashier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	apperrs "github.com/fatballfish/pic-gallery/pkg/errs"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/webhook"
)

type recordingStripePaymentIntents struct {
	newParams *stripe.PaymentIntentParams
	intent    *stripe.PaymentIntent
	newErr    error
	getID     string
	getIntent *stripe.PaymentIntent
}

func (c *recordingStripePaymentIntents) New(params *stripe.PaymentIntentParams) (*stripe.PaymentIntent, error) {
	c.newParams = params
	return c.intent, c.newErr
}

func (c *recordingStripePaymentIntents) Get(id string, _ *stripe.PaymentIntentParams) (*stripe.PaymentIntent, error) {
	c.getID = id
	return c.getIntent, nil
}

type recordingStripeRefunds struct {
	newParams *stripe.RefundParams
	refund    *stripe.Refund
	newErr    error
	getErr    error
}

func (c *recordingStripeRefunds) New(params *stripe.RefundParams) (*stripe.Refund, error) {
	c.newParams = params
	return c.refund, c.newErr
}

func (c *recordingStripeRefunds) Get(string, *stripe.RefundParams) (*stripe.Refund, error) {
	return c.refund, c.getErr
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

func TestConfigureStripeAPIBackendUsesOnlyLoopbackServer(t *testing.T) {
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payment_intents" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"pi_loopback","object":"payment_intent","amount":1025,"currency":"cny","client_secret":"pi_loopback_secret_client","status":"requires_payment_method"}`))
	}))
	defer server.Close()

	if err := ConfigureStripeAPIBackend(server.URL); err != nil {
		t.Fatalf("ConfigureStripeAPIBackend loopback: %v", err)
	}
	result, err := NewStripePaymentDisplayBuilder()(context.Background(), PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "stripe", Config: map[string]any{
			"publishable_key": "pk_test_loopback",
			"secret_key":      "sk_test_loopback",
		}},
		OrderNo: "PGO-LOOPBACK", AmountCNY: "10.25", Subject: "Loopback",
	}, nil)
	if err != nil {
		t.Fatalf("build payment display through loopback: %v", err)
	}
	if result.ClientToken != "pi_loopback" {
		t.Fatalf("unexpected loopback PaymentIntent: %#v", result)
	}
	if err := ConfigureStripeAPIBackend("https://api.stripe.com"); err == nil {
		t.Fatal("expected non-loopback Stripe API base URL to be rejected")
	}
}

func TestStripeAmountFenFromCNYRejectsFractionalFen(t *testing.T) {
	if _, err := StripeAmountFenFromCNY("10.251"); err == nil {
		t.Fatal("expected fractional fen amount to be rejected")
	}
}

func TestStripePaymentIntentBuilderSeparatesDefiniteRejectionFromTransportUncertainty(t *testing.T) {
	req := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "stripe", Config: map[string]any{
			"publishable_key": "pk_test_public",
			"secret_key":      "sk_test_private",
		}},
		OrderNo: "PGO-STRIPE-OUTCOME", AmountCNY: "10.00", Subject: "outcome classification",
	}

	definiteBuilder := newStripePaymentDisplayBuilder(func(string) StripePaymentIntents {
		return &recordingStripePaymentIntents{newErr: &stripe.Error{
			HTTPStatusCode: http.StatusUnauthorized,
			Type:           stripe.ErrorTypeInvalidRequest,
			Msg:            "invalid API key",
		}}
	})
	_, err := definiteBuilder(t.Context(), req, nil)
	if err == nil || PaymentInitializationOutcomeUncertain(err) {
		t.Fatalf("Stripe 4xx rejection must be definite, got %v", err)
	}

	uncertainBuilder := newStripePaymentDisplayBuilder(func(string) StripePaymentIntents {
		return &recordingStripePaymentIntents{newErr: errors.New("connection reset after request write")}
	})
	_, err = uncertainBuilder(t.Context(), req, nil)
	if err == nil || !PaymentInitializationOutcomeUncertain(err) {
		t.Fatalf("Stripe transport failure must remain uncertain, got %v", err)
	}
}

func TestStripeWebhookParsesSignedPaymentIntentFromExactBody(t *testing.T) {
	payload := []byte(fmt.Sprintf(`{"id":"evt_stripe_success","object":"event","api_version":%q,"type":"payment_intent.succeeded","data":{"object":{"id":"pi_webhook_123","object":"payment_intent","amount":1025,"currency":"cny","metadata":{"order_no":"PGO-STRIPE-WEBHOOK-001"},"status":"succeeded"}}}`, stripe.APIVersion))
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: "whsec_test"})

	event, err := ParseStripeWebhookEvent(payload, signed.Header, "whsec_test")
	if err != nil {
		t.Fatalf("parse signed Stripe webhook: %v", err)
	}
	if event.EventID != "evt_stripe_success" || event.Type != "payment_intent.succeeded" || event.PaymentIntentID != "pi_webhook_123" || event.OrderNo != "PGO-STRIPE-WEBHOOK-001" || event.AmountCNY != "10.25" || event.Currency != "cny" {
		t.Fatalf("unexpected Stripe webhook event %#v", event)
	}

	tampered := append([]byte(nil), payload...)
	tampered[len(tampered)-2] = 'x'
	if _, err := ParseStripeWebhookEvent(tampered, signed.Header, "whsec_test"); !errors.Is(err, ErrStripeWebhookSignatureInvalid) {
		t.Fatalf("expected tampered exact body to fail signature verification, got %v", err)
	}
}

func TestStripeOrderQueryMapsPaymentIntentStatuses(t *testing.T) {
	tests := []struct {
		status stripe.PaymentIntentStatus
		want   string
		paid   bool
	}{
		{status: stripe.PaymentIntentStatusSucceeded, want: "paid", paid: true},
		{status: stripe.PaymentIntentStatusProcessing, want: "pending"},
		{status: stripe.PaymentIntentStatusRequiresAction, want: "pending"},
		{status: stripe.PaymentIntentStatusCanceled, want: "failed"},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			client := &recordingStripePaymentIntents{getIntent: &stripe.PaymentIntent{
				ID:       "pi_query_123",
				Amount:   1025,
				Currency: stripe.CurrencyCNY,
				Status:   tt.status,
			}}
			builder := newStripeOrderStatusQueryBuilder(func(string) StripePaymentIntents { return client })
			result, err := builder(context.Background(), QueryOrderStatusRequest{
				Order: OrderSnapshot{OrderNo: "PGO-QUERY-STRIPE", AmountCNY: "10.25", ClientToken: "pi_query_123"},
				Instance: domaincashier.ProviderInstance{ID: 8, ProviderType: "stripe", Config: map[string]any{
					"secret_key": "sk_test_query",
				}},
			})
			if err != nil {
				t.Fatalf("query Stripe PaymentIntent: %v", err)
			}
			if client.getID != "pi_query_123" || result.QueryStatus != tt.want || result.Paid != tt.paid || result.TradeNo != "pi_query_123" || result.AmountCNY != "10.25" {
				t.Fatalf("unexpected Stripe query result %#v client=%#v", result, client)
			}
		})
	}
}

func TestStripeRefundUsesExactAmountAndLocalIdempotencyKey(t *testing.T) {
	for _, tt := range []struct {
		name       string
		amountCNY  string
		wantAmount int64
	}{
		{name: "partial", amountCNY: "5.25", wantAmount: 525},
		{name: "full", amountCNY: "", wantAmount: 1025},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingStripeRefunds{refund: &stripe.Refund{ID: "re_stripe_123", Amount: tt.wantAmount, Currency: stripe.CurrencyCNY, Status: stripe.RefundStatusSucceeded}}
			builder := newStripeRefundPaymentBuilder(func(string) StripeRefunds { return client })
			result, err := builder(context.Background(), RefundPaymentRequest{
				Order:           OrderSnapshot{OrderNo: "PGO-STRIPE-REFUND", AmountCNY: "10.25", TradeNo: "pi_refund_123", Status: "completed"},
				Instance:        domaincashier.ProviderInstance{ID: 9, ProviderType: "stripe", Config: map[string]any{"secret_key": "sk_test_refund"}},
				RefundTradeNo:   "LOCAL-REFUND-001",
				RefundAmountCNY: tt.amountCNY,
				Reason:          "requested by user",
			})
			if err != nil {
				t.Fatalf("create Stripe refund: %v", err)
			}
			if client.newParams == nil || client.newParams.Amount == nil || *client.newParams.Amount != tt.wantAmount || client.newParams.PaymentIntent == nil || *client.newParams.PaymentIntent != "pi_refund_123" {
				t.Fatalf("unexpected Stripe refund params %#v", client.newParams)
			}
			if client.newParams.IdempotencyKey == nil || *client.newParams.IdempotencyKey != "LOCAL-REFUND-001" || client.newParams.Metadata["refund_trade_no"] != "LOCAL-REFUND-001" {
				t.Fatalf("expected local refund idempotency metadata, got %#v", client.newParams)
			}
			if result.RefundStatus != "succeeded" || result.ChannelRefundNo != "re_stripe_123" || result.RefundTradeNo != "LOCAL-REFUND-001" {
				t.Fatalf("unexpected Stripe refund result %#v", result)
			}
		})
	}
}

func TestStripeRefundMarksOnlyPostCallFailuresAsOutcomeUncertain(t *testing.T) {
	request := RefundPaymentRequest{
		Order:           OrderSnapshot{OrderNo: "PGO-STRIPE-OUTCOME", AmountCNY: "10.00", TradeNo: "pi_outcome", Status: "completed"},
		Instance:        domaincashier.ProviderInstance{ID: 9, ProviderType: "stripe", Config: map[string]any{}},
		RefundTradeNo:   "REFUND-OUTCOME-001",
		RefundAmountCNY: "5.00",
	}
	missingSecretResult, missingSecretErr := newStripeRefundPaymentBuilder(func(string) StripeRefunds {
		t.Fatal("missing secret must fail before creating a Stripe client")
		return nil
	})(context.Background(), request)
	if missingSecretErr == nil || missingSecretResult.OutcomeUncertain {
		t.Fatalf("missing secret is a certain pre-call failure, result=%#v err=%v", missingSecretResult, missingSecretErr)
	}

	request.Instance.Config["secret_key"] = "sk_test_outcome"
	transportResult, transportErr := newStripeRefundPaymentBuilder(func(string) StripeRefunds {
		return &recordingStripeRefunds{newErr: errors.New("connection reset after request write")}
	})(context.Background(), request)
	if transportErr == nil || !transportResult.OutcomeUncertain {
		t.Fatalf("post-call transport failure must be uncertain, result=%#v err=%v", transportResult, transportErr)
	}
}

func TestStripeQueryAndRefundRejectCurrencyOrAmountMismatch(t *testing.T) {
	queryClient := &recordingStripePaymentIntents{getIntent: &stripe.PaymentIntent{
		ID: "pi_wrong_currency", Amount: 1025, Currency: stripe.CurrencyUSD, Status: stripe.PaymentIntentStatusSucceeded,
	}}
	_, queryErr := newStripeOrderStatusQueryBuilder(func(string) StripePaymentIntents { return queryClient })(context.Background(), QueryOrderStatusRequest{
		Order:    OrderSnapshot{ClientToken: "pi_wrong_currency", AmountCNY: "10.25"},
		Instance: domaincashier.ProviderInstance{ProviderType: "stripe", Config: map[string]any{"secret_key": "sk_test"}},
	})
	assertPaymentAmountMismatchError(t, queryErr)

	refundClient := &recordingStripeRefunds{refund: &stripe.Refund{
		ID: "re_wrong_amount", Amount: 500, Currency: stripe.CurrencyCNY, Status: stripe.RefundStatusSucceeded,
	}}
	_, refundErr := newStripeRefundPaymentBuilder(func(string) StripeRefunds { return refundClient })(context.Background(), RefundPaymentRequest{
		Order:           OrderSnapshot{OrderNo: "PGO-MISMATCH", AmountCNY: "10.25", TradeNo: "pi_wrong_amount", Status: "completed"},
		Instance:        domaincashier.ProviderInstance{ProviderType: "stripe", Config: map[string]any{"secret_key": "sk_test"}},
		RefundTradeNo:   "REFUND-MISMATCH",
		RefundAmountCNY: "5.25",
	})
	assertPaymentAmountMismatchError(t, refundErr)
}

func assertPaymentAmountMismatchError(t *testing.T, err error) {
	t.Helper()
	var appErr *apperrs.Error
	if !errors.As(err, &appErr) || appErr.Code != apperrs.CodePaymentAmountMismatch {
		t.Fatalf("expected PAYMENT_AMOUNT_MISMATCH, got %T %v", err, err)
	}
}
