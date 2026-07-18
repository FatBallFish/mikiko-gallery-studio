package storage

import (
	"strings"
	"testing"
)

func TestStorageInvalidationPayloadContainsOnlyRoutingIdentity(t *testing.T) {
	event := StorageInvalidation{ConfigID: "config-id", Version: 7, DefaultChanged: true}
	payload, err := encodeStorageInvalidation(event)
	if err != nil {
		t.Fatalf("encode invalidation: %v", err)
	}
	if strings.Contains(string(payload), "secret") || strings.Contains(string(payload), "endpoint") {
		t.Fatalf("invalidation payload leaked config details: %s", payload)
	}
	decoded, err := decodeStorageInvalidation(payload)
	if err != nil {
		t.Fatalf("decode invalidation: %v", err)
	}
	if decoded != event {
		t.Fatalf("decoded event mismatch: got %#v want %#v", decoded, event)
	}
}
