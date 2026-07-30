package mgsctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	ReleaseManifestSchemaVersion = 1
	releaseManifestName          = "release-manifest.json"
	maxReleaseManifestSize       = int64(1 << 20)
)

var releaseVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type ReleaseManifest struct {
	SchemaVersion      int                     `json:"schema_version"`
	ApplicationVersion string                  `json:"application_version"`
	Commit             string                  `json:"commit"`
	Images             map[string]ReleaseImage `json:"images"`
	Assets             map[string]ReleaseAsset `json:"assets"`
}

type ReleaseImage struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Digest     string `json:"digest"`
	Version    string `json:"version"`
	Revision   string `json:"revision"`
}

type ReleaseAsset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type ReleaseManifestOptions struct {
	ReleaseBaseURL string
	Version        string
	Components     []Component
}

type ReleaseManifestDependencies struct {
	HTTPClient *http.Client
}

type ResolvedRelease struct {
	ApplicationVersion string
	Commit             string
	Images             map[Component]ReleaseImage
	MigrationImage     ReleaseImage
	Assets             map[string]ReleaseAsset
	ManifestURL        string
}

func ResolveReleaseManifest(ctx context.Context, options ReleaseManifestOptions, dependencies ReleaseManifestDependencies) (ResolvedRelease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ResolvedRelease{}, err
	}
	baseURL := strings.TrimSpace(options.ReleaseBaseURL)
	if baseURL == "" {
		baseURL = DefaultMGSCTLReleaseBaseURL
	}
	if err := validateMGSCTLDownloadURL(baseURL, true); err != nil {
		return ResolvedRelease{}, fmt.Errorf("release manifest base URL: %w", err)
	}
	selector := strings.TrimSpace(options.Version)
	if selector == "" {
		selector = "latest"
	}
	if selector != "latest" && !releaseVersionPattern.MatchString(selector) {
		return ResolvedRelease{}, fmt.Errorf("release selector must be latest or a vX.Y.Z version")
	}
	client := dependencies.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	releasePath := "latest/download"
	if selector != "latest" {
		releasePath = "download/" + url.PathEscape(selector)
	}
	manifestURL := strings.TrimRight(baseURL, "/") + "/" + releasePath + "/" + releaseManifestName
	checksumContent, _, err := downloadBytes(ctx, client, manifestURL+".sha256", maxMGSCTLChecksumSize)
	if err != nil {
		return ResolvedRelease{}, fmt.Errorf("download release manifest checksum: %w", err)
	}
	expectedChecksum, err := parseMGSCTLChecksum(checksumContent)
	if err != nil {
		return ResolvedRelease{}, fmt.Errorf("parse release manifest checksum: %w", err)
	}
	content, finalURL, err := downloadBytes(ctx, client, manifestURL, maxReleaseManifestSize)
	if err != nil {
		return ResolvedRelease{}, fmt.Errorf("download release manifest: %w", err)
	}
	actualDigest := sha256.Sum256(content)
	expectedDigest, _ := hex.DecodeString(expectedChecksum)
	if subtle.ConstantTimeCompare(actualDigest[:], expectedDigest) != 1 {
		return ResolvedRelease{}, fmt.Errorf("release manifest checksum mismatch")
	}
	manifest, err := decodeReleaseManifest(content)
	if err != nil {
		return ResolvedRelease{}, err
	}
	if err := validateReleaseManifest(manifest, selector); err != nil {
		return ResolvedRelease{}, err
	}
	selectedImages := make(map[Component]ReleaseImage)
	for _, component := range options.Components {
		if !releaseImageComponent(component) {
			continue
		}
		image, exists := manifest.Images[string(component)]
		if !exists {
			return ResolvedRelease{}, fmt.Errorf("release manifest is missing selected %s image", component)
		}
		selectedImages[component] = image
	}
	assets := make(map[string]ReleaseAsset, len(manifest.Assets))
	for name, asset := range manifest.Assets {
		assets[name] = asset
	}
	return ResolvedRelease{
		ApplicationVersion: manifest.ApplicationVersion,
		Commit:             manifest.Commit,
		Images:             selectedImages,
		MigrationImage:     manifest.Images[string(ComponentAPI)],
		Assets:             assets,
		ManifestURL:        finalURL,
	}, nil
}

func decodeReleaseManifest(content []byte) (ReleaseManifest, error) {
	var manifest ReleaseManifest
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("parse release manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ReleaseManifest{}, fmt.Errorf("release manifest contains trailing data")
	}
	return manifest, nil
}

func validateReleaseManifest(manifest ReleaseManifest, selector string) error {
	if manifest.SchemaVersion != ReleaseManifestSchemaVersion {
		return fmt.Errorf("unsupported release manifest schema version %d", manifest.SchemaVersion)
	}
	if !releaseVersionPattern.MatchString(manifest.ApplicationVersion) {
		return fmt.Errorf("release manifest application version must be vX.Y.Z")
	}
	if selector != "latest" && selector != manifest.ApplicationVersion {
		return fmt.Errorf("release manifest version %s does not match requested version %s", manifest.ApplicationVersion, selector)
	}
	if strings.TrimSpace(manifest.Commit) == "" {
		return fmt.Errorf("release manifest commit is required")
	}
	for _, component := range []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb} {
		image, exists := manifest.Images[string(component)]
		if !exists {
			if component == ComponentAPI {
				return fmt.Errorf("release manifest is missing the API migration image")
			}
			return fmt.Errorf("release manifest is missing %s image", component)
		}
		if strings.TrimSpace(image.Repository) == "" {
			return fmt.Errorf("release manifest %s image repository is required", component)
		}
		if image.Tag != manifest.ApplicationVersion {
			return fmt.Errorf("release manifest %s image tag %q does not match application version %q", component, image.Tag, manifest.ApplicationVersion)
		}
		if image.Version != manifest.ApplicationVersion {
			return fmt.Errorf("release manifest %s image version %q does not match application version %q", component, image.Version, manifest.ApplicationVersion)
		}
		if image.Revision != manifest.Commit {
			return fmt.Errorf("release manifest %s image revision does not match release commit", component)
		}
		if !validSHA256Digest(image.Digest) {
			return fmt.Errorf("release manifest %s image digest must be an immutable sha256 digest", component)
		}
	}
	if len(manifest.Assets) == 0 {
		return fmt.Errorf("release manifest must contain assets")
	}
	for key, asset := range manifest.Assets {
		if strings.TrimSpace(key) == "" || asset.Name != key {
			return fmt.Errorf("release manifest asset name %q is inconsistent", key)
		}
		if !validSHA256Hex(asset.SHA256) {
			return fmt.Errorf("release manifest asset checksum for %s must be SHA-256", key)
		}
	}
	return nil
}

func releaseImageComponent(component Component) bool {
	switch component {
	case ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb:
		return true
	default:
		return false
	}
}

func validSHA256Digest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validSHA256Hex(strings.TrimPrefix(value, "sha256:"))
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
