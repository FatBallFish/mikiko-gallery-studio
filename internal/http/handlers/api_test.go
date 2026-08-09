package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	cashierservice "github.com/fatballfish/pic-gallery/internal/service/cashier"
)

func TestPaymentReadinessMatchesCheckoutEligibilityAndIgnoresLegacySwitch(t *testing.T) {
	cfg := config.Config{}
	cfg.App.Env = "production"
	admin := adminconfigservice.NewService(cfg)
	if _, err := admin.UpdateTab(t.Context(), domainadminconfig.UpdateTabRequest{
		TabKey: "payments", Version: 1, Items: []domainadminconfig.Item{
			{ConfigCategory: "payments", ConfigKey: "enabled", ConfigValue: map[string]any{"value": false}, Scope: "global"},
			{ConfigCategory: "payments", ConfigKey: "visible_methods", ConfigValue: map[string]any{"value": []map[string]any{{"method": "alipay", "label": "支付宝", "enabled": true, "source_provider_type": "alipay_direct", "scheduler_strategy": "round_robin", "display_order": 10}}}, Scope: "global"},
			{ConfigCategory: "payments", ConfigKey: "provider_instances", ConfigValue: map[string]any{"value": []map[string]any{
				{"id": 1, "provider_type": "mock", "name": "Mock", "enabled": true, "supported_methods": []string{"alipay"}},
				{"id": 2, "provider_type": "easypay_alipay", "name": "Wrong provider type", "enabled": true, "supported_methods": []string{"alipay"}, "config": map[string]any{"pid": "merchant", "key": "secret"}},
				{"id": 3, "provider_type": "alipay_direct", "name": "Ready", "enabled": true, "supported_methods": []string{"alipay"}, "config": map[string]any{"app_id": "app", "payment_url": "https://pay.example.test"}},
			}}, Scope: "global"},
		},
	}); err != nil {
		t.Fatalf("UpdateTab payments: %v", err)
	}
	api := NewAPIWithServices(cfg, nil, nil, nil, admin)
	check := api.paymentReadinessCheck(t.Context(), time.Now().UTC())
	if check.Status != "pass" || check.Availability != "healthy" || !strings.Contains(check.Detail, "1 个支付方式均可服务") {
		methods := api.cashierVisibleMethods(t.Context(), false)
		instances := api.cashierProviderInstances(t.Context())
		t.Fatalf("payment readiness = %#v methods=%#v instances=%#v", check, methods, instances)
	}
}

