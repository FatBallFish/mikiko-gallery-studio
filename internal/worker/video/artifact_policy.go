package video

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
)

// ArtifactHostPolicyFromExtra preserves legacy explicit lists while letting an
// explicit switch disable only host matching, never the transport protections.
func ArtifactHostPolicyFromExtra(extra map[string]any) ([]string, *bool) {
	var hosts []string
	add := func(value string) {
		if value = strings.TrimSpace(value); value != "" {
			hosts = append(hosts, value)
		}
	}
	switch values := extra["video_artifact_hosts"].(type) {
	case []string:
		for _, value := range values {
			add(value)
		}
	case []any:
		for _, value := range values {
			if text, ok := value.(string); ok {
				add(text)
			}
		}
	}
	var enabled *bool
	if value, exists := extra["video_artifact_hosts_enabled"]; exists {
		// Invalid explicit switches fail closed instead of silently disabling checks.
		flag, ok := value.(bool)
		if !ok {
			flag = true
		}
		enabled = &flag
	}
	return hosts, enabled
}

// ValidateArtifact probes the result through the worker's pinned HTTPS transport.
// A range GET supports signed GET-only URLs; the body is closed without copying,
// even if the server ignores Range. The probe has a short independent deadline.
func (r *Runner) ValidateArtifact(ctx context.Context, artifact providervideo.Artifact, hosts []string, enabled *bool) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	transport, err := r.artifactTransport(hosts, enabled)
	if err != nil {
		return err
	}
	client := *r.httpClient
	client.Transport = transport
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("artifact redirect must use HTTPS")
		}
		if len(via) >= 10 {
			return errors.New("too many artifact redirects")
		}
		if r.httpClient.CheckRedirect != nil {
			return r.httpClient.CheckRedirect(req, via)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(artifact.URL), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe artifact: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("probe artifact: HTTP %d", response.StatusCode)
	}
	return nil
}
