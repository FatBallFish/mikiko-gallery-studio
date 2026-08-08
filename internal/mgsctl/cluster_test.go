package mgsctl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestExecuteClusterJoinEncryptsHTTPAndPublishesCompletedRuntimeBeforeDeployment(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	installationID := "019d0000-0000-7000-8000-000000000951"
	store := clusterservice.NewMemoryStore(domaincluster.Installation{
		InstallationID: installationID, Initialized: true, ApplicationVersion: "v1", RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 9,
	})
	service := clusterservice.NewService(clusterservice.ServiceOptions{
		Store: store, InstallationID: installationID, DeploymentRole: domaincluster.NodeRoleControl,
		Now: func() time.Time { return now }, Entropy: bytes.NewReader(bytes.Repeat([]byte{0x81}, 8192)),
		EnrollmentSealKey: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x82}, 32)),
		RuntimeValues: map[string]string{
			"DATABASE_URL": "postgres://app:database-secret@db:5432/app", "REDIS_URL": "redis://:redis-secret@redis:6379/0", "REDIS_KEY_PREFIX": "app",
			"STORAGE_DRIVER": "s3", "STORAGE_S3_ENDPOINT": "http://minio:9000", "STORAGE_S3_REGION": "us-east-1", "STORAGE_S3_BUCKET": "assets",
			"STORAGE_S3_ACCESS_KEY_ID": "access", "STORAGE_S3_SECRET_ACCESS_KEY": "storage-secret", "STORAGE_S3_FORCE_PATH_STYLE": "true",
			"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "secure-secret", "SETUP_TOKEN": "setup-secret-must-not-leave-control",
		},
	})
	issued, err := service.CreateToken(t.Context(), domaincluster.CreateTokenRequest{Role: domaincluster.JoinRoleWorker, TTL: time.Hour, ActorID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	requestBodies := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var result any
		var handlerErr error
		switch r.URL.Path {
		case "/api/open/cluster/v1/challenges":
			var request domaincluster.CreateChallengeRequest
			handlerErr = json.NewDecoder(r.Body).Decode(&request)
			requestBodies = append(requestBodies, mustEncodeJoinTest(t, request))
			if handlerErr == nil {
				result, handlerErr = service.CreateChallenge(r.Context(), request)
			}
		case "/api/open/cluster/v1/join":
			var request domaincluster.JoinRequest
			handlerErr = json.NewDecoder(r.Body).Decode(&request)
			requestBodies = append(requestBodies, mustEncodeJoinTest(t, request))
			if handlerErr == nil {
				result, handlerErr = service.Join(r.Context(), request)
			}
		default:
			http.NotFound(w, r)
			return
		}
		if handlerErr != nil {
			http.Error(w, handlerErr.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": result})
	}))
	defer server.Close()

	runtimeDir := t.TempDir()
	events := make([]string, 0, 2)
	result, err := ExecuteClusterJoin(t.Context(), ClusterJoinOptions{
		Server: server.URL, Token: issued.Credential, RuntimeDir: runtimeDir,
		Mode: config.DeploymentModeDocker, ApplicationVersion: "v1", ImageTag: "v1",
	}, ClusterJoinDependencies{
		HTTPClient: server.Client(), Entropy: bytes.NewReader(bytes.Repeat([]byte{0x83}, 4096)), Now: func() time.Time { return now },
		PreflightDeployment: func(context.Context, InstallPlan) error { events = append(events, "preflight"); return nil },
		ApplyDeployment: func(_ context.Context, plan InstallPlan) error {
			if _, err := os.Stat(filepath.Join(runtimeDir, "config", "runtime.env")); err != nil {
				t.Fatalf("deployment started before runtime publication: %v", err)
			}
			events = append(events, "apply:"+string(plan.Role))
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Role != config.DeploymentRoleWorker || strings.Join(events, ",") != "preflight,apply:worker" {
		t.Fatalf("join result=%#v events=%v", result, events)
	}
	for _, body := range requestBodies {
		if strings.Contains(body, issued.Credential) || strings.Contains(body, "database-secret") || strings.Contains(body, "redis-secret") {
			t.Fatalf("cluster join HTTP exposed secret: %s", body)
		}
	}
	runtimeContent, err := os.ReadFile(filepath.Join(runtimeDir, "config", "runtime.env"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.ParseRuntimeEnv(runtimeContent)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"DEPLOYMENT_MODE": "docker", "DEPLOYMENT_PROFILE": "core", "DEPLOYMENT_TOPOLOGY": "cluster", "DEPLOYMENT_ROLE": "worker",
		"DEPLOYMENT_MODULES": "worker", "SETUP_COMPLETED": "true", "INSTALLATION_ID": installationID,
		"APPLICATION_VERSION": "v1", "DATABASE_URL": "postgres://app:database-secret@db:5432/app",
	} {
		if document.Values[key] != want {
			t.Errorf("runtime %s = %q, want %q", key, document.Values[key], want)
		}
	}
	for _, forbidden := range []string{"SETUP_TOKEN", "CLUSTER_ENROLLMENT_SEAL_KEY", "AUTH_ACCESS_TOKEN_SECRET"} {
		if document.Values[forbidden] != "" {
			t.Errorf("joined Worker runtime contains forbidden %s", forbidden)
		}
	}
	stateContent, err := os.ReadFile(filepath.Join(runtimeDir, "config", "install-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state setup.InstallState
	if err := json.Unmarshal(stateContent, &state); err != nil || state.Validate() != nil || state.Phase != setup.InstallPhaseCompleted || state.DeploymentRole != config.DeploymentRoleWorker {
		t.Fatalf("joined install state = %#v, decode=%v validate=%v", state, err, state.Validate())
	}
}

func TestExecuteClusterJoinRejectsExistingTargetBeforeNetwork(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runtimeDir, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "config", "runtime.env"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	_, err := ExecuteClusterJoin(t.Context(), ClusterJoinOptions{
		Server: "http://127.0.0.1:1", Token: "pgjoin.v1." + uuid.NewString() + "." + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)),
		RuntimeDir: runtimeDir, Mode: config.DeploymentModeDocker, ApplicationVersion: "v1", ImageTag: "v1",
	}, ClusterJoinDependencies{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { requests++; return nil, context.Canceled })}})
	if err == nil || requests != 0 {
		t.Fatalf("existing target error=%v requests=%d", err, requests)
	}
}

func TestJoinedRuntimeUsesNodeProbeAndDropsControlNodeLocalProbe(t *testing.T) {
	joined := domaincluster.JoinResponse{
		RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion,
		InstallationID:       "019d0000-0000-7000-8000-000000000321",
		NodeID:               "019d0000-0000-7000-8000-000000000322",
		ConfigRevision:       3,
		ApplicationVersion:   "v1",
	}
	remote := map[string]string{
		"PIC_GALLERY_DOCS_URL":       "/developer-docs/",
		"PIC_GALLERY_DOCS_PROBE_URL": "http://gateway/developer-docs/",
	}
	plan := InstallPlan{
		Mode: config.DeploymentModeDocker, Role: config.DeploymentRoleAPI,
		Components: []Component{ComponentAPI}, DocsProbeURL: "https://gateway.node.example.test/developer-docs/",
	}
	values := joinedRuntimeValues(ClusterJoinOptions{ImageTag: "v1"}, plan, joined, remote)
	if values["PIC_GALLERY_DOCS_PROBE_URL"] != plan.DocsProbeURL {
		t.Fatalf("joined probe URL = %q, want node-local %q", values["PIC_GALLERY_DOCS_PROBE_URL"], plan.DocsProbeURL)
	}

	plan.DocsProbeURL = ""
	values = joinedRuntimeValues(ClusterJoinOptions{ImageTag: "v1"}, plan, joined, remote)
	if _, exists := values["PIC_GALLERY_DOCS_PROBE_URL"]; exists {
		t.Fatalf("joined API inherited control node local probe: %#v", values)
	}
}

func TestPostClusterJSONRejectsRedirectsAndTrailingOuterJSON(t *testing.T) {
	t.Run("redirect", func(t *testing.T) {
		redirected := false
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			redirected = true
		}))
		defer target.Close()
		server := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusTemporaryRedirect))
		defer server.Close()
		var response map[string]any
		err := postClusterJSON(t.Context(), server.Client(), server.URL, map[string]string{"value": "x"}, &response)
		if err == nil || redirected {
			t.Fatalf("redirect error=%v redirected=%t", err, redirected)
		}
	})

	t.Run("trailing outer JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"value":"ok"}} {"data":{"value":"smuggled"}}`)
		}))
		defer server.Close()
		var response struct {
			Value string `json:"value"`
		}
		err := postClusterJSON(t.Context(), server.Client(), server.URL, map[string]string{"value": "x"}, &response)
		if err == nil {
			t.Fatal("accepted more than one outer JSON object")
		}
	})
}