func TestCanceledReadinessRequestDoesNotPopulateCache(t *testing.T) {
	api := NewAPIWithModelAdminService(config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	checks := 0
	api.SetDocsReadinessChecker(func(context.Context) DocsReadinessResult {
		checks++
		return DocsReadinessResult{Status: "pass", Detail: "ready"}
	})
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, appErr := api.adminReadinessChecks(canceled); appErr == nil {
		t.Fatal("canceled readiness request must fail")
	}
	if _, appErr := api.adminReadinessChecks(t.Context()); appErr != nil {
		t.Fatalf("live readiness request: %v", appErr)
	}
	if checks != 2 {
		t.Fatalf("canceled readiness result was cached: docs checks=%d", checks)
	}
}

type docsReadinessRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn docsReadinessRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type docsReadinessResolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (fn docsReadinessResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return fn(ctx, host)
}

type docsReadinessDialFunc func(context.Context, string, string) (net.Conn, error)

func (fn docsReadinessDialFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return fn(ctx, network, address)
}

func TestResolveDocsReadinessTarget(t *testing.T) {
	tests := []struct {
		name    string
		runtime config.RuntimeConfig
		want    string
		wantErr bool
	}{
		{
			name: "docker full uses its internal gateway",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "docs-web", "gateway"},
				DocsURL: "/developer-docs/",
			},
			want: "http://gateway/developer-docs/",
		},
		{
			name: "native local gateway uses its loopback port",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeNative, DeploymentModules: []string{"api", "docs-web", "gateway"},
				GatewayPort: "18000", DocsURL: "/developer-docs/",
			},
			want: "http://127.0.0.1:18000/developer-docs/",
		},
		{
			name: "API only node uses explicit probe target instead of public API origin",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api"},
				PublicAPIURL: "https://api.example.test/v1", DocsURL: "/developer-docs/",
				DocsProbeURL: "https://gateway.example.test/developer-docs/",
			},
			want: "https://gateway.example.test/developer-docs/",
		},
		{
			name:    "absolute user target is retained when probe target is absent",
			runtime: config.RuntimeConfig{PublicAPIURL: "https://api.example.test", DocsURL: "https://docs.example.test/reference/"},
			want:    "https://docs.example.test/reference/",
		},
		{
			name: "explicit probe separates API and Gateway origins",
			runtime: config.RuntimeConfig{
				PublicAPIURL: "https://api.example.test", DocsURL: "/developer-docs/",
				DocsProbeURL: "https://studio.example.test/developer-docs/",
			},
			want: "https://studio.example.test/developer-docs/",
		},
		{name: "relative target without local gateway or explicit probe fails", runtime: config.RuntimeConfig{DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api"}, PublicAPIURL: "https://api.example.test", DocsURL: "/developer-docs/"}, wantErr: true},
		{name: "credentials are rejected", runtime: config.RuntimeConfig{DocsURL: "https://user:secret@docs.example.test/"}, wantErr: true},
		{name: "non HTTP scheme is rejected", runtime: config.RuntimeConfig{DocsURL: "file:///tmp/docs"}, wantErr: true},
		{name: "empty host is rejected", runtime: config.RuntimeConfig{DocsURL: "http://:80/developer-docs/"}, wantErr: true},
		{name: "out of range port is rejected", runtime: config.RuntimeConfig{DocsURL: "http://docs.example.test:99999/developer-docs/"}, wantErr: true},
		{name: "encoded path traversal is rejected", runtime: config.RuntimeConfig{DocsURL: "https://docs.example.test/developer-docs/%2e%2e/healthz"}, wantErr: true},
		{name: "query is rejected", runtime: config.RuntimeConfig{DocsURL: "https://docs.example.test/?token=secret"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := resolveDocsReadinessTarget(tt.runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveDocsReadinessTarget(%#v) succeeded with %v", tt.runtime, target)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDocsReadinessTarget(%#v): %v", tt.runtime, err)
			}
			if target.String() != tt.want {
				t.Fatalf("target = %q, want %q", target, tt.want)
			}
		})
	}
}

func TestResolveDocsReadinessProbeTargetUsesEffectiveDocsURLAndProvenance(t *testing.T) {
	tests := []struct {
		name           string
		runtime        config.RuntimeConfig
		wantURL        string
		wantProvenance docsReadinessTargetProvenance
		wantErr        bool
	}{
		{
			name: "absolute docs URL ignores an internal probe",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "gateway"},
				DocsURL: "https://docs.example.test/reference/", DocsProbeURL: "http://gateway/developer-docs/",
			},
			wantURL: "https://docs.example.test/reference/", wantProvenance: docsReadinessTargetConfiguredPublic,
		},
		{
			name: "derived Docker gateway is trusted topology",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "gateway"}, DocsURL: "/developer-docs/",
			},
			wantURL: "http://gateway/developer-docs/", wantProvenance: docsReadinessTargetTrustedTopology,
		},
		{
			name: "real local Compose nginx is trusted for its declared Docker topology",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker,
				DeploymentModules: []string{
					"postgres", "redis", "minio", "mailpit", "api", "worker", "user-web", "admin-web", "docs-web", "nginx",
				},
				DocsURL: "/developer-docs/", DocsProbeURL: "http://nginx/developer-docs/",
			},
			wantURL: "http://nginx/developer-docs/", wantProvenance: docsReadinessTargetTrustedTopology,
		},
		{
			name: "nginx host without nginx module uses public policy",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "docs-web"},
				DocsURL: "/developer-docs/", DocsProbeURL: "http://nginx/developer-docs/",
			},
			wantURL: "http://nginx/developer-docs/", wantProvenance: docsReadinessTargetConfiguredPublic,
		},
		{
			name: "nginx module does not trust an external host",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "docs-web", "nginx"},
				DocsURL: "/developer-docs/", DocsProbeURL: "https://docs.example.test/developer-docs/",
			},
			wantURL: "https://docs.example.test/developer-docs/", wantProvenance: docsReadinessTargetConfiguredPublic,
		},
		{
			name: "nginx module does not trust a non-documentation path",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "docs-web", "nginx"},
				DocsURL: "/healthz", DocsProbeURL: "http://nginx/healthz",
			},
			wantURL: "http://nginx/healthz", wantProvenance: docsReadinessTargetConfiguredPublic,
		},
		{
			name: "nginx probe with a different path is rejected",
			runtime: config.RuntimeConfig{
				DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "docs-web", "nginx"},
				DocsURL: "/developer-docs/", DocsProbeURL: "http://nginx/healthz",
			},
			wantErr: true,
		},
		{
			name: "external relative probe uses public policy",
			runtime: config.RuntimeConfig{DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api"},
				DocsURL: "/developer-docs/", DocsProbeURL: "https://gateway.example.test/developer-docs/"},
			wantURL: "https://gateway.example.test/developer-docs/", wantProvenance: docsReadinessTargetConfiguredPublic,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := resolveDocsReadinessProbeTarget(tt.runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveDocsReadinessProbeTarget(%#v) succeeded with %#v", tt.runtime, target)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if target.URL.String() != tt.wantURL || target.Provenance != tt.wantProvenance {
				t.Fatalf("target = %#v, want URL %q provenance %q", target, tt.wantURL, tt.wantProvenance)
			}
		})
	}
}

