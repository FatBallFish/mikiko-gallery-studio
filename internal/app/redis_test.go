package app

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestNewRedisClientNeverFallsBackForCompletedRuntimeEnvironments(t *testing.T) {
	for _, environment := range []string{"", "local", "dev", "development", "test", "production"} {
		t.Run(environment, func(t *testing.T) {
			client, err := newRedisClient(context.Background(), config.Config{App: config.AppConfig{Env: environment}})
			if client != nil {
				_ = client.Close()
			}
			if err == nil || client != nil {
				t.Fatalf("missing Redis in %q env = client %v err %v; want strict error", environment, client != nil, err)
			}
		})
	}
}

func TestNewRedisClientUnreachableIsBoundedAndNeverFallsBack(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Redis address: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	client, err := newRedisClient(ctx, config.Config{
		App: config.AppConfig{Env: "local"}, Redis: config.RedisConfig{URL: "redis://" + address + "/0"},
	})
	if client != nil {
		_ = client.Close()
	}
	if err == nil || client != nil || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("unreachable Redis = client %v elapsed %s err %v", client != nil, time.Since(started), err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "falling back") {
		t.Fatalf("unreachable Redis error implies fallback: %v", err)
	}
}
