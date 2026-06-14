// Package bootstrap initialises all shared application dependencies
// (database, repositories, services, provider registry) so that each
// server binary (api, admin) only needs to register its routes and start.
package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ai-proxy/internal/audit"
	"ai-proxy/internal/client"
	"ai-proxy/internal/config"
	"ai-proxy/internal/database"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/provider"
)

// Deps holds all initialised shared dependencies.
type Deps struct {
	Cfg             *config.Config
	DB              *pgxpool.Pool
	ClientRepo      client.Repository
	ClientSvc       *client.Service
	ProviderRepo    provider.Repository
	ProviderReg     *provider.Registry
	AuditRepo       audit.Repository
	AuditSvc        *audit.Service
	ProviderKeyRepo client.ClientProviderKeyRepository
	ProviderKeySvc  *client.ProviderKeyService
}

// Init loads configuration, connects to the database, and creates all
// repositories and services. Callers should call Stop() on relevant deps
// during graceful shutdown.
func Init(ctx context.Context) (*Deps, error) {
	cfg := config.Load()

	logger.Init(logger.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: cfg.IsDevelopment(),
	})

	slog.Info("starting server",
		slog.String("env", cfg.Env),
		slog.String("addr", cfg.Addr()),
	)

	pool, err := database.Connect(ctx, cfg)
	if err != nil {
		slog.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("database connection established")

	clientRepo := client.NewPostgresRepository(pool)
	clientSvc := client.NewService(clientRepo, cfg.EncryptionKey)

	providerRepo := provider.NewPostgresRepository(pool)
	providerReg := provider.NewRegistry(providerRepo)
	auditRepo := audit.NewPostgresRepository(pool)
	auditSvc := audit.NewService(auditRepo)

	providerKeyRepo := client.NewClientProviderKeyRepository(pool)
	providerKeySvc := client.NewProviderKeyService(providerKeyRepo, clientSvc, cfg.EncryptionKey)

	if err := providerReg.Refresh(ctx); err != nil {
		slog.Warn("failed to refresh provider registry", slog.String("error", err.Error()))
	}

	return &Deps{
		Cfg:             cfg,
		DB:              pool,
		ClientRepo:      clientRepo,
		ClientSvc:       clientSvc,
		ProviderRepo:    providerRepo,
		ProviderReg:     providerReg,
		AuditRepo:       auditRepo,
		AuditSvc:        auditSvc,
		ProviderKeyRepo: providerKeyRepo,
		ProviderKeySvc:  providerKeySvc,
	}, nil
}

// ShutdownCtx returns a context suitable for graceful shutdown operations.
func ShutdownCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}
