package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/telemetry-platform/backend/internal/models"
)

func (r *Repository) CreateCommand(ctx context.Context, agentID, cmdType string, payload []byte, createdBy *uuid.UUID) (*models.Command, error) {
	pl := []byte("{}")
	if len(payload) > 0 {
		pl = payload
	}
	var c models.Command
	err := r.db.QueryRow(ctx, `
		INSERT INTO commands (agent_id, command_type, payload, status, created_by)
		VALUES ($1, $2, $3::jsonb, 'pending', $4)
		RETURNING id, agent_id, command_type, payload, status, created_at
	`, agentID, cmdType, pl, createdBy).Scan(&c.ID, &c.AgentID, &c.CommandType, &c.Payload, &c.Status, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) GetPendingCommands(ctx context.Context, agentID string) ([]models.Command, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, agent_id, command_type, payload, status, created_at
		FROM commands WHERE agent_id = $1 AND status = 'pending' ORDER BY created_at ASC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cmds []models.Command
	for rows.Next() {
		var c models.Command
		if err := rows.Scan(&c.ID, &c.AgentID, &c.CommandType, &c.Payload, &c.Status, &c.CreatedAt); err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

func (r *Repository) UpdateCommandResult(ctx context.Context, cmdID, agentID, status string, result, errMsg *string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE commands SET status = $1, result = $2, error_message = $3, executed_at = NOW()
		WHERE id::text = $4 AND agent_id = $5
	`, status, result, errMsg, cmdID, agentID)
	return err
}

func (r *Repository) MarkCommandSent(ctx context.Context, cmdID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE commands SET status = 'sent' WHERE id::text = $1 AND status = 'pending'
	`, cmdID)
	return err
}
