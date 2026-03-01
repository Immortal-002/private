package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/telemetry-platform/backend/internal/auth"
	"github.com/telemetry-platform/backend/internal/config"
	"github.com/telemetry-platform/backend/internal/models"
	"github.com/telemetry-platform/backend/internal/repository"
)

type AdminHandler struct {
	repo *repository.Repository
	cfg  *config.Config
}

func NewAdminHandler(repo *repository.Repository, cfg *config.Config) *AdminHandler {
	return &AdminHandler{repo: repo, cfg: cfg}
}

func (h *AdminHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	userID, hash, err := h.repo.GetAdminByUsername(c.Request.Context(), req.Username)
	if err != nil || !auth.CheckPasswordHash(req.Password, hash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	token, err := auth.GenerateToken(userID.String(), req.Username, h.cfg.JWT.Secret, h.cfg.JWT.ExpireMins)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, models.LoginResponse{Token: token})
}

func (h *AdminHandler) RegisterAdmin(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	_, err = h.repo.CreateAdmin(c.Request.Context(), req.Username, hash)
	if err != nil {
		if errors.Is(err, repository.ErrAdminExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
			return
		}
		log.Error().Err(err).Str("username", req.Username).Msg("failed to create admin")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create admin"})
		return
	}
	log.Info().Str("username", req.Username).Msg("admin created")
	c.JSON(http.StatusCreated, gin.H{"message": "admin created"})
}

func (h *AdminHandler) ListAgents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit > 100 {
		limit = 100
	}
	agents, total, err := h.repo.ListAgents(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": agents, "total": total})
}

func (h *AdminHandler) GetAgent(c *gin.Context) {
	agentID := c.Param("agent_id")
	agent, err := h.repo.GetAgent(c.Request.Context(), agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	c.JSON(http.StatusOK, agent)
}

func (h *AdminHandler) GetTelemetry(c *gin.Context) {
	agentID := c.Param("agent_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit > 200 {
		limit = 200
	}
	records, total, err := h.repo.ListTelemetry(c.Request.Context(), agentID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list telemetry"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"telemetry": records, "total": total})
}

func (h *AdminHandler) CreateCommand(c *gin.Context) {
	var req models.CreateCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	uid, _ := c.Get("user_id")
	var createdBy *uuid.UUID
	if s, ok := uid.(string); ok {
		if u, err := uuid.Parse(s); err == nil {
			createdBy = &u
		}
	}
	var payload []byte
	if req.Payload != nil {
		payload = req.Payload
	}
	cmd, err := h.repo.CreateCommand(c.Request.Context(), req.AgentID, req.CommandType, payload, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create command"})
		return
	}
	c.JSON(http.StatusCreated, cmd)
}
