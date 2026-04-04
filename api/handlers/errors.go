package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

func writeInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.ErrorContext(r.Context(), "request failed", "error", err)
	}
	writeError(w, http.StatusInternalServerError, "internal_error")
}
