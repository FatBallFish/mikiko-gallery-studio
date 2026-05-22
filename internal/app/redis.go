package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func newRedisClient(ctx context.Context, cfg config.Config) (*redis.Client, bool, error) {
	url := strings.TrimSpace(cfg.Redis.URL)
	if url == "" {
		if redisFallbackAllowed(cfg.App.Env) {
			slog.Warn("redis is not configured; falling back to local in-memory auth hot paths", "env", cfg.App.Env)
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("redis.url must be configured in %s env", cfg.App.Env)
	}

	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, false, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(options)
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		if redisFallbackAllowed(cfg.App.Env) {
			slog.Warn("redis ping failed; falling back to local in-memory auth hot paths", "env", cfg.App.Env, "err", err)
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("ping redis: %w", err)
	}

	slog.Info("redis runtime enabled", "env", cfg.App.Env)
	return client, false, nil
}

func redisFallbackAllowed(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "local", "dev", "development", "test":
		return true
	default:
		return false
	}
}
