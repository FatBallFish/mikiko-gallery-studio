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

func newRedisClient(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	url := strings.TrimSpace(cfg.Redis.URL)
	if url == "" {
		return nil, fmt.Errorf("redis.url must be configured in %s env", cfg.App.Env)
	}

	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(options)
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	slog.Info("redis runtime enabled", "env", cfg.App.Env)
	return client, nil
}
