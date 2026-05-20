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
	if driver != "local" {
		return fmt.Errorf("storage.driver=%s is not implemented yet; only local storage is currently supported", driver)
	}
	if cfg.Storage.SharedVolume {
		return nil
	}
	if strings.EqualFold(cfg.App.Env, "local") && databaseLooksLocal(cfg.Database.URL) {
		return nil
	}
	return fmt.Errorf("storage.driver=local requires storage.shared_volume=true outside local env so api and worker can share reference assets")
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