func TestDocsReadinessAbsoluteTargetOutageCannotPassThroughInternalProbe(t *testing.T) {
	var requested string
	client := &http.Client{Transport: docsReadinessRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = request.URL.String()
		return nil, errors.New("external docs unavailable")
	})}
	cfg := config.Config{Runtime: config.RuntimeConfig{
		DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api", "gateway"},
		DocsURL: "https://docs.example.test/reference/", DocsProbeURL: "http://gateway/developer-docs/",
	}}
	result := newDocsReadinessChecker(cfg, client, 100*time.Millisecond)(context.Background())
	if result.Status != "fail" || requested != cfg.Runtime.DocsURL {
		t.Fatalf("readiness = %#v requested %q, want external target failure", result, requested)
	}
}

func TestDocsReadinessHTTPClientPinsValidatedAddresses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	serverAddress := server.Listener.Addr().String()
	publicIP := net.ParseIP("93.184.216.34")
	resolverCalls := 0
	resolver := docsReadinessResolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		resolverCalls++
		if host != "docs.example.test" {
			t.Fatalf("resolved host = %q", host)
		}
		if resolverCalls > 1 {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: publicIP}}, nil
	})
	var dialed string
	dialer := docsReadinessDialFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		dialed = address
		return (&net.Dialer{}).DialContext(ctx, network, serverAddress)
	})
	targetURL, _ := url.Parse("http://docs.example.test/developer-docs/")
	client := newDocsReadinessHTTPClient(200*time.Millisecond, docsReadinessResolvedTarget{
		URL: targetURL, Provenance: docsReadinessTargetConfiguredPublic,
	}, resolver, dialer)
	response, err := client.Get(targetURL.String())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if resolverCalls != 1 || dialed != "93.184.216.34:80" {
		t.Fatalf("resolver calls = %d, dialed = %q; want one lookup and pinned public IP", resolverCalls, dialed)
	}
}

