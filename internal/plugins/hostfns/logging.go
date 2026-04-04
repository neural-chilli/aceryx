package hostfns

import "log/slog"

type LoggerHost struct{}

func (LoggerHost) Log(level, message string) {
	switch level {
	case "debug":
		slog.Debug(message)
	case "warn":
		slog.Warn(message)
	case "error":
		slog.Error(message)
	default:
		slog.Info(message)
	}
}
