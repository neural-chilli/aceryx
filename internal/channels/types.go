package channels

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ChannelType string

const (
	ChannelEmail    ChannelType = "email"
	ChannelWebhook  ChannelType = "webhook"
	ChannelForm     ChannelType = "form"
	ChannelFileDrop ChannelType = "file_drop"
	ChannelPlugin   ChannelType = "plugin"
)

type EventStatus string

const (
	EventProcessed EventStatus = "processed"
	EventDeduped   EventStatus = "deduped"
	EventFailed    EventStatus = "failed"
)

type Channel struct {
	ID            uuid.UUID       `json:"id"`
	TenantID      uuid.UUID       `json:"tenant_id"`
	Name          string          `json:"name"`
	Type          ChannelType     `json:"type"`
	PluginRef     string          `json:"plugin_ref,omitempty"`
	Config        json.RawMessage `json:"config"`
	CaseTypeID    uuid.UUID       `json:"case_type_id"`
	WorkflowID    *uuid.UUID      `json:"workflow_id,omitempty"`
	AdapterConfig AdapterConfig   `json:"adapter_config"`
	DedupConfig   DedupConfig     `json:"dedup_config"`
	Enabled       bool            `json:"enabled"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type ChannelEvent struct {
	ID           uuid.UUID       `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	ChannelID    uuid.UUID       `json:"channel_id"`
	RawPayload   json.RawMessage `json:"raw_payload,omitempty"`
	Attachments  []AttachmentRef `json:"attachments,omitempty"`
	CaseID       *uuid.UUID      `json:"case_id,omitempty"`
	Status       EventStatus     `json:"status"`
	ErrorMessage string          `json:"error_message,omitempty"`
	ProcessingMS int             `json:"processing_ms"`
	CreatedAt    time.Time       `json:"created_at"`
}

type AttachmentInput struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}

type AttachmentRef struct {
	VaultID     uuid.UUID `json:"vault_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	Checksum    string    `json:"checksum"`
}

type PipelineRequest struct {
	TenantID      uuid.UUID         `json:"tenant_id"`
	ChannelID     uuid.UUID         `json:"channel_id"`
	Data          json.RawMessage   `json:"data"`
	Attachments   []AttachmentInput `json:"attachments"`
	Source        string            `json:"source"`
	ReceivedAt    time.Time         `json:"received_at"`
	ActorID       uuid.UUID         `json:"actor_id"`
	DedupOverride *DedupConfig      `json:"dedup_override,omitempty"`
}

type PipelineResult struct {
	CaseID  uuid.UUID `json:"case_id"`
	Deduped bool      `json:"deduped"`
	EventID uuid.UUID `json:"event_id"`
}

type DedupConfig struct {
	Strategy   string   `json:"strategy"`
	Fields     []string `json:"fields"`
	WindowMins int      `json:"window_mins"`
}

type AdapterConfig struct {
	Mappings []FieldMapping `json:"mappings"`
}

type FieldMapping struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Expression string `json:"expression"`
}

type EmailConfig struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	TLS                bool   `json:"tls"`
	UsernameSecret     string `json:"username_secret"`
	PasswordSecret     string `json:"password_secret"`
	Mailbox            string `json:"mailbox"`
	PollIntervalSecs   int    `json:"poll_interval_seconds"`
	MarkAsRead         bool   `json:"mark_as_read"`
	DeleteAfterProcess bool   `json:"delete_after_processing"`
}

type WebhookConfig struct {
	AuthType      string `json:"auth_type"`
	AuthSecret    string `json:"auth_secret"`
	AuthHeader    string `json:"auth_header"`
	HMACAlgorithm string `json:"hmac_algorithm"`
}

type FormConfig struct {
	RateLimitPerMinute int    `json:"rate_limit_per_minute"`
	CaptchaEnabled     bool   `json:"captcha_enabled"`
	CaptchaProvider    string `json:"captcha_provider"`
	CaptchaSecret      string `json:"captcha_secret"`
	SuccessRedirectURL string `json:"success_redirect_url"`
}

type FileDropConfig struct {
	WatchPath        string   `json:"watch_path"`
	ProcessedPath    string   `json:"processed_path"`
	FilePatterns     []string `json:"file_patterns"`
	PollIntervalSecs int      `json:"poll_interval_seconds"`
}

func (c EmailConfig) WithDefaults() EmailConfig {
	if c.Mailbox == "" {
		c.Mailbox = "INBOX"
	}
	if c.PollIntervalSecs <= 0 {
		c.PollIntervalSecs = 60
	}
	if !c.MarkAsRead {
		c.MarkAsRead = true
	}
	return c
}

func (c FormConfig) WithDefaults() FormConfig {
	if c.RateLimitPerMinute <= 0 {
		c.RateLimitPerMinute = 10
	}
	return c
}

func (c FileDropConfig) WithDefaults() FileDropConfig {
	if c.PollIntervalSecs <= 0 {
		c.PollIntervalSecs = 30
	}
	if len(c.FilePatterns) == 0 {
		c.FilePatterns = []string{"*"}
	}
	return c
}
