package assistant

import (
	"time"

	"github.com/google/uuid"
)

const (
	ModeDescribe     = "describe"
	ModeRefactor     = "refactor"
	ModeExplain      = "explain"
	ModeTestGenerate = "test_generate"
)

type Session struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	UserID      uuid.UUID `json:"user_id"`
	PageContext string    `json:"page_context"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Message struct {
	ID         uuid.UUID `json:"id"`
	SessionID  uuid.UUID `json:"session_id"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	Mode       string    `json:"mode,omitempty"`
	YAMLBefore string    `json:"yaml_before,omitempty"`
	YAMLAfter  string    `json:"yaml_after,omitempty"`
	Diff       string    `json:"diff,omitempty"`
	Applied    bool      `json:"applied"`
	ModelUsed  string    `json:"model_used,omitempty"`
	TokensUsed int       `json:"tokens_used,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type DiffRecord struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	WorkflowID uuid.UUID  `json:"workflow_id"`
	MessageID  uuid.UUID  `json:"message_id"`
	UserID     uuid.UUID  `json:"user_id"`
	Prompt     string     `json:"prompt"`
	Diff       string     `json:"diff"`
	Applied    bool       `json:"applied"`
	AppliedAt  *time.Time `json:"applied_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type SessionWithMessages struct {
	Session  Session   `json:"session"`
	Messages []Message `json:"messages"`
}

type CreateSessionRequest struct {
	PageContext string `json:"page_context"`
}

type MessageRequest struct {
	SessionID   *uuid.UUID       `json:"session_id,omitempty"`
	Content     string           `json:"content"`
	Mode        string           `json:"mode,omitempty"`
	PageContext string           `json:"page_context,omitempty"`
	WorkflowID  *uuid.UUID       `json:"workflow_id,omitempty"`
	PromptPack  *PromptPackInput `json:"prompt_pack,omitempty"`
}

type PromptPackInput struct {
	FrontendContext string `json:"frontend_context,omitempty"`
}

type MessageResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	MessageID uuid.UUID `json:"message_id"`
	Content   string    `json:"content"`
	YAMLAfter string    `json:"yaml_after,omitempty"`
	Diff      string    `json:"diff,omitempty"`
	Applied   bool      `json:"applied"`
}
