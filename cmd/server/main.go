package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/telemetry-platform/backend/internal/config"
	"github.com/telemetry-platform/backend/internal/database"
	"github.com/telemetry-platform/backend/internal/middleware"
	"github.com/telemetry-platform/backend/internal/redis"
	"github.com/telemetry-platform/backend/internal/repository"
	"github.com/telemetry-platform/backend/internal/routes"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	gin.SetMode(gin.ReleaseMode)

	cfg := config.Load()
	ctx := context.Background()

	pool, err := database.NewPostgres(ctx, &cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pool.Close()

	rdb := redis.NewClient(&cfg.Redis)
	if err := redis.Ping(ctx, rdb); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer rdb.Close()

	repo := repository.New(pool, rdb)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger())

	routes.Setup(router, repo, rdb, cfg)

	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Server.Port),
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	log.Info().Str("addr", srv.Addr).Msg("server started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
	}
}
