package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/telemetry-platform/backend/internal/config"
)

func NewClient(cfg *config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

func Ping(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
