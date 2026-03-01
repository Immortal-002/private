package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TelemetryKeyPrefix = "telemetry:"
	TTL               = 2 * time.Minute
)

func SetLatestTelemetry(ctx context.Context, rdb *redis.Client, agentID string, data []byte) error {
	key := TelemetryKeyPrefix + agentID
	return rdb.Set(ctx, key, data, TTL).Err()
}

func GetLatestTelemetry(ctx context.Context, rdb *redis.Client, agentID string) ([]byte, error) {
	return rdb.Get(ctx, TelemetryKeyPrefix+agentID).Bytes()
}

func SetAgentStatus(ctx context.Context, rdb *redis.Client, agentID string, online bool) error {
	status := "offline"
	if online {
		status = "online"
	}
	return rdb.Set(ctx, fmt.Sprintf("agent:%s:status", agentID), status, TTL).Err()
}

func MarshalTelemetry(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
