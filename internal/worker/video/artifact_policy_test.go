package video

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
)

func TestArtifactHostPolicyCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name      string
		extra     map[string]any
		wantHosts int
		wantFlag  int
	}{
		{"new account", nil, 0, -1},
		{"legacy hosts", map[string]any{"video_artifact_hosts": []any{" cdn.example.com "}}, 1, -1},
		{"disabled preserves hosts", map[string]any{"video_artifact_hosts": []string{"cdn.example.com"}, "video_artifact_hosts_enabled": false}, 1, 0},
		{"enabled empty", map[string]any{"video_artifact_hosts_enabled": true}, 0, 1},
		{"invalid flag fails closed", map[string]any{"video_artifact_hosts_enabled": "false"}, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hosts, flag := ArtifactHostPolicyFromExtra(tc.extra)
			if len(hosts) != tc.wantHosts || (flag == nil) != (tc.wantFlag < 0) {
				t.Fatalf("hosts=%v flag=%v", hosts, flag)
			}
			if flag != nil && *flag != (tc.wantFlag == 1) {
				t.Fatalf("flag=%v", *flag)
			}
		})
	}
}

func TestArtifactHostSwitchDoesNotDisableAddressProtection(t *testing.T) {
	disabled, enabled := false, true
	runner := NewRunner(nil, nil, nil, Options{ArtifactAllowedHosts: []string{"global.example.com"}, ResolveHostIPs: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }})
	for _, tc := range []struct {
		name    string
		hosts   []string
		flag    *bool
		host    string
		allowed bool
	}{
		{"disabled permits other public host", []string{"cdn.example.com"}, &disabled, "other.example.com", true},
		{"new default permits public host", nil, nil, "other.example.com", false},
		{"legacy list rejects mismatch", []string{"cdn.example.com"}, nil, "other.example.com", false},
		{"explicit enabled empty rejects global", nil, &enabled, "global.example.com", false},
		{"explicit enabled does not inherit global", []string{"cdn.example.com"}, &enabled, "global.example.com", false},
		{"exact match", []string{"cdn.example.com"}, &enabled, "CDN.example.com", true},
		{"no subdomain wildcard", []string{"cdn.example.com"}, &enabled, "sub.cdn.example.com", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runner.resolveArtifactHost(t.Context(), tc.host, tc.hosts, tc.flag)
			if (err == nil) != tc.allowed {
				t.Fatalf("err=%v", err)
			}
		})
	}
	runner.options.ArtifactAllowedHosts = nil
	if _, err := runner.resolveArtifactHost(t.Context(), "new.example.com", nil); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "::1", "fc00::1"} {
		runner.options.ResolveHostIPs = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP(ip)}, nil }
		if _, err := runner.resolveArtifactHost(t.Context(), "anything.example.com", nil, &disabled); err == nil {
			t.Fatalf("disabled allowed %s", ip)
		}
	}
}

func TestDisabledArtifactHostsRetainsHTTPSAndRedirectProtection(t *testing.T) {
	disabled := false
	for _, target := range []string{"http://public.example.com/file.mp4", "https://169.254.169.254/file.mp4", "https://10.0.0.1/file.mp4"} {
		t.Run(target, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target, http.StatusFound) }))
			defer server.Close()
			runner := NewRunner(nil, nil, nil, Options{HTTPClient: server.Client(), AllowLoopbackArtifactHosts: true})
			if _, _, _, _, err := runner.downloadArtifact(t.Context(), providervideo.Artifact{URL: server.URL}, nil, &disabled); err == nil {
				t.Fatal("worker accepted unsafe redirect with whitelist disabled")
			}
			if err := runner.ValidateArtifact(t.Context(), providervideo.Artifact{URL: server.URL}, nil, &disabled); err == nil {
				t.Fatal("unsafe redirect accepted")
			}
		})
	}
	runner := NewRunner(nil, nil, nil, Options{})
	if err := runner.ValidateArtifact(t.Context(), providervideo.Artifact{URL: "http://8.8.8.8/video.mp4"}, nil, &disabled); err == nil {
		t.Fatal("HTTP accepted")
	}
}

func TestAdminArtifactProbeDoesNotDownloadFullVideo(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Range") != "bytes=0-0" {
			t.Errorf("method=%s range=%s", r.Method, r.Header.Get("Range"))
		}
		// Simulate a provider ignoring Range and streaming a large, slow video.
		w.Header().Set("Content-Length", "1000000000")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	disabled := false
	runner := NewRunner(nil, nil, nil, Options{HTTPClient: server.Client(), AllowLoopbackArtifactHosts: true})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := runner.ValidateArtifact(ctx, providervideo.Artifact{URL: server.URL}, nil, &disabled); err != nil {
		t.Fatal(err)
	}
}
