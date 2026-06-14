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
	"ai-proxy/internal/bootstrap"
	"ai-proxy/internal/shared"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deps, err := bootstrap.Init(ctx)
	if err != nil {
		slog.Error("bootstrap failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer deps.DB.Close()

	// Allow admin server to run on a different port via ADMIN_PORT
	if port := os.Getenv("ADMIN_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			deps.Cfg.ServerPort = p
		}
	}

	// ─── Admin-specific setup ─────────────────────────────
	router := shared.NewRouter(deps.Cfg)
	wsHub := admin.NewHub()

	adminHandler := admin.NewHandler(
		deps.Cfg, deps.DB, deps.ClientSvc, deps.ClientRepo,
		deps.ProviderKeySvc, deps.ProviderRepo, deps.ProviderReg,
		deps.AuditRepo, deps.AuditSvc,
	)
	adminGroup := router.Group("/api/v1/admin")
	adminGroup.Use(audit.Middleware(deps.AuditSvc, wsHub))
	adminHandler.RegisterRoutes(adminGroup, deps.Cfg.JWTSecret)

	router.GET("/api/v1/admin/ws/connections", func(c *gin.Context) {
		wsHub.HandleWebSocket(c)
	})

	// Serve frontend static assets
	if _, err := os.Stat("./web/dist/index.html"); err == nil {
		router.Static("/assets", "./web/dist/assets")
		router.StaticFile("/", "./web/dist/index.html")
		router.NoRoute(func(c *gin.Context) {
			c.File("./web/dist/index.html")
		})
		slog.Info("serving frontend from ./web/dist")
	} else {
		slog.Warn("frontend build not found at ./web/dist — run 'make web' first")
	}

	// ─── HTTP server ──────────────────────────────────────
	srv := &http.Server{
		Addr:         deps.Cfg.Addr(),
		Handler:      router,
		ReadTimeout:  deps.Cfg.ServerReadTimeout,
		WriteTimeout: deps.Cfg.ServerWriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// ─── Graceful shutdown ────────────────────────────────
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		slog.Info("shutting down admin server", slog.String("signal", sig.String()))

		deps.AuditSvc.Stop()
		deps.ClientSvc.Stop()

		shutdownCtx, shutdownCancel := bootstrap.ShutdownCtx()
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server forced to shutdown", slog.String("error", err.Error()))
		}
	}()

	slog.Info("admin server listening", slog.String("addr", deps.Cfg.Addr()))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("admin server error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("admin server stopped gracefully")
}
