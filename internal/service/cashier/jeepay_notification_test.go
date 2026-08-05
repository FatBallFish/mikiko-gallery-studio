package cashier

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestJeePayNotificationNormalizesJSONAndFormBodies(t *testing.T) {
	jsonBody := []byte(`{"mchNo":"MCH10001","appId":"APP10001","mchOrderNo":"PGO-100","amount":1250,"state":2,"status":"SUCCESS","payOrderId":"JP-100","sign":"ABCDEF"}`)
	formValues := url.Values{
		"mchNo":      {"MCH10001"},
		"appId":      {"APP10001"},
		"mchOrderNo": {"PGO-100"},
		"amount":     {"1250"},
		"state":      {"2"},
		"status":     {"SUCCESS"},
		"payOrderId": {"JP-100"},
		"sign":       {"ABCDEF"},
	}

	fromJSON, err := ParseJeePayNotification(jsonBody, "application/json; charset=utf-8")
	if err != nil {
		t.Fatalf("ParseJeePayNotification(JSON) error = %v", err)
	}
	fromForm, err := ParseJeePayNotification([]byte(formValues.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		t.Fatalf("ParseJeePayNotification(form) error = %v", err)
	}
	if !reflect.DeepEqual(fromJSON, fromForm) {
		t.Fatalf("canonical notifications differ:\nJSON: %#v\nform: %#v", fromJSON, fromForm)
	}
}

func TestJeePayNotificationFallsBackWhenContentTypeIsIncorrect(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{
			name:        "JSON labeled as form",
			contentType: "application/x-www-form-urlencoded",
			body:        `{"mchNo":"MCH10001","amount":1250,"state":2}`,
		},
		{
			name:        "form labeled as JSON",
			contentType: "application/json",
			body:        "mchNo=MCH10001&amount=1250&state=2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJeePayNotification([]byte(tt.body), tt.contentType)
			if err != nil {
				t.Fatalf("ParseJeePayNotification() error = %v", err)
			}
			if got["mchNo"] != "MCH10001" || got["amount"] != "1250" || got["state"] != "2" {
				t.Fatalf("ParseJeePayNotification() = %#v", got)
			}
		})
	}
}

func TestJeePayNotificationPreservesSigningFieldValues(t *testing.T) {
	body := []byte(`{"largeInteger":900719925474099312345,"decimal":12.5000,"enabled":true,"empty":"","escaped":"A\u0026B","sign":"AbCdEf0123"}`)

	got, err := ParseJeePayNotification(body, "application/json")
	if err != nil {
		t.Fatalf("ParseJeePayNotification() error = %v", err)
	}
	want := map[string]string{
		"largeInteger": "900719925474099312345",
		"decimal":      "12.5000",
		"enabled":      "true",
		"empty":        "",
		"escaped":      "A&B",
		"sign":         "AbCdEf0123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseJeePayNotification() = %#v, want %#v", got, want)
	}
}

func TestJeePayNotificationRejectsInvalidBodies(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "empty", contentType: "application/json", body: " \n\t"},
		{name: "JSON array", contentType: "application/json", body: `["value"]`},
		{name: "nested JSON object", contentType: "application/json", body: `{"mchNo":{"value":"MCH=10001"}}`},
		{name: "nested JSON array", contentType: "application/json", body: `{"items":["one"]}`},
		{name: "JSON null", contentType: "application/json", body: `{"mchNo":null}`},
		{name: "duplicate JSON field", contentType: "application/json", body: `{"mchNo":"one","mchNo":"two=2"}`},
		{name: "duplicate form field", contentType: "application/x-www-form-urlencoded", body: `mchNo=one&mchNo=two`},
		{name: "invalid form escape", contentType: "application/x-www-form-urlencoded", body: `mchNo=%zz`},
		{name: "trailing JSON value", contentType: "application/json", body: `{"mchNo":"one"} {"mchNo":"two"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := ParseJeePayNotification([]byte(tt.body), tt.contentType); err == nil {
				t.Fatalf("ParseJeePayNotification() = %#v, want error", got)
			}
		})
	}
}

func TestJeePayNotificationRejectsOversizedBody(t *testing.T) {
	body := []byte("value=" + strings.Repeat("x", maxJeePayNotificationBodyBytes))

	if got, err := ParseJeePayNotification(body, "application/x-www-form-urlencoded"); err == nil {
		t.Fatalf("ParseJeePayNotification() = %#v, want oversized-body error", got)
	}
}
