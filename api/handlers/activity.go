package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/activity"
)

type ActivityHandlers struct {
	Activity *activity.Service
}

func NewActivityHandlers(svc *activity.Service) *ActivityHandlers {
	return &ActivityHandlers{Activity: svc}
}

func (h *ActivityHandlers) Feed(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var (
		beforeTime *time.Time
		beforeID   *uuid.UUID
	)
	beforeTimeRaw := strings.TrimSpace(r.URL.Query().Get("before_time"))
	beforeIDRaw := strings.TrimSpace(r.URL.Query().Get("before_id"))
	if beforeTimeRaw != "" || beforeIDRaw != "" {
		if beforeTimeRaw == "" || beforeIDRaw == "" {
			writeError(w, http.StatusBadRequest, "before_time and before_id must be provided together")
			return
		}
		parsedTime, err := time.Parse(time.RFC3339Nano, beforeTimeRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_before_time")
			return
		}
		parsedID, err := uuid.Parse(beforeIDRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_before_id")
			return
		}
		beforeTime = &parsedTime
		beforeID = &parsedID
	}

	filter := r.URL.Query().Get("filter")
	out, err := h.Activity.GetFeedByFilter(r.Context(), principal.TenantID, limit, beforeTime, beforeID, filter)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
