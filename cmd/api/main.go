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

	"ai-proxy/internal/bootstrap"
	"ai-proxy/internal/provider"
	"ai-proxy/internal/security"
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

	// ─── API-specific setup ───────────────────────────────
	proxy := provider.NewProxy(deps.ProviderReg, deps.ProviderKeySvc, 120*time.Second)
	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(deps.Cfg.RateLimitRequestsPerMin, deps.Cfg.RateLimitBurst)

	router := shared.NewRouter(deps.Cfg)
	api := router.Group("/api/v1")
	{
		api.POST("/chat/completions",
			provider.DualAuthMiddleware(deps.ClientSvc, nonceStore, 5*time.Minute),
			security.RateLimitMiddleware(rateLimiter),
			provider.RouteMiddleware(proxy),
		)
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
		slog.Info("shutting down API server", slog.String("signal", sig.String()))

		deps.AuditSvc.Stop()
		deps.ClientSvc.Stop()
		deps.ProviderKeySvc.Stop()
		rateLimiter.Stop()
		nonceStore.Stop()

		shutdownCtx, shutdownCancel := bootstrap.ShutdownCtx()
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server forced to shutdown", slog.String("error", err.Error()))
		}
	}()

	slog.Info("API server listening", slog.String("addr", deps.Cfg.Addr()))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("API server stopped gracefully")
}
