package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type redisVideoClaimGate struct {
	client  *redis.Client
	mu      sync.Mutex
	lastLog time.Time
}

func newRedisVideoClaimGate(client *redis.Client) *redisVideoClaimGate {
	return &redisVideoClaimGate{client: client}
}

func (gate *redisVideoClaimGate) Allowed(ctx context.Context) (bool, error) {
	if gate == nil || gate.client == nil {
		return false, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := gate.client.Ping(probeCtx).Err(); err != nil {
		gate.logPaused("redis_unavailable")
		return false, nil
	}
	return true, nil
}

func (gate *redisVideoClaimGate) logPaused(code string) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	now := time.Now()
	if now.Sub(gate.lastLog) < 30*time.Second {
		return
	}
	gate.lastLog = now
	slog.Warn("video claims paused", "error_code", code)
}

type diskClaimGate struct {
	path         string
	pausePercent int
	mu           sync.Mutex
	lastLog      time.Time
	usage        func(string) (int, int64, error)
	observer     interface{ SetTemporaryDisk(int, int64) }
}

func newDiskClaimGate(path string, pausePercent int, observer interface{ SetTemporaryDisk(int, int64) }) *diskClaimGate {
	return &diskClaimGate{path: path, pausePercent: pausePercent, usage: diskUsage, observer: observer}
}

func (gate *diskClaimGate) Allowed(context.Context) (bool, error) {
	usage := gate.usage
	if usage == nil {
		usage = diskUsage
	}
	usedPercent, freeBytes, err := usage(gate.path)
	if err != nil {
		return false, err
	}
	if gate.observer != nil {
		gate.observer.SetTemporaryDisk(usedPercent, freeBytes)
	}
	if usedPercent < gate.pausePercent {
		return true, nil
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	now := time.Now()
	if now.Sub(gate.lastLog) >= 30*time.Second {
		gate.lastLog = now
		slog.Warn("media claims paused", "error_code", "temporary_disk_watermark", "used_percent", usedPercent, "pause_percent", gate.pausePercent)
	}
	return false, nil
}
