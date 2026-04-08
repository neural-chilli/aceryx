package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/extraction"
)

type ExtractionHandlers struct {
	Service *extraction.Service
}

func NewExtractionHandlers(service *extraction.Service) *ExtractionHandlers {
	return &ExtractionHandlers{Service: service}
}

func (h *ExtractionHandlers) ListSchemas(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.Service.ListSchemas(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ExtractionHandlers) CreateSchema(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req extraction.UpsertSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	item, err := h.Service.CreateSchema(r.Context(), principal.TenantID, req)
	if err != nil {
		if writeExtractionValidationError(w, err) {
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *ExtractionHandlers) GetSchema(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	item, err := h.Service.GetSchema(r.Context(), principal.TenantID, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *ExtractionHandlers) UpdateSchema(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	var req extraction.UpsertSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	item, err := h.Service.UpdateSchema(r.Context(), principal.TenantID, id, req)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if writeExtractionValidationError(w, err) {
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *ExtractionHandlers) DeleteSchema(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.Service.DeleteSchema(r.Context(), principal.TenantID, id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *ExtractionHandlers) GetJob(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	item, err := h.Service.GetJob(r.Context(), principal.TenantID, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *ExtractionHandlers) ListFields(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	jobID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	items, err := h.Service.ListFields(r.Context(), principal.TenantID, jobID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ExtractionHandlers) AcceptJob(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	jobID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.Service.AcceptJob(r.Context(), principal.TenantID, jobID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *ExtractionHandlers) RejectJob(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	jobID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.Service.RejectJob(r.Context(), principal.TenantID, jobID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *ExtractionHandlers) ConfirmField(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	fieldID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.Service.ConfirmField(r.Context(), principal.TenantID, fieldID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *ExtractionHandlers) CorrectField(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	fieldID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	var req extraction.CorrectFieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Service.CorrectField(r.Context(), principal.TenantID, fieldID, req.CorrectedValue, principal.ID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if writeExtractionValidationError(w, err) {
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *ExtractionHandlers) RejectField(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	fieldID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.Service.RejectField(r.Context(), principal.TenantID, fieldID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *ExtractionHandlers) ListCorrections(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var schemaID *uuid.UUID
	schemaRaw := strings.TrimSpace(r.URL.Query().Get("schema_id"))
	if schemaRaw != "" {
		parsed, err := uuid.Parse(schemaRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_schema_id")
			return
		}
		schemaID = &parsed
	}
	var since *time.Time
	sinceRaw := strings.TrimSpace(r.URL.Query().Get("since"))
	if sinceRaw != "" {
		parsed, err := time.Parse(time.RFC3339, sinceRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_since")
			return
		}
		since = &parsed
	}
	items, err := h.Service.ListCorrections(r.Context(), principal.TenantID, schemaID, since)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func writeExtractionValidationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case msg == "name is required",
		msg == "fields is required",
		msg == "fields must be a json array",
		msg == "corrected_value is required":
		writeError(w, http.StatusBadRequest, msg)
		return true
	case strings.HasPrefix(msg, "fields must be valid json:"):
		writeError(w, http.StatusBadRequest, msg)
		return true
	default:
		return false
	}
}