func TestPostClusterJSONAcceptsTheStandardHTTPEnvelopeAndRejectsUnknownOuterFields(t *testing.T) {
	t.Run("standard metadata", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"value":"ok"},"meta":{"request_id":"request-1"}}`)
		}))
		defer server.Close()
		var response struct {
			Value string `json:"value"`
		}
		if err := postClusterJSON(t.Context(), server.Client(), server.URL, map[string]string{"value": "x"}, &response); err != nil || response.Value != "ok" {
			t.Fatalf("standard response envelope value=%q error=%v", response.Value, err)
		}
	})

	t.Run("unknown outer field", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"value":"ok"},"unexpected":true}`)
		}))
		defer server.Close()
		var response map[string]any
		if err := postClusterJSON(t.Context(), server.Client(), server.URL, map[string]string{"value": "x"}, &response); err == nil {
			t.Fatal("accepted an unknown outer response field")
		}
	})
}

func TestValidateJoinResponseUsesTheProtocolTimestampPrecision(t *testing.T) {
	expiresAt := time.Date(2026, 7, 23, 10, 1, 0, 987654321, time.UTC)
	challenge := domaincluster.EnrollmentChallenge{
		Protocol: clusterservice.EnrollmentProtocolV1, InstallationID: "installation-1", NodeID: "node-1",
		Role: domaincluster.JoinRoleAPI, ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 2,
		ExpiresAt: expiresAt,
	}
	joined := domaincluster.JoinResponse{
		Protocol: challenge.Protocol, InstallationID: challenge.InstallationID, NodeID: challenge.NodeID,
		Role: domaincluster.NodeRoleAPI, ApplicationVersion: challenge.ApplicationVersion,
		RuntimeSchemaVersion: challenge.RuntimeSchemaVersion, ConfigRevision: challenge.ConfigRevision,
		ExpiresAt: expiresAt.Truncate(time.Microsecond),
	}
	if err := validateJoinResponse(joined, challenge, expiresAt.Add(-time.Minute)); err != nil {
		t.Fatalf("same protocol second was rejected after database timestamp normalization: %v", err)
	}
	joined.ExpiresAt = expiresAt.Add(time.Second)
	if err := validateJoinResponse(joined, challenge, expiresAt.Add(-time.Minute)); err == nil {
		t.Fatal("join response accepted an expiry from a different protocol second")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func mustEncodeJoinTest(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
