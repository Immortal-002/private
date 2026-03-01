package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/telemetry-platform/backend/internal/cache"
	"github.com/telemetry-platform/backend/internal/models"
	"github.com/telemetry-platform/backend/internal/repository"
)

type AgentHandler struct {
	repo *repository.Repository
	rdb  *redis.Client
}

func NewAgentHandler(repo *repository.Repository, rdb *redis.Client) *AgentHandler {
	return &AgentHandler{repo: repo, rdb: rdb}
}

func (h *AgentHandler) RegisterAgent(c *gin.Context) {
	var req models.AgentRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}
	if err := h.repo.UpsertAgent(c.Request.Context(), req.AgentID, req.Hostname, req.IPAddress, req.OSType); err != nil {
		log.Error().Err(err).Str("agent_id", req.AgentID).Msg("failed to register agent")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register agent"})
		return
	}
	if h.rdb != nil {
		_ = cache.SetAgentStatus(c.Request.Context(), h.rdb, req.AgentID, true)
	}
	log.Info().Str("agent_id", req.AgentID).Str("hostname", req.Hostname).Msg("agent registered")
	c.JSON(http.StatusOK, gin.H{"status": "registered", "agent_id": req.AgentID})
}

func (h *AgentHandler) IngestTelemetry(c *gin.Context) {
	var req models.TelemetryPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.CPUUsage < 0 || req.CPUUsage > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cpu_usage out of range"})
		return
	}
	if req.MemoryUsage < 0 || req.MemoryUsage > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "memory_usage out of range"})
		return
	}
	// Auto-register agent if not exists (handles agents that skip registration)
	if err := h.repo.RegisterAgent(c.Request.Context(), &req); err != nil {
		log.Error().Err(err).Str("agent_id", req.AgentID).Msg("failed to upsert agent on telemetry")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register"})
		return
	}
	if err := h.repo.StoreTelemetry(c.Request.Context(), &req); err != nil {
		log.Error().Err(err).Str("agent_id", req.AgentID).Msg("failed to store telemetry")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store telemetry"})
		return
	}
	if data, err := cache.MarshalTelemetry(req); err == nil && h.rdb != nil {
		_ = cache.SetLatestTelemetry(c.Request.Context(), h.rdb, req.AgentID, data)
		_ = cache.SetAgentStatus(c.Request.Context(), h.rdb, req.AgentID, true)
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AgentHandler) ReportCommandResult(c *gin.Context) {
	var req models.CommandResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := h.repo.UpdateCommandResult(c.Request.Context(), req.CommandID, req.AgentID, req.Status, req.Result, req.ErrorMessage); err != nil {
		log.Error().Err(err).Str("command_id", req.CommandID).Msg("failed to update command result")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update command"})
		return
	}
	log.Info().Str("command_id", req.CommandID).Str("status", req.Status).Msg("command result received")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
