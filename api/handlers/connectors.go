package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

type ConnectorHandlers struct {
	Registry *connectors.Registry
	Secrets  connectors.SecretStore
}

func NewConnectorHandlers(registry *connectors.Registry, secrets connectors.SecretStore) *ConnectorHandlers {
	return &ConnectorHandlers{Registry: registry, Secrets: secrets}
}

func (h *ConnectorHandlers) List(w http.ResponseWriter, r *http.Request) {
	if middleware.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	writeJSON(w, http.StatusOK, h.Registry.Describe())
}

func (h *ConnectorHandlers) TestAction(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	connectorKey := r.PathValue("key")
	actionKey := r.PathValue("action")

	action, ok := h.Registry.GetAction(connectorKey, actionKey)
	if !ok {
		writeError(w, http.StatusNotFound, "connector_action_not_found")
		return
	}

	var req struct {
		Auth  map[string]string `json:"auth"`
		Input map[string]any    `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if req.Auth == nil {
		req.Auth = map[string]string{}
	}
	if req.Input == nil {
		req.Input = map[string]any{}
	}

	for k, v := range req.Auth {
		req.Auth[k] = connectors.ResolveTemplateString(v, map[string]any{
			"tenant": map[string]any{"id": principal.TenantID.String()},
			"secrets": map[string]any{
				k: v,
			},
		})
	}

	// Resolve missing required auth fields via secret store.
	if connector, ok := h.Registry.Get(connectorKey); ok && h.Secrets != nil {
		for _, field := range connector.Auth().Fields {
			if req.Auth[field.Key] != "" {
				continue
			}
			value, err := h.Secrets.Get(r.Context(), principal.TenantID, field.Key)
			if err == nil {
				req.Auth[field.Key] = value
			}
		}
	}

	req.Input["_tenant_id"] = principal.TenantID.String()
	req.Input["_actor_id"] = principal.ID.String()
	result, err := action.Execute(r.Context(), req.Auth, req.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
