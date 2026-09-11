package mgsctl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveLatestReleasePinsConcreteVersionAndDigests(t *testing.T) {
	manifest := validReleaseManifestForTest()
	server := releaseManifestServer(t, manifest, "")
	defer server.Close()

	resolved, err := ResolveReleaseManifest(context.Background(), ReleaseManifestOptions{
		ReleaseBaseURL: server.URL + "/releases",
		Version:        "latest",
		Components:     []Component{ComponentAPI, ComponentWorker, ComponentGateway},
	}, ReleaseManifestDependencies{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("ResolveReleaseManifest: %v", err)
	}
	if resolved.ApplicationVersion != "v1.2.3" || resolved.Commit != strings.Repeat("a", 40) {
		t.Fatalf("resolved identity = %#v", resolved)
	}
	if len(resolved.Images) != 2 {
		t.Fatalf("resolved images = %#v, want selected application images only", resolved.Images)
	}
	for component, image := range resolved.Images {
		if image.Tag != "v1.2.3" || !strings.HasPrefix(image.Digest, "sha256:") {
			t.Fatalf("resolved image %s = %#v", component, image)
		}
	}
	if resolved.MigrationImage.Repository != "docker.io/fatballfish/mikiko-gallery-studio-api" {
		t.Fatalf("migration image = %#v", resolved.MigrationImage)
	}
	if resolved.Assets["mgsctl-linux-amd64"].SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("resolved assets = %#v", resolved.Assets)
	}
}

func TestResolveReleaseUsesConfiguredEnvironmentBaseURL(t *testing.T) {
	manifest := validReleaseManifestForTest()
	server := releaseManifestServer(t, manifest, "")
	defer server.Close()
	t.Setenv("MGSCTL_RELEASE_BASE_URL", server.URL+"/releases")

	resolved, err := ResolveReleaseManifest(context.Background(), ReleaseManifestOptions{
		Version: "latest", Components: []Component{ComponentAPI},
	}, ReleaseManifestDependencies{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("ResolveReleaseManifest: %v", err)
	}
	if resolved.ApplicationVersion != manifest.ApplicationVersion {
		t.Fatalf("resolved version = %q, want %q", resolved.ApplicationVersion, manifest.ApplicationVersion)
	}
}

func TestResolveReleaseRejectsChecksumVersionAndComponentDrift(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*ReleaseManifest)
		checksum  string
		selector  string
		wantError string
	}{
		{name: "checksum mismatch", checksum: strings.Repeat("0", 64), selector: "latest", wantError: "checksum mismatch"},
		{name: "selector mismatch", selector: "v1.2.4", wantError: "does not match requested version"},
		{name: "image version drift", selector: "latest", mutate: func(manifest *ReleaseManifest) {
			image := manifest.Images[string(ComponentWorker)]
			image.Version = "v1.2.4"
			manifest.Images[string(ComponentWorker)] = image
		}, wantError: "worker image version"},
		{name: "invalid digest", selector: "latest", mutate: func(manifest *ReleaseManifest) {
			image := manifest.Images[string(ComponentDocsWeb)]
			image.Digest = "latest"
			manifest.Images[string(ComponentDocsWeb)] = image
		}, wantError: "docs-web image digest"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validReleaseManifestForTest()
			if test.mutate != nil {
				test.mutate(&manifest)
			}
			server := releaseManifestServer(t, manifest, test.checksum)
			defer server.Close()
			_, err := ResolveReleaseManifest(context.Background(), ReleaseManifestOptions{
				ReleaseBaseURL: server.URL + "/releases", Version: test.selector,
				Components: []Component{ComponentAPI, ComponentWorker},
			}, ReleaseManifestDependencies{HTTPClient: server.Client()})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ResolveReleaseManifest error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestResolveReleaseRejectsUnknownSchemaAndMissingMigrationImage(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*ReleaseManifest)
		wantError string
	}{
		{name: "unknown schema", mutate: func(manifest *ReleaseManifest) { manifest.SchemaVersion = 2 }, wantError: "schema version"},
		{name: "missing migration image", mutate: func(manifest *ReleaseManifest) { delete(manifest.Images, string(ComponentAPI)) }, wantError: "migration image"},
		{name: "invalid asset checksum", mutate: func(manifest *ReleaseManifest) {
			asset := manifest.Assets["mgsctl-linux-amd64"]
			asset.SHA256 = "bad"
			manifest.Assets["mgsctl-linux-amd64"] = asset
		}, wantError: "asset checksum"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validReleaseManifestForTest()
			test.mutate(&manifest)
			server := releaseManifestServer(t, manifest, "")
			defer server.Close()
			_, err := ResolveReleaseManifest(context.Background(), ReleaseManifestOptions{
				ReleaseBaseURL: server.URL + "/releases", Version: "latest", Components: []Component{ComponentWorker},
			}, ReleaseManifestDependencies{HTTPClient: server.Client()})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ResolveReleaseManifest error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func validReleaseManifestForTest() ReleaseManifest {
	images := make(map[string]ReleaseImage, 5)
	for index, component := range []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb} {
		images[string(component)] = ReleaseImage{
			Repository: "docker.io/fatballfish/mikiko-gallery-studio-" + string(component),
			Tag:        "v1.2.3", Digest: "sha256:" + strings.Repeat(fmt.Sprintf("%x", index+1), 64),
			Version: "v1.2.3", Revision: strings.Repeat("a", 40),
		}
	}
	return ReleaseManifest{
		SchemaVersion: 1, ApplicationVersion: "v1.2.3", Commit: strings.Repeat("a", 40),
		Images: images,
		Assets: map[string]ReleaseAsset{
			"mgsctl-linux-amd64": {Name: "mgsctl-linux-amd64", SHA256: strings.Repeat("b", 64)},
		},
	}
}

func releaseManifestServer(t *testing.T, manifest ReleaseManifest, checksumOverride string) *httptest.Server {
	t.Helper()
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	checksum := hex.EncodeToString(digest[:])
	if checksumOverride != "" {
		checksum = checksumOverride
	}
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/release-manifest.json.sha256"):
			_, _ = fmt.Fprintf(writer, "%s  release-manifest.json\n", checksum)
		case strings.HasSuffix(request.URL.Path, "/release-manifest.json"):
			_, _ = writer.Write(content)
		default:
			http.NotFound(writer, request)
		}
	}))
}