func TestDocsReadinessHTTPClientRejectsUnsafePublicTargets(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		resolved []net.IPAddr
	}{
		{name: "literal private", target: "http://127.0.0.1/developer-docs/"},
		{name: "DNS private", target: "http://docs.example.test/developer-docs/", resolved: []net.IPAddr{{IP: net.ParseIP("10.0.0.8")}}},
		{name: "DNS CGNAT metadata", target: "http://docs.example.test/developer-docs/", resolved: []net.IPAddr{{IP: net.ParseIP("100.100.100.200")}}},
		{name: "mixed public and private", target: "http://docs.example.test/developer-docs/", resolved: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("169.254.169.254")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetURL, _ := url.Parse(tt.target)
			resolver := docsReadinessResolverFunc(func(context.Context, string) ([]net.IPAddr, error) { return tt.resolved, nil })
			dialer := docsReadinessDialFunc(func(context.Context, string, string) (net.Conn, error) {
				t.Fatal("unsafe address must be rejected before dial")
				return nil, nil
			})
			client := newDocsReadinessHTTPClient(100*time.Millisecond, docsReadinessResolvedTarget{
				URL: targetURL, Provenance: docsReadinessTargetConfiguredPublic,
			}, resolver, dialer)
			response, err := client.Get(tt.target)
			if response != nil {
				response.Body.Close()
			}
			if err == nil {
				t.Fatal("unsafe public target must fail")
			}
		})
	}
}

func TestDocsReadinessHTTPClientRevalidatesRedirectDNS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/developer-docs/" {
			http.Redirect(w, request, "/developer-docs/index.html", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	serverAddress := server.Listener.Addr().String()
	resolverCalls := 0
	resolver := docsReadinessResolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		resolverCalls++
		if resolverCalls == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	dialCalls := 0
	dialer := docsReadinessDialFunc(func(ctx context.Context, network, _ string) (net.Conn, error) {
		dialCalls++
		return (&net.Dialer{}).DialContext(ctx, network, serverAddress)
	})
	targetURL, _ := url.Parse("http://docs.example.test/developer-docs/")
	client := newDocsReadinessHTTPClient(200*time.Millisecond, docsReadinessResolvedTarget{URL: targetURL, Provenance: docsReadinessTargetConfiguredPublic}, resolver, dialer)
	response, err := client.Get(targetURL.String())
	if response != nil {
		response.Body.Close()
	}
	if err == nil || resolverCalls != 2 || dialCalls != 1 {
		t.Fatalf("redirect error = %v, resolver calls = %d, dial calls = %d; want second DNS policy rejection", err, resolverCalls, dialCalls)
	}
}

func TestDocsReadinessHTTPClientAllowsTrustedTopologyTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	serverAddress := server.Listener.Addr().String()
	tests := []struct {
		name     string
		target   string
		resolved []net.IPAddr
	}{
		{name: "Docker gateway", target: "http://gateway/developer-docs/", resolved: []net.IPAddr{{IP: net.ParseIP("172.18.0.8")}}},
		{name: "native loopback", target: "http://127.0.0.1:18000/developer-docs/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetURL, _ := url.Parse(tt.target)
			resolver := docsReadinessResolverFunc(func(context.Context, string) ([]net.IPAddr, error) { return tt.resolved, nil })
			dialer := docsReadinessDialFunc(func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, serverAddress)
			})
			client := newDocsReadinessHTTPClient(200*time.Millisecond, docsReadinessResolvedTarget{URL: targetURL, Provenance: docsReadinessTargetTrustedTopology}, resolver, dialer)
			response, err := client.Get(tt.target)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
		})
	}
}

func TestDocsReadinessCheckerProbesDeployedTarget(t *testing.T) {
	var requested *url.URL
	client := &http.Client{Transport: docsReadinessRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = request.URL
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	cfg := config.Config{Runtime: config.RuntimeConfig{
		DeploymentMode:    config.DeploymentModeDocker,
		DeploymentModules: []string{"api"},
		DocsURL:           "/developer-docs/",
		DocsProbeURL:      "https://studio.example.test/developer-docs/",
	}}

	result := newDocsReadinessChecker(cfg, client, 100*time.Millisecond)(context.Background())
	if result.Status != "pass" || !strings.Contains(result.Detail, "部署探测入口") || !strings.Contains(result.Detail, "HTTP 204") {
		t.Fatalf("readiness result = %#v, want deployed target pass", result)
	}
	if requested == nil || requested.String() != "https://studio.example.test/developer-docs/" {
		t.Fatalf("requested URL = %v", requested)
	}
}

func TestDocsReadinessCheckerReportsMissingProbeWithoutUsingPublicAPIOrigin(t *testing.T) {
	client := &http.Client{Transport: docsReadinessRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("relative docs target must not be probed through PUBLIC_API_URL: %s", request.URL)
		return nil, nil
	})}
	cfg := config.Config{Runtime: config.RuntimeConfig{
		DeploymentMode: config.DeploymentModeDocker, DeploymentModules: []string{"api"},
		PublicAPIURL: "https://api.example.test", DocsURL: "/developer-docs/",
	}}
	result := newDocsReadinessChecker(cfg, client, 100*time.Millisecond)(context.Background())
	if result.Status != "fail" || !strings.Contains(result.Detail, "未配置可探测文档地址") {
		t.Fatalf("readiness result = %#v, want missing probe diagnostic", result)
	}
}

