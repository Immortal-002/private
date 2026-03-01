package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/telemetry-platform/backend/internal/models"
)

func (r *Repository) ListAgents(ctx context.Context, limit, offset int) ([]models.Agent, int, error) {
	var total int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM agents`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, agent_id, hostname, ip_address, os_type, status, last_heartbeat, registered_at, metadata
		FROM agents ORDER BY last_heartbeat DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		err := rows.Scan(&a.ID, &a.AgentID, &a.Hostname, &a.IPAddress, &a.OSType, &a.Status, &a.LastHeartbeat, &a.RegisteredAt, &a.Metadata)
		if err != nil {
			return nil, 0, err
		}
		agents = append(agents, a)
	}
	return agents, total, rows.Err()
}

func (r *Repository) GetAgent(ctx context.Context, agentID string) (*models.Agent, error) {
	var a models.Agent
	err := r.db.QueryRow(ctx, `
		SELECT id, agent_id, hostname, ip_address, os_type, status, last_heartbeat, registered_at, metadata
		FROM agents WHERE agent_id = $1
	`, agentID).Scan(&a.ID, &a.AgentID, &a.Hostname, &a.IPAddress, &a.OSType, &a.Status, &a.LastHeartbeat, &a.RegisteredAt, &a.Metadata)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) GetAdminByUsername(ctx context.Context, username string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var hash string
	err := r.db.QueryRow(ctx, `SELECT id, password_hash FROM admin_users WHERE username = $1`, username).Scan(&id, &hash)
	return id, hash, err
}

var ErrAdminExists = errors.New("admin already exists")

func (r *Repository) CreateAdmin(ctx context.Context, username, passwordHash string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO admin_users (username, password_hash) VALUES ($1, $2)
		ON CONFLICT (username) DO NOTHING
		RETURNING id
	`, username, passwordHash).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrAdminExists
		}
		return uuid.Nil, err
	}
	return id, nil
}
