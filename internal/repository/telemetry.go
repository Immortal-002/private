package repository

import (
	"context"

	"github.com/telemetry-platform/backend/internal/models"
)

func (r *Repository) ListTelemetry(ctx context.Context, agentID string, limit, offset int) ([]models.TelemetryRecord, int, error) {
	var total int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM telemetry WHERE agent_id = $1`, agentID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, agent_id, cpu_usage, memory_usage,
		       memory_total, memory_used, disk_usage, disk_total, disk_used,
		       uptime_seconds, load_avg_1, load_avg_5, load_avg_15, recorded_at
		FROM telemetry WHERE agent_id = $1 ORDER BY recorded_at DESC LIMIT $2 OFFSET $3
	`, agentID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var records []models.TelemetryRecord
	for rows.Next() {
		var t models.TelemetryRecord
		if err := rows.Scan(&t.ID, &t.AgentID, &t.CPUUsage, &t.MemoryUsage,
			&t.MemoryTotal, &t.MemoryUsed, &t.DiskUsage, &t.DiskTotal, &t.DiskUsed,
			&t.UptimeSecs, &t.LoadAvg1, &t.LoadAvg5, &t.LoadAvg15, &t.RecordedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, t)
	}
	return records, total, rows.Err()
}