func TestDocsReadinessCheckerRejectsUnhealthyTargetWithoutLeakingURL(t *testing.T) {
	secretTarget := "https://docs.example.test/?token=must-not-leak"
	tests := []struct {
		name      string
		runtime   config.RuntimeConfig
		transport docsReadinessRoundTripFunc
	}{
		{
			name:    "invalid target",
			runtime: config.RuntimeConfig{DocsURL: secretTarget},
			transport: func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid target must not be requested")
				return nil, nil
			},
		},
		{
			name:    "transport error",
			runtime: config.RuntimeConfig{DocsURL: "https://docs.example.test/"},
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial secret.internal:443 failed")
			},
		},
		{
			name:    "non 2xx",
			runtime: config.RuntimeConfig{DocsURL: "https://docs.example.test/"},
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("token=body-secret")), Header: make(http.Header)}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: tt.transport}
			result := newDocsReadinessChecker(config.Config{Runtime: tt.runtime}, client, 100*time.Millisecond)(context.Background())
			if result.Status != "fail" {
				t.Fatalf("readiness result = %#v, want failure", result)
			}
			for _, secret := range []string{"must-not-leak", "secret.internal", "body-secret", "docs.example.test"} {
				if strings.Contains(result.Detail, secret) {
					t.Fatalf("readiness detail leaked %q: %q", secret, result.Detail)
				}
			}
		})
	}
}

func TestDocsReadinessCheckerBoundsProbeTime(t *testing.T) {
	client := &http.Client{Transport: docsReadinessRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	started := time.Now()
	result := newDocsReadinessChecker(config.Config{Runtime: config.RuntimeConfig{DocsURL: "https://docs.example.test/"}}, client, 15*time.Millisecond)(context.Background())
	if result.Status != "fail" || !strings.Contains(result.Detail, "超时") {
		t.Fatalf("readiness result = %#v, want timeout failure", result)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("probe exceeded bounded timeout: %v", elapsed)
	}
}

func TestDocsReadinessHTTPClientRestrictsRedirects(t *testing.T) {
	targetURL, _ := url.Parse("https://docs.example.test/developer-docs/")
	client := newDocsReadinessHTTPClient(100*time.Millisecond, docsReadinessResolvedTarget{URL: targetURL, Provenance: docsReadinessTargetConfiguredPublic}, nil, nil)
	checkRedirect := client.CheckRedirect
	allowedTarget, _ := http.NewRequest(http.MethodGet, "https://docs.example.test/developer-docs/reference/", nil)
	allowedOrigin, _ := http.NewRequest(http.MethodGet, "https://docs.example.test/developer-docs/", nil)
	if err := checkRedirect(allowedTarget, []*http.Request{allowedOrigin}); err != nil {
		t.Fatalf("same-origin public redirect was rejected: %v", err)
	}
	for _, tt := range []struct {
		name   string
		target string
		origin string
	}{
		{name: "scheme change", target: "https://docs.example.test/", origin: "http://docs.example.test/"},
		{name: "port change", target: "https://docs.example.test:8443/", origin: "https://docs.example.test/"},
		{name: "cross host", target: "https://other.example.test/", origin: "https://docs.example.test/"},
		{name: "outside docs path", target: "https://docs.example.test/healthz", origin: "https://docs.example.test/developer-docs/"},
		{name: "encoded path traversal", target: "https://docs.example.test/developer-docs/%2e%2e/healthz", origin: "https://docs.example.test/developer-docs/"},
		{name: "credentials", target: "https://user:secret@docs.example.test/", origin: "https://docs.example.test/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target, _ := http.NewRequest(http.MethodGet, tt.target, nil)
			origin, _ := http.NewRequest(http.MethodGet, tt.origin, nil)
			if err := checkRedirect(target, []*http.Request{origin}); err == nil {
				t.Fatalf("redirect from %q to %q must be rejected", tt.origin, tt.target)
			}
		})
	}
}

