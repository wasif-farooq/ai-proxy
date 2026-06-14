package logger

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Config defines the logger initialisation options.
type Config struct {
	Level     string // debug, info, warn, error
	Format    string // json, text
	AddSource bool
}

var (
	defaultLogger *slog.Logger
	loggerKey     = struct{}{}
	loggerOnce    sync.Once
)

// Init sets up the global default logger.
func Init(cfg Config) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   slog.TimeKey,
					Value: slog.StringValue(a.Value.Time().Format(time.RFC3339Nano)),
				}
			}
			return a
		},
	}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// Default returns the package-level default logger.
func Default() *slog.Logger {
	loggerOnce.Do(func() {
		if defaultLogger == nil {
			Init(Config{Level: "info", Format: "text"})
		}
	})
	return defaultLogger
}

// FromContext extracts a request-scoped logger from the context.
// Falls back to the default logger if none is attached.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return Default()
}

// WithContext attaches key-value pairs to the logger stored in the context
// and returns a new context with the enriched logger.
// args must be alternating key-value pairs (string, any, string, any, ...).
func WithContext(ctx context.Context, args ...any) context.Context {
	l := FromContext(ctx).With(args...)
	return context.WithValue(ctx, loggerKey, l)
}