func TestResolveLatestReleasePrefersNewestManifestCarryingRelease(t *testing.T) {
	previous := githubAPIRoot
	t.Cleanup(func() { githubAPIRoot = previous })

	manifest := validReleaseManifestForTest()
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	checksum := hex.EncodeToString(digest[:])

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/repos/owner/repo/releases":
			if request.Header.Get("Accept") != "application/vnd.github+json" {
				t.Errorf("releases API accept header = %q", request.Header.Get("Accept"))
			}
			_, _ = fmt.Fprintf(writer, `[
				{"draft": false, "assets": [{"name": "mgsctl-linux-amd64", "browser_download_url": %q}]},
				{"draft": false, "assets": [{"name": "release-manifest.json", "browser_download_url": %q}]},
				{"draft": false, "assets": [{"name": "release-manifest.json", "browser_download_url": %q}]}
			]`,
				server.URL+"/releases/download/mgsctl-v0.2.0/mgsctl-linux-amd64",
				server.URL+"/releases/download/v1.2.3/release-manifest.json",
				server.URL+"/releases/download/v1.1.0/release-manifest.json")
		case request.URL.Path == "/releases/download/v1.2.3/release-manifest.json.sha256":
			_, _ = fmt.Fprintf(writer, "%s  release-manifest.json\n", checksum)
		case request.URL.Path == "/releases/download/v1.2.3/release-manifest.json":
			_, _ = writer.Write(content)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	githubAPIRoot = server.URL

	resolved, err := ResolveReleaseManifest(context.Background(), ReleaseManifestOptions{
		ReleaseBaseURL: "https://github.com/owner/repo/releases",
		Version:        "latest",
		Components:     []Component{ComponentAPI},
	}, ReleaseManifestDependencies{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("ResolveReleaseManifest: %v", err)
	}
	if resolved.ApplicationVersion != "v1.2.3" {
		t.Fatalf("resolved version = %q, want the newest manifest-carrying release", resolved.ApplicationVersion)
	}
}

func TestResolveLatestReleaseManifestURLRequiresManifestCarryingRelease(t *testing.T) {
	previous := githubAPIRoot
	t.Cleanup(func() { githubAPIRoot = previous })

	manifest := validReleaseManifestForTest()
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	checksum := hex.EncodeToString(digest[:])

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/repos/owner/repo/releases":
			if request.URL.Query().Get("scenario") == "" {
				_, _ = writer.Write([]byte(`[
					{"draft": true, "assets": [{"name": "release-manifest.json", "browser_download_url": "https://example.invalid/m.json"}]},
					{"draft": false, "assets": [{"name": "mgsctl-linux-amd64", "browser_download_url": "https://example.invalid/m"}]}
				]`))
				return
			}
			http.Error(writer, "boom", http.StatusInternalServerError)
		case strings.HasSuffix(request.URL.Path, "/release-manifest.json.sha256"):
			_, _ = fmt.Fprintf(writer, "%s  release-manifest.json\n", checksum)
		case strings.HasSuffix(request.URL.Path, "/release-manifest.json"):
			_, _ = writer.Write(content)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	githubAPIRoot = server.URL
	client := server.Client()
	const base = "https://github.com/owner/repo/releases"

	if url := resolveLatestReleaseManifestURL(context.Background(), client, base); url != "" {
		t.Fatalf("latest manifest URL = %q, want empty when no release carries the manifest", url)
	}
	if url := githubReleasesAPIURL(base); url != server.URL+"/repos/owner/repo/releases?per_page=100" {
		t.Fatalf("releases API URL = %q", url)
	}
	if url := githubReleasesAPIURL("https://example.com/owner/repo/releases"); url != "" {
		t.Fatalf("non-github base should skip the API, got %q", url)
	}
	if url := githubReleasesAPIURL("https://github.com/owner/repo/releases/v1.2.3"); url != "" {
		t.Fatalf("unexpected release base shape should skip the API, got %q", url)
	}

	var manifestServer *httptest.Server
	manifestServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/repos/owner/repo/releases":
			_, _ = fmt.Fprintf(writer, `[
				{"draft": false, "assets": [{"name": "release-manifest.json", "browser_download_url": %q}]}
			]`, manifestServer.URL+"/releases/download/v2.0.0/release-manifest.json")
		case strings.HasSuffix(request.URL.Path, "/release-manifest.json"):
			_, _ = writer.Write(content)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer manifestServer.Close()
	githubAPIRoot = manifestServer.URL
	if url := resolveLatestReleaseManifestURL(context.Background(), manifestServer.Client(), base); url == "" {
		t.Fatal("latest manifest URL was empty although a release carries the manifest")
	}
}

func TestResolveLatestReleaseSkipsGitHubAPIForOtherHosts(t *testing.T) {
	previous := githubAPIRoot
	t.Cleanup(func() { githubAPIRoot = previous })

	manifest := validReleaseManifestForTest()
	apiRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/repos/") {
			apiRequests++
			t.Errorf("releases API was queried for a non-github base URL: %s", request.URL.Path)
			http.NotFound(writer, request)
			return
		}
		switch {
		case strings.HasSuffix(request.URL.Path, "/release-manifest.json.sha256"):
			content, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(content)
			_, _ = fmt.Fprintf(writer, "%s  release-manifest.json\n", hex.EncodeToString(digest[:]))
		case strings.HasSuffix(request.URL.Path, "/release-manifest.json"):
			content, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write(content)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	githubAPIRoot = server.URL

	resolved, err := ResolveReleaseManifest(context.Background(), ReleaseManifestOptions{
		ReleaseBaseURL: server.URL + "/releases",
		Version:        "latest",
		Components:     []Component{ComponentAPI},
	}, ReleaseManifestDependencies{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("ResolveReleaseManifest: %v", err)
	}
	if apiRequests != 0 || resolved.ApplicationVersion != "v1.2.3" {
		t.Fatalf("non-github resolution = %#v apiRequests=%d", resolved, apiRequests)
	}
}
