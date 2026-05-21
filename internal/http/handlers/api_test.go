package handlers

import (
	"strings"
	"testing"
)

func TestReadBoundedBodyRejectsOversizedUnsignedBody(t *testing.T) {
	if _, err := readBoundedBody(strings.NewReader(strings.Repeat("x", 3<<20)), 1); err == nil {
		t.Fatal("expected oversized signed body to be rejected")
	}
	body, err := readBoundedBody(strings.NewReader("small"), 1)
	if err != nil {
		t.Fatalf("expected small body to pass: %v", err)
	}
	if string(body) != "small" {
		t.Fatalf("unexpected body %q", string(body))
	}
}
