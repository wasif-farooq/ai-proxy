package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-proxy/internal/audit"
	"ai-proxy/internal/client"
	"ai-proxy/internal/config"
	"ai-proxy/internal/database"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/provider"
	"ai-proxy/internal/security"
	"ai-proxy/internal/shared"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialise structured logging
	logger.Init(logger.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: cfg.IsDevelopment(),
	})
	slog.Info("starting API server", slog.String("env", cfg.Env), slog.String("addr", cfg.Addr()))

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

	// Initialise repositories and services
	clientRepo := client.NewPostgresRepository(pool)
	clientSvc := client.NewService(clientRepo, cfg.EncryptionKey)

	providerRepo := provider.NewPostgresRepository(pool)
	providerReg := provider.NewRegistry(providerRepo)
	auditRepo := audit.NewPostgresRepository(pool)
	auditSvc := audit.NewService(auditRepo)

	// Initialise provider key service for per-client API keys
	providerKeyRepo := client.NewClientProviderKeyRepository(pool)
	providerKeySvc := client.NewProviderKeyService(providerKeyRepo, clientSvc, cfg.EncryptionKey)

	// Refresh provider registry
	if err := providerReg.Refresh(ctx); err != nil {
		slog.Warn("failed to refresh provider registry", slog.String("error", err.Error()))
	}

	// Create proxy and security middleware
	proxy := provider.NewProxy(providerReg, providerKeySvc, 120*time.Second)
	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(cfg.RateLimitRequestsPerMin, cfg.RateLimitBurst)

	// Create Gin router
	router := shared.NewRouter(cfg)

	// API v1 routes
	api := router.Group("/api/v1")
	{
		// Proxy route: chat completions
		api.POST("/chat/completions",
			provider.AuthMiddleware(clientSvc),
			security.NonceMiddleware(nonceStore, 5*time.Minute),
			security.RateLimitMiddleware(rateLimiter),
			provider.RouteMiddleware(proxy),
		)
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
		slog.Info("shutting down server", slog.String("signal", sig.String()))

		// Cleanup
		auditSvc.Stop()
		clientSvc.Stop()
		rateLimiter.Stop()
		nonceStore.Stop()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server forced to shutdown", slog.String("error", err.Error()))
		}
	}()

	// Start server
	slog.Info("listening", slog.String("addr", cfg.Addr()))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("server stopped gracefully")
}
