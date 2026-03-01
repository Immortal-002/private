package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/telemetry-platform/backend/internal/repository"
)

type StatsHandler struct {
	repo *repository.Repository
}

func NewStatsHandler(repo *repository.Repository) *StatsHandler {
	return &StatsHandler{repo: repo}
}

func (h *StatsHandler) GetStats(c *gin.Context) {
	stats, err := h.repo.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}
	c.JSON(http.StatusOK, stats)
}
