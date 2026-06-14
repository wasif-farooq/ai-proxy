package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"ai-proxy/internal/admin"
	"ai-proxy/internal/audit"
	"ai-proxy/internal/client"
	"ai-proxy/internal/config"
	"ai-proxy/internal/database"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/provider"
	"ai-proxy/internal/shared"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Allow admin server to run on a different port
	if port := os.Getenv("ADMIN_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.ServerPort = p
		}
	}

	// Initialise structured logging
	logger.Init(logger.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: cfg.IsDevelopment(),
	})
	slog.Info("starting admin server", slog.String("env", cfg.Env), slog.String("addr", cfg.Addr()))

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Connect(ctx, cfg)
	if err != nil {
		slog.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connection established")

	// Initialise repositories
	clientRepo := client.NewPostgresRepository(pool)
	clientSvc := client.NewService(clientRepo, cfg.EncryptionKey)
	providerRepo := provider.NewPostgresRepository(pool)
	providerReg := provider.NewRegistry(providerRepo)
	auditRepo := audit.NewPostgresRepository(pool)
	auditSvc := audit.NewService(auditRepo)

	// Initialise provider key service
	providerKeyRepo := client.NewClientProviderKeyRepository(pool)
	providerKeySvc := client.NewProviderKeyService(providerKeyRepo, clientSvc, cfg.EncryptionKey)

	// Refresh provider registry
	if err := providerReg.Refresh(ctx); err != nil {
		slog.Warn("failed to refresh provider registry", slog.String("error", err.Error()))
	}

	// Create Gin router
	router := shared.NewRouter(cfg)

	// WebSocket hub for live connections — must be created before admin routes
	// so the audit middleware can broadcast events to connected clients.
	wsHub := admin.NewHub()

	// Create admin handler and register routes with audit middleware
	// that broadcasts each audit event to WebSocket clients in real time.
	adminHandler := admin.NewHandler(cfg, pool, clientSvc, clientRepo, providerKeySvc, providerRepo, providerReg, auditRepo, auditSvc)
	adminGroup := router.Group("/api/v1/admin")
	adminGroup.Use(audit.Middleware(auditSvc, wsHub))
	adminHandler.RegisterRoutes(adminGroup, cfg.JWTSecret)

	// WebSocket endpoint for live audit events
	router.GET("/api/v1/admin/ws/connections", func(c *gin.Context) {
		wsHub.HandleWebSocket(c)
	})

	// Serve frontend static assets (always in dev, required in prod)
	if _, err := os.Stat("./web/dist/index.html"); err == nil {
		router.Static("/assets", "./web/dist/assets")
		router.StaticFile("/", "./web/dist/index.html")
		router.NoRoute(func(c *gin.Context) {
			c.File("./web/dist/index.html")
		})
		slog.Info("serving frontend from ./web/dist")
	} else {
		slog.Warn("frontend build not found at ./web/dist — run 'make web-build' first", slog.String("error", err.Error()))
	}

	// Configure HTTP server
	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      router,
		ReadTimeout:  cfg.ServerReadTimeout,
		WriteTimeout: cfg.ServerWriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		slog.Info("shutting down admin server", slog.String("signal", sig.String()))

		// Stop audit service (flush remaining events)
		auditSvc.Stop()
		clientSvc.Stop()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server forced to shutdown", slog.String("error", err.Error()))
		}
	}()

	slog.Info("admin server listening", slog.String("addr", cfg.Addr()))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("admin server error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("admin server stopped gracefully")
}
