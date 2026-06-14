package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ai-proxy/internal/config"
	"ai-proxy/internal/logger"
)

// Connect creates a new pgx connection pool from the configuration.
func Connect(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: parse config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.DatabaseMaxConns)
	poolCfg.MinConns = int32(cfg.DatabaseMinConns)
	poolCfg.MaxConnLifetime = cfg.DatabaseMaxIdleTime
	poolCfg.MaxConnIdleTime = cfg.DatabaseMaxIdleTime

	poolCfg.ConnConfig.Tracer = &queryTracer{}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}

	// Verify connectivity
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping failed: %w", err)
	}

	slog.Info("database connection established",
		slog.Int("max_conns", cfg.DatabaseMaxConns),
		slog.Int("min_conns", cfg.DatabaseMinConns),
	)
	return pool, nil
}

// queryTracer implements pgx.QueryTracer to log SQL queries via slog.
type queryTracer struct{}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	logger.FromContext(ctx).Debug("sql query",
		slog.String("sql", data.SQL),
		slog.Any("args", data.Args),
	)
	return ctx
}

func (t *queryTracer) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.Err != nil {
		slog.Debug("sql query error",
			slog.String("error", data.Err.Error()),
			slog.String("command_tag", data.CommandTag.String()),
		)
	}
}
