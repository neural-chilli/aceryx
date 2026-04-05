package form

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/channels"
	"golang.org/x/time/rate"
)

type CaseTypeStore interface {
	GetFormSchema(ctx context.Context, tenantID, caseTypeID uuid.UUID) (json.RawMessage, error)
}

type FormHandler struct {
	ChannelStore  channels.ChannelStore
	Pipeline      *channels.Pipeline
	CaseTypeStore CaseTypeStore
	HTTPClient    *http.Client

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewFormHandler(store channels.ChannelStore, pipeline *channels.Pipeline, caseTypes CaseTypeStore) *FormHandler {
	return &FormHandler{ChannelStore: store, Pipeline: pipeline, CaseTypeStore: caseTypes, HTTPClient: &http.Client{Timeout: 10 * time.Second}, limiters: map[string]*rate.Limiter{}}
}

func (h *FormHandler) ServeForm(w http.ResponseWriter, r *http.Request) {
	channel, tenantID, err := h.loadChannel(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_channel")
		return
	}
	if !channel.Enabled {
		writeError(w, http.StatusServiceUnavailable, "channel_disabled")
		return
	}
	if h.CaseTypeStore == nil {
		writeError(w, http.StatusServiceUnavailable, "case_type_store_unavailable")
		return
	}
	schema, err := h.CaseTypeStore.GetFormSchema(r.Context(), tenantID, channel.CaseTypeID)
	if err != nil {
		writeError(w, http.StatusNotFound, "case_type_schema_not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel_id": channel.ID.String(), "schema": json.RawMessage(schema)})
}

func (h *FormHandler) SubmitForm(w http.ResponseWriter, r *http.Request) {
	channel, _, err := h.loadChannel(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_channel")
		return
	}
	if !channel.Enabled {
		writeError(w, http.StatusServiceUnavailable, "channel_disabled")
		return
	}
	cfg := channels.FormConfig{}
	_ = json.Unmarshal(channel.Config, &cfg)
	cfg = cfg.WithDefaults()

	if !h.allowIP(r, cfg.RateLimitPerMinute) {
		writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded")
		return
	}
	if cfg.CaptchaEnabled {
		token := strings.TrimSpace(r.FormValue("captcha_token"))
		if err := h.verifyCaptcha(r, cfg, token); err != nil {
			writeError(w, http.StatusBadRequest, "captcha_invalid")
			return
		}
	}

	payload, attachments, err := parseFormPayload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_form")
		return
	}
	result, err := h.Pipeline.Process(r.Context(), channels.PipelineRequest{
		TenantID:    channel.TenantID,
		ChannelID:   channel.ID,
		Data:        payload,
		Attachments: attachments,
		Source:      "form",
	})
	if err != nil {
		if err == channels.ErrDeduped {
			writeJSON(w, http.StatusConflict, map[string]any{"status": "deduped"})
			return
		}
		writeError(w, http.StatusInternalServerError, "pipeline_failed")
		return
	}
	if cfg.SuccessRedirectURL != "" {
		http.Redirect(w, r, cfg.SuccessRedirectURL, http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "submitted", "case_id": result.CaseID.String()})
}

func (h *FormHandler) verifyCaptcha(r *http.Request, cfg channels.FormConfig, token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("captcha token required")
	}
	if strings.ToLower(strings.TrimSpace(cfg.CaptchaProvider)) != "hcaptcha" {
		return fmt.Errorf("unsupported captcha provider")
	}
	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	form := url.Values{}
	form.Set("secret", cfg.CaptchaSecret)
	form.Set("response", token)
	resp, err := client.PostForm("https://hcaptcha.com/siteverify", form)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("captcha verification failed")
	}
	return nil
}

func (h *FormHandler) loadChannel(r *http.Request) (*channels.Channel, uuid.UUID, error) {
	if h.ChannelStore == nil {
		return nil, uuid.Nil, fmt.Errorf("channel store unavailable")
	}
	channelID, err := uuid.Parse(strings.TrimSpace(r.PathValue("channel_id")))
	if err != nil {
		return nil, uuid.Nil, err
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("tenant_id")))
	if err != nil {
		return nil, uuid.Nil, err
	}
	channel, err := h.ChannelStore.Get(r.Context(), tenantID, channelID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	return channel, tenantID, nil
}

func (h *FormHandler) allowIP(r *http.Request, perMinute int) bool {
	if perMinute <= 0 {
		perMinute = 10
	}
	ip := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if ip == "" {
		ip = r.RemoteAddr
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.limiters == nil {
		h.limiters = map[string]*rate.Limiter{}
	}
	lim := h.limiters[ip]
	if lim == nil {
		lim = rate.NewLimiter(rate.Every(time.Minute/time.Duration(perMinute)), perMinute)
		h.limiters[ip] = lim
	}
	return lim.Allow()
}

func parseFormPayload(r *http.Request) ([]byte, []channels.AttachmentInput, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			return nil, nil, err
		}
		payload := map[string]any{}
		for key, vals := range r.MultipartForm.Value {
			if len(vals) == 1 {
				payload[key] = vals[0]
			} else {
				payload[key] = vals
			}
		}
		attachments := make([]channels.AttachmentInput, 0)
		for field, fhs := range r.MultipartForm.File {
			for _, fh := range fhs {
				file, err := fh.Open()
				if err != nil {
					return nil, nil, err
				}
				data, err := io.ReadAll(file)
				_ = file.Close()
				if err != nil {
					return nil, nil, err
				}
				attachments = append(attachments, channels.AttachmentInput{Filename: fh.Filename, ContentType: fh.Header.Get("Content-Type"), Data: data})
				_ = field
			}
		}
		raw, _ := json.Marshal(payload)
		return raw, attachments, nil
	}
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			return nil, nil, err
		}
		payload := map[string]any{}
		for k, vals := range r.PostForm {
			if len(vals) == 1 {
				payload[k] = vals[0]
			} else {
				payload[k] = vals
			}
		}
		raw, _ := json.Marshal(payload)
		return raw, nil, nil
	}
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, r.Body); err != nil {
		return nil, nil, err
	}
	if buf.Len() == 0 {
		return []byte(`{}`), nil, nil
	}
	var out any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), nil, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code, "code": code})
}
