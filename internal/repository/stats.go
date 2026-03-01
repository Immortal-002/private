package repository

import (
	"context"
)

type Stats struct {
	TotalAgents   int     `json:"total_agents"`
	OnlineAgents  int     `json:"online_agents"`
	TotalTelemetry int64  `json:"total_telemetry"`
	AvgCPU        float64 `json:"avg_cpu,omitempty"`
	AvgMemory     float64 `json:"avg_memory,omitempty"`
}

func (r *Repository) GetStats(ctx context.Context) (*Stats, error) {
	var s Stats
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM agents`).Scan(&s.TotalAgents)
	if err != nil {
		return nil, err
	}
	err = r.db.QueryRow(ctx, `SELECT COUNT(*) FROM agents WHERE status = 'online' AND last_heartbeat > NOW() - INTERVAL '2 minutes'`).Scan(&s.OnlineAgents)
	if err != nil {
		return nil, err
	}
	err = r.db.QueryRow(ctx, `SELECT COUNT(*) FROM telemetry`).Scan(&s.TotalTelemetry)
	if err != nil {
		return nil, err
	}
	_ = r.db.QueryRow(ctx, `
		SELECT AVG(cpu_usage), AVG(memory_usage) FROM telemetry
		WHERE recorded_at > NOW() - INTERVAL '1 hour'
	`).Scan(&s.AvgCPU, &s.AvgMemory)
	return &s, nil
}