func TestPublicGalleryDetailPreservesReusableCreationConfiguration(t *testing.T) {
	item := domainimagetask.GalleryImage{
		ID: "image-a", Prompt: "full prompt", TaskType: "image_edit", RouteModelCode: "plus",
		SizeMode: "pixel", RequestedSize: "1536x1024", BaseResolution: "2k", AspectRatio: "3:2",
		Quality: "high", OutputFormat: "webp", OutputCompression: 72, Moderation: "low", OutputImageCount: 4,
	}
	detail := publicGalleryDetailItem(item)
	if detail.SizeMode != item.SizeMode || detail.RequestedSize != item.RequestedSize || detail.OutputFormat != item.OutputFormat || detail.OutputCompression != item.OutputCompression || detail.Moderation != item.Moderation || detail.OutputImageCount != item.OutputImageCount {
		t.Fatalf("public detail lost reusable creation configuration: %#v", detail)
	}
}

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

func TestOpenAPIRequestBodyLimitAccountsForUploadEncoding(t *testing.T) {
	const imageBytes int64 = 20 * 1024 * 1024
	jsonLimit := openAPIRequestBodyLimit("/api/open/image/v1/reference-assets/uploads", "application/json", imageBytes)
	wantJSONMinimum := int64(base64.StdEncoding.EncodedLen(int(imageBytes))) + referenceAssetMultipartOverheadBytes
	if jsonLimit < wantJSONMinimum {
		t.Fatalf("JSON upload limit %d cannot carry 20 MB base64 payload; want at least %d", jsonLimit, wantJSONMinimum)
	}
	multipartLimit := openAPIRequestBodyLimit("/api/open/image/v1/reference-assets", "multipart/form-data; boundary=test", imageBytes)
	if multipartLimit != imageBytes+referenceAssetMultipartOverheadBytes {
		t.Fatalf("multipart upload limit = %d, want %d", multipartLimit, imageBytes+referenceAssetMultipartOverheadBytes)
	}
}

func TestOpenAPIRequestBodyLimitClampsPersistedImagePolicy(t *testing.T) {
	const unsafePersistedLimit = int64(10 * 1024 * 1024 * 1024)
	got := openAPIRequestBodyLimit("/api/open/image/v1/reference-assets/uploads", "application/json", unsafePersistedLimit)
	want := int64(base64.StdEncoding.EncodedLen(assetservice.MaxImageAttachmentSizeMB*1024*1024)) + referenceAssetMultipartOverheadBytes
	if got != want {
		t.Fatalf("OpenAPI body limit = %d, want hard-clamped %d", got, want)
	}
}

func TestPromptExcerptNeverReturnsFullPrompt(t *testing.T) {
	for _, prompt := range []string{
		"short prompt",
		"Generate a downloadable banner",
		"生成一张适合电商首页的明亮横幅图",
	} {
		excerpt := promptExcerpt(prompt, 24)
		if excerpt == "" {
			t.Fatalf("expected excerpt for %q", prompt)
		}
		if excerpt == prompt {
			t.Fatalf("excerpt should not expose full prompt %q", prompt)
		}
		if len([]rune(excerpt)) > 24 {
			t.Fatalf("excerpt should be capped at 24 runes, got %d for %q", len([]rune(excerpt)), excerpt)
		}
	}
}

