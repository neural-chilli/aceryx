package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

func SetupLoggerFromEnv(w io.Writer) *slog.Logger {
	level := parseLevel(strings.TrimSpace(os.Getenv("ACERYX_LOG_LEVEL")))
	return SetupLogger(w, level)
}

func SetupLogger(w io.Writer, level slog.Level) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	}))
	slog.SetDefault(logger)
	return logger
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func RequestAttrs(ctx context.Context) []any {
	return []any{
		"correlation_id", CorrelationIDFromContext(ctx),
		"tenant_id", TenantIDFromContext(ctx),
		"principal_id", PrincipalIDFromContext(ctx),
	}
}
