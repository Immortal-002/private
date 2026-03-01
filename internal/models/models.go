package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID            uuid.UUID       `json:"id"`
	AgentID       string          `json:"agent_id"`
	Hostname      string          `json:"hostname"`
	IPAddress     *string         `json:"ip_address,omitempty"`
	OSType        *string         `json:"os_type,omitempty"`
	Status        string          `json:"status"`
	LastHeartbeat time.Time       `json:"last_heartbeat"`
	RegisteredAt  time.Time       `json:"registered_at"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

type TelemetryPayload struct {
	AgentID     string   `json:"agent_id" binding:"required"`
	CPUUsage    float64  `json:"cpu_usage" binding:"required"`
	MemoryUsage float64  `json:"memory_usage" binding:"required"`
	MemoryTotal *int64   `json:"memory_total,omitempty"`
	MemoryUsed  *int64   `json:"memory_used,omitempty"`
	DiskUsage   *float64 `json:"disk_usage,omitempty"`
	DiskTotal   *int64   `json:"disk_total,omitempty"`
	DiskUsed    *int64   `json:"disk_used,omitempty"`
	UptimeSecs  *int64   `json:"uptime_seconds,omitempty"`
	LoadAvg1    *float64 `json:"load_avg_1,omitempty"`
	LoadAvg5    *float64 `json:"load_avg_5,omitempty"`
	LoadAvg15   *float64 `json:"load_avg_15,omitempty"`
}

type AgentRegistrationRequest struct {
	AgentID   string  `json:"agent_id" binding:"required"`
	Hostname  string  `json:"hostname" binding:"required"`
	IPAddress *string `json:"ip_address,omitempty"`
	OSType    *string `json:"os_type,omitempty"`
}

type TelemetryRecord struct {
	ID          int64     `json:"id"`
	AgentID     string    `json:"agent_id"`
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryUsage float64   `json:"memory_usage"`
	MemoryTotal *int64    `json:"memory_total,omitempty"`
	MemoryUsed  *int64    `json:"memory_used,omitempty"`
	DiskUsage   *float64  `json:"disk_usage,omitempty"`
	DiskTotal   *int64    `json:"disk_total,omitempty"`
	DiskUsed    *int64    `json:"disk_used,omitempty"`
	UptimeSecs  *int64    `json:"uptime_seconds,omitempty"`
	LoadAvg1    *float64  `json:"load_avg_1,omitempty"`
	LoadAvg5    *float64  `json:"load_avg_5,omitempty"`
	LoadAvg15   *float64  `json:"load_avg_15,omitempty"`
	RecordedAt  time.Time `json:"recorded_at"`
}

type Command struct {
	ID           uuid.UUID       `json:"id"`
	AgentID      string          `json:"agent_id"`
	CommandType  string          `json:"command_type"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Status       string          `json:"status"`
	Result       *string         `json:"result,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	ExecutedAt   *time.Time      `json:"executed_at,omitempty"`
}

type CreateCommandRequest struct {
	AgentID     string          `json:"agent_id" binding:"required"`
	CommandType string          `json:"command_type" binding:"required,oneof=restart_agent collect_logs simulate_load ping"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

type CommandResultRequest struct {
	CommandID    string  `json:"command_id" binding:"required"`
	AgentID      string  `json:"agent_id" binding:"required"`
	Status       string  `json:"status" binding:"required,oneof=success failed"`
	Result       *string `json:"result,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token string `json:"token"`
}
