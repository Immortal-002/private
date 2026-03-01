package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/telemetry-platform/backend/internal/repository"
)

type CommandsHandler struct {
	repo *repository.Repository
}

func NewCommandsHandler(repo *repository.Repository) *CommandsHandler {
	return &CommandsHandler{repo: repo}
}

func (h *CommandsHandler) GetPendingCommands(c *gin.Context) {
	agentID := c.Query("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id required"})
		return
	}
	cmds, err := h.repo.GetPendingCommands(c.Request.Context(), agentID)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentID).Msg("failed to get pending commands")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get commands"})
		return
	}
	// Mark fetched commands as "sent" so they're not re-fetched
	for _, cmd := range cmds {
		_ = h.repo.MarkCommandSent(c.Request.Context(), cmd.ID.String())
	}
	c.JSON(http.StatusOK, gin.H{"commands": cmds})
}
