package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeRouteImagePricingStrictKnownSubtree(t *testing.T) {
	good := `{"code":"priced","name":"Priced","extra_future":true,"image_pricing":{"schema_version":1,"rules":[{"operation":"generate","mode":"per_image","currency":"CNY","version":"v1","input":{"free_images":0,"per_image":"0"},"output_tiers":[{"name":"1K","max_pixels":100,"price":"0.1"}]}]}}`
	req, ok := decodeRouteModelWriteRequest(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(good)))
	if !ok || req.ImagePricing == nil || req.ImagePricing.Rules[0].OutputTiers[0].Price != "0.1" {
		t.Fatal("route pricing lost in decoder")
	}
	bad := strings.Replace(good, `"input":{"free_images":0`, `"input":{"surprise":true,"free_images":0`, 1)
	if _, ok := decodeRouteModelWriteRequest(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(bad))); ok {
		t.Fatal("unknown pricing key accepted")
	}
}

func TestDecodeRoutePricingPresence(t *testing.T) {
	for _, tc := range []struct {
		body    string
		present bool
	}{{`{"name":"Route"}`, false}, {`{"name":"Route","image_pricing":null}`, true}} {
		req, ok := decodeRouteModelWriteRequest(httptest.NewRecorder(), httptest.NewRequest("PUT", "/", strings.NewReader(tc.body)))
		if !ok || req.ImagePricingPresent != tc.present || req.ImagePricing != nil {
			t.Fatalf("%s: %+v %v", tc.body, req, ok)
		}
	}
}
