package oauthserver

import (
	"net/netip"
	"testing"
)

func TestOAuthMetadataAddressPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "198.18.0.1", "192.0.2.1", "2001:db8::1", "64:ff9b::a00:1", "::ffff:127.0.0.1"} {
		if publicAddress(netip.MustParseAddr(raw)) {
			t.Errorf("accepted nonpublic address %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicAddress(netip.MustParseAddr(raw)) {
			t.Errorf("rejected public address %s", raw)
		}
	}
}
func TestOAuthRejectUnsafeClientMetadata(t *testing.T) {
	for _, redirect := range []string{"https://good.example/cb#fragment", "http://remote.example/cb", "https://user:pass@example.com/cb", "javascript:alert(1)", "https:///cb"} {
		if validateClient(Client{Name: "App", RedirectURIs: []string{redirect}}) == nil {
			t.Errorf("accepted %s", redirect)
		}
	}
	if validateClient(Client{Name: "App", RedirectURIs: []string{"https://example.com/cb"}, TokenEndpointAuthMethod: "client_secret_basic"}) == nil {
		t.Fatal("accepted unsupported auth method")
	}
}
