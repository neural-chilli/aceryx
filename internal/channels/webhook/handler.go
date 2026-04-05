package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/channels"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

type WebhookHandler struct {
	ChannelStore channels.ChannelStore
	Pipeline     *channels.Pipeline
	SecretStore  connectors.SecretStore
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if h.ChannelStore == nil || h.Pipeline == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook_pipeline_unavailable")
		return
	}
	channelID, err := uuid.Parse(strings.TrimSpace(r.PathValue("channel_id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_channel_id")
		return
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("tenant_id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "tenant_id_required")
		return
	}

	channel, err := h.ChannelStore.Get(r.Context(), tenantID, channelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel_not_found")
		return
	}
	if !channel.Enabled {
		writeError(w, http.StatusServiceUnavailable, "channel_disabled")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body")
		return
	}

	cfg := channels.WebhookConfig{}
	_ = json.Unmarshal(channel.Config, &cfg)
	if err := h.authenticate(r, body, cfg, channel.TenantID); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	payload, err := decodeBodyPayload(r.Header.Get("Content-Type"), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload")
		return
	}
	result, err := h.Pipeline.Process(r.Context(), channels.PipelineRequest{
		TenantID:  channel.TenantID,
		ChannelID: channel.ID,
		Data:      payload,
		Source:    "webhook",
	})
	if err != nil {
		if err == channels.ErrDeduped {
			writeJSON(w, http.StatusConflict, map[string]any{"status": "deduped"})
			return
		}
		if err == channels.ErrChannelDisabled {
			writeError(w, http.StatusServiceUnavailable, "channel_disabled")
			return
		}
		writeError(w, http.StatusInternalServerError, "pipeline_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"case_id": result.CaseID.String(), "event_id": result.EventID.String()})
}

func (h *WebhookHandler) authenticate(r *http.Request, body []byte, cfg channels.WebhookConfig, tenantID uuid.UUID) error {
	authType := strings.ToLower(strings.TrimSpace(cfg.AuthType))
	if authType == "" {
		authType = "none"
	}
	secret := ""
	if authType != "none" {
		if h.SecretStore == nil {
			return fmt.Errorf("secret store missing")
		}
		var err error
		secret, err = h.SecretStore.Get(r.Context(), tenantID, cfg.AuthSecret)
		if err != nil {
			return err
		}
	}
	switch authType {
	case "none":
		slog.Warn("webhook channel configured without authentication", "channel_id", r.PathValue("channel_id"))
		return nil
	case "hmac":
		header := cfg.AuthHeader
		if header == "" {
			header = "X-Signature"
		}
		received := strings.TrimSpace(r.Header.Get(header))
		if strings.HasPrefix(strings.ToLower(received), "sha256=") {
			received = strings.TrimPrefix(strings.ToLower(received), "sha256=")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(strings.ToLower(received))) {
			return fmt.Errorf("hmac mismatch")
		}
		return nil
	case "api_key":
		header := cfg.AuthHeader
		if header == "" {
			header = "X-API-Key"
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(header)), []byte(secret)) != 1 {
			return fmt.Errorf("api key mismatch")
		}
		return nil
	case "bearer":
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer"))
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
			return fmt.Errorf("bearer mismatch")
		}
		return nil
	default:
		return fmt.Errorf("unsupported auth_type")
	}
}

func decodeBodyPayload(contentType string, body []byte) ([]byte, error) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch mediaType {
	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		payload := map[string]any{}
		for key, vals := range values {
			if len(vals) == 1 {
				payload[key] = vals[0]
			} else {
				payload[key] = vals
			}
		}
		return json.Marshal(payload)
	default:
		if len(body) == 0 {
			return []byte(`{}`), nil
		}
		var js any
		if err := json.Unmarshal(body, &js); err != nil {
			return nil, err
		}
		return body, nil
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code, "code": code})
}
