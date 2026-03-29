package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

type TaskHandlers struct {
	Tasks *tasks.TaskService
}

func NewTaskHandlers(taskSvc *tasks.TaskService) *TaskHandlers {
	return &TaskHandlers{Tasks: taskSvc}
}

func (h *TaskHandlers) Inbox(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.Tasks.Inbox(r.Context(), principal.TenantID, principal.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *TaskHandlers) GetTask(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, stepID, ok := parseTaskPath(w, r)
	if !ok {
		return
	}
	detail, err := h.Tasks.GetTask(r.Context(), principal.TenantID, caseID, stepID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *TaskHandlers) Claim(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, stepID, ok := parseTaskPath(w, r)
	if !ok {
		return
	}
	if err := h.Tasks.ClaimTask(r.Context(), principal.TenantID, principal.ID, caseID, stepID); err != nil {
		if errors.Is(err, tasks.ErrAlreadyClaimed) {
			writeError(w, http.StatusConflict, "task_already_claimed")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "claimed"})
}

func (h *TaskHandlers) Complete(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, stepID, ok := parseTaskPath(w, r)
	if !ok {
		return
	}
	var req tasks.CompleteTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Tasks.CompleteTask(r.Context(), principal.TenantID, principal.ID, caseID, stepID, req); err != nil {
		switch {
		case errors.Is(err, tasks.ErrInvalidOutcome):
			writeError(w, http.StatusBadRequest, "invalid_outcome")
		case errors.Is(err, tasks.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden")
		case errors.Is(err, tasks.ErrAlreadyCompleted):
			writeError(w, http.StatusConflict, "task_already_completed")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "completed"})
}

func (h *TaskHandlers) SaveDraft(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, stepID, ok := parseTaskPath(w, r)
	if !ok {
		return
	}
	var req tasks.DraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Tasks.SaveDraft(r.Context(), principal.TenantID, principal.ID, caseID, stepID, req); err != nil {
		if errors.Is(err, tasks.ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved"})
}

func (h *TaskHandlers) Reassign(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, stepID, ok := parseTaskPath(w, r)
	if !ok {
		return
	}
	var req tasks.ReassignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Tasks.ReassignTask(r.Context(), principal.TenantID, principal.ID, caseID, stepID, req); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "reassigned"})
}

func (h *TaskHandlers) Escalate(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, stepID, ok := parseTaskPath(w, r)
	if !ok {
		return
	}
	var cfg tasks.EscalationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Tasks.EscalateTask(r.Context(), principal.TenantID, caseID, stepID, cfg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "escalated"})
}

func parseTaskPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	caseID, err := uuid.Parse(r.PathValue("case_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_case_id")
		return uuid.Nil, "", false
	}
	stepID := r.PathValue("step_id")
	if stepID == "" {
		writeError(w, http.StatusBadRequest, "invalid_step_id")
		return uuid.Nil, "", false
	}
	return caseID, stepID, true
}
