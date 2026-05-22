package app

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func validateStorageTopology(cfg config.Config) error {
	driver := strings.ToLower(strings.TrimSpace(cfg.Storage.Driver))
	if driver == "" {
		driver = "local"
	}
	switch driver {
	case "local":
		if cfg.Storage.SharedVolume {
			return nil
		}
		if strings.EqualFold(cfg.App.Env, "local") && databaseLooksLocal(cfg.Database.URL) {
			return nil
		}
		return fmt.Errorf("storage.driver=local requires storage.shared_volume=true outside local env so api and worker can share reference assets")
	case "s3":
		if strings.TrimSpace(cfg.Storage.S3.Endpoint) == "" {
			return fmt.Errorf("storage.driver=s3 requires storage.s3.endpoint")
		}
		if strings.TrimSpace(cfg.Storage.S3.Region) == "" {
			return fmt.Errorf("storage.driver=s3 requires storage.s3.region")
		}
		if strings.TrimSpace(cfg.Storage.S3.Bucket) == "" {
			return fmt.Errorf("storage.driver=s3 requires storage.s3.bucket")
		}
		if strings.TrimSpace(cfg.Storage.S3.AccessKeyID) == "" || strings.TrimSpace(cfg.Storage.S3.SecretAccessKey) == "" {
			return fmt.Errorf("storage.driver=s3 requires storage.s3 credentials")
		}
		return nil
	default:
		return fmt.Errorf("storage.driver=%s is not implemented", driver)
	}
}

func databaseLooksLocal(rawURL string) bool {
	normalized := strings.TrimSpace(rawURL)
	switch {
	case normalized == "", normalized == ":memory:", strings.HasPrefix(normalized, "sqlite://"), strings.HasPrefix(normalized, "file:"):
		return true
	}
	if strings.Contains(normalized, "=") && !strings.Contains(normalized, "://") {
		return kvDSNLooksLocal(normalized)
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1"
}

func kvDSNLooksLocal(raw string) bool {
	for _, field := range strings.Fields(raw) {
		key, value, ok := strings.Cut(field, "=")
		if !ok || !strings.EqualFold(key, "host") {
			continue
		}
		host := strings.ToLower(strings.TrimSpace(value))
		return host == "localhost" || host == "127.0.0.1"
	}
	return false
}