func TestNormalizeCashierQueryStatusMapsProviderTerminalStates(t *testing.T) {
	tests := []struct {
		name             string
		status           string
		wantStatus       string
		wantPaid         bool
		wantMessage      string
		wantRiskCategory string
		wantActionHint   string
	}{
		{name: "alipay trade success", status: "TRADE_SUCCESS", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付", wantRiskCategory: "paid", wantActionHint: "核对本地到账"},
		{name: "alipay waiting buyer pay", status: "WAIT_BUYER_PAY", wantStatus: "pending", wantPaid: false, wantMessage: "渠道订单未支付或仍在处理中", wantRiskCategory: "pending", wantActionHint: "稍后可再次查单"},
		{name: "alipay trade closed", status: "TRADE_CLOSED", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭", wantRiskCategory: "closed", wantActionHint: "重新创建订单"},
		{name: "wxpay revoked", status: "REVOKED", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭", wantRiskCategory: "closed", wantActionHint: "重新创建订单"},
		{name: "wxpay refund", status: "REFUND", wantStatus: "refunded", wantPaid: false, wantMessage: "渠道订单已退款", wantRiskCategory: "refunded", wantActionHint: "本地退款流水"},
		{name: "wxpay pay error", status: "PAYERROR", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单支付失败", wantRiskCategory: "channel_error", wantActionHint: "商户后台"},
		{name: "easypay paid", status: "1", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付", wantRiskCategory: "paid", wantActionHint: "核对本地到账"},
		{name: "easypay pending", status: "0", wantStatus: "pending", wantPaid: false, wantMessage: "渠道订单未支付或仍在处理中", wantRiskCategory: "pending", wantActionHint: "稍后可再次查单"},
		{name: "jeepay success", status: "2", wantStatus: "paid", wantPaid: true, wantMessage: "渠道订单已支付", wantRiskCategory: "paid", wantActionHint: "核对本地到账"},
		{name: "jeepay closed", status: "3", wantStatus: "closed", wantPaid: false, wantMessage: "渠道订单已关闭", wantRiskCategory: "closed", wantActionHint: "重新创建订单"},
		{name: "jeepay failed", status: "4", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单支付失败", wantRiskCategory: "channel_error", wantActionHint: "商户后台"},
		{name: "provider risk", status: "risk_control", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单被风控拦截", wantRiskCategory: "risk_control", wantActionHint: "更换支付渠道"},
		{name: "provider limited", status: "limited", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单触发限额限制", wantRiskCategory: "channel_limited", wantActionHint: "切换备用渠道"},
		{name: "provider signature", status: "sign_error", wantStatus: "failed", wantPaid: false, wantMessage: "渠道验签或签名配置异常", wantRiskCategory: "signature_error", wantActionHint: "检查商户密钥"},
		{name: "provider amount mismatch", status: "amount_mismatch", wantStatus: "failed", wantPaid: false, wantMessage: "渠道订单金额与本地订单不一致", wantRiskCategory: "amount_mismatch", wantActionHint: "暂停到账"},
		{name: "provider account abnormal", status: "merchant_disabled", wantStatus: "failed", wantPaid: false, wantMessage: "渠道商户账号状态异常", wantRiskCategory: "account_abnormal", wantActionHint: "切换备用账号"},
		{name: "provider timeout", status: "timeout", wantStatus: "failed", wantPaid: false, wantMessage: "渠道查单超时或网络异常", wantRiskCategory: "channel_timeout", wantActionHint: "稍后重试"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cashierservice.NormalizeQueryStatus(tt.status)
			if got.Status != tt.wantStatus || got.Paid != tt.wantPaid || got.Message != tt.wantMessage || got.RiskCategory != tt.wantRiskCategory || !strings.Contains(got.ActionHint, tt.wantActionHint) {
				t.Fatalf("NormalizeQueryStatus(%q)=%#v, want status=%q paid=%v message=%q risk_category=%q action_hint containing %q", tt.status, got, tt.wantStatus, tt.wantPaid, tt.wantMessage, tt.wantRiskCategory, tt.wantActionHint)
			}
		})
	}
}
