package routes

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/telemetry-platform/backend/internal/config"
	"github.com/telemetry-platform/backend/internal/handlers"
	"github.com/telemetry-platform/backend/internal/middleware"
	"github.com/telemetry-platform/backend/internal/repository"
)

func Setup(router *gin.Engine, repo *repository.Repository, rdb *redis.Client, cfg *config.Config) {
	admin := handlers.NewAdminHandler(repo, cfg)
	agent := handlers.NewAgentHandler(repo, rdb)
	cmds := handlers.NewCommandsHandler(repo)
	stats := handlers.NewStatsHandler(repo)

	rateLimit := middleware.RateLimit(rdb, 100, time.Minute) // 100 req/min per IP

	// Health check (no auth, no rate limit)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Public agent endpoints (no JWT - agents use agent_id)
	api := router.Group("/api/v1")
	api.Use(rateLimit)
	{
		api.POST("/agents/register", agent.RegisterAgent)
		api.POST("/telemetry", agent.IngestTelemetry)
		api.GET("/commands/pending", cmds.GetPendingCommands)
		api.POST("/commands/result", agent.ReportCommandResult)
	}

	// Auth endpoints (public)
	api.POST("/auth/login", admin.Login)
	api.POST("/auth/register", admin.RegisterAdmin)

	// Protected admin endpoints
	adminGroup := api.Group("")
	adminGroup.Use(middleware.JWTAuth(cfg.JWT.Secret))
	{
		adminGroup.GET("/agents", admin.ListAgents)
		adminGroup.GET("/agents/:agent_id", admin.GetAgent)
		adminGroup.GET("/telemetry/:agent_id", admin.GetTelemetry)
		adminGroup.POST("/commands", admin.CreateCommand)
		adminGroup.GET("/stats", stats.GetStats)
	}
}
