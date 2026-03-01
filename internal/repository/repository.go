package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/telemetry-platform/backend/internal/models"
)

type Repository struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func New(db *pgxpool.Pool, rdb *redis.Client) *Repository {
	return &Repository{db: db, rdb: rdb}
}

func (r *Repository) Redis() *redis.Client {
	return r.rdb
}

func (r *Repository) RegisterAgent(ctx context.Context, req *models.TelemetryPayload) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO agents (agent_id, hostname, ip_address, os_type, status, last_heartbeat)
		VALUES ($1, $2, $3, $4, 'online', NOW())
		ON CONFLICT (agent_id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			last_heartbeat = NOW(),
			status = 'online'
	`, req.AgentID, req.AgentID, nil, nil)
	return err
}

func (r *Repository) UpsertAgent(ctx context.Context, agentID, hostname string, ipAddr, osType *string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO agents (agent_id, hostname, ip_address, os_type, status, last_heartbeat)
		VALUES ($1, $2, $3, $4, 'online', NOW())
		ON CONFLICT (agent_id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			ip_address = COALESCE(EXCLUDED.ip_address, agents.ip_address),
			os_type = COALESCE(EXCLUDED.os_type, agents.os_type),
			last_heartbeat = NOW(),
			status = 'online'
	`, agentID, hostname, ipAddr, osType)
	return err
}

func (r *Repository) StoreTelemetry(ctx context.Context, req *models.TelemetryPayload) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO telemetry (
			agent_id, cpu_usage, memory_usage, memory_total, memory_used,
			disk_usage, disk_total, disk_used, uptime_seconds,
			load_avg_1, load_avg_5, load_avg_15
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, req.AgentID, req.CPUUsage, req.MemoryUsage, req.MemoryTotal, req.MemoryUsed,
		req.DiskUsage, req.DiskTotal, req.DiskUsed, req.UptimeSecs,
		req.LoadAvg1, req.LoadAvg5, req.LoadAvg15)
	return err
}

func (r *Repository) UpdateAgentStatus(ctx context.Context, agentID string) error {
	_, err := r.db.Exec(ctx, `UPDATE agents SET last_heartbeat = NOW(), status = 'online' WHERE agent_id = $1`, agentID)
	return err
}
