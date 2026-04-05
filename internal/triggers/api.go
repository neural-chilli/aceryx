package triggers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type AdminHandlers struct {
	Manager *TriggerManager
}

func NewAdminHandlers(manager *TriggerManager) *AdminHandlers {
	return &AdminHandlers{Manager: manager}
}

func (h *AdminHandlers) List(w http.ResponseWriter, _ *http.Request) {
	if h.Manager == nil {
		writeJSON(w, http.StatusOK, []*TriggerInstanceInfo{})
		return
	}
	writeJSON(w, http.StatusOK, h.Manager.List())
}

func (h *AdminHandlers) Get(w http.ResponseWriter, r *http.Request) {
	if h.Manager == nil {
		writeError(w, http.StatusNotFound, "trigger_not_found")
		return
	}
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_trigger_id")
		return
	}
	item, err := h.Manager.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "trigger_not_found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AdminHandlers) Restart(w http.ResponseWriter, r *http.Request) {
	if h.Manager == nil {
		writeError(w, http.StatusNotFound, "trigger_not_found")
		return
	}
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_trigger_id")
		return
	}
	if err := h.Manager.Restart(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "trigger_restart_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *AdminHandlers) Stop(w http.ResponseWriter, r *http.Request) {
	if h.Manager == nil {
		writeError(w, http.StatusNotFound, "trigger_not_found")
		return
	}
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_trigger_id")
		return
	}
	if err := h.Manager.Stop(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "trigger_stop_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *AdminHandlers) ListCheckpoints(w http.ResponseWriter, r *http.Request) {
	if h.Manager == nil {
		writeJSON(w, http.StatusOK, []CheckpointRecord{})
		return
	}
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_trigger_id")
		return
	}
	items, err := h.Manager.ListCheckpoints(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "checkpoint_list_failed")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AdminHandlers) ResetCheckpoints(w http.ResponseWriter, r *http.Request) {
	if h.Manager == nil {
		writeError(w, http.StatusNotFound, "trigger_not_found")
		return
	}
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_trigger_id")
		return
	}
	err = h.Manager.ResetCheckpoints(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "must be stopped") {
			writeError(w, http.StatusConflict, "trigger_running")
			return
		}
		if errors.Is(err, ErrTriggerInstanceNotFound) {
			writeError(w, http.StatusNotFound, "trigger_not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, "checkpoint_reset_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func parseID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(r.PathValue("id")))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code, "code": code})
}
