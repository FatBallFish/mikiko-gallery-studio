package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	redis "github.com/redis/go-redis/v9"
)

type StorageInvalidation struct {
	ConfigID       string `json:"config_id,omitempty"`
	Version        int64  `json:"version,omitempty"`
	DefaultChanged bool   `json:"default_changed,omitempty"`
}

type InvalidationPublisher interface {
	Publish(ctx context.Context, event StorageInvalidation) error
}

type RedisInvalidationBus struct {
	client  *redis.Client
	channel string
}

func NewRedisInvalidationBus(client *redis.Client, keyPrefix string) *RedisInvalidationBus {
	channel := strings.Trim(strings.TrimSpace(keyPrefix), ":")
	if channel != "" {
		channel += ":"
	}
	return &RedisInvalidationBus{client: client, channel: channel + "storage-config-invalidation"}
}

func (b *RedisInvalidationBus) Publish(ctx context.Context, event StorageInvalidation) error {
	if b == nil || b.client == nil {
		return nil
	}
	payload, err := encodeStorageInvalidation(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.channel, payload).Err()
}

func (b *RedisInvalidationBus) Subscribe(ctx context.Context, handler func(StorageInvalidation)) error {
	if b == nil || b.client == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	pubsub := b.client.Subscribe(ctx, b.channel)
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("subscribe storage invalidation: %w", err)
	}
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-channel:
			if !ok {
				return nil
			}
			event, err := decodeStorageInvalidation([]byte(message.Payload))
			if err == nil && handler != nil {
				handler(event)
			}
		}
	}
}

func encodeStorageInvalidation(event StorageInvalidation) ([]byte, error) {
	return json.Marshal(event)
}

func decodeStorageInvalidation(payload []byte) (StorageInvalidation, error) {
	var event StorageInvalidation
	if err := json.Unmarshal(payload, &event); err != nil {
		return StorageInvalidation{}, err
	}
	return event, nil
}
