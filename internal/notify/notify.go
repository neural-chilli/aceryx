package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
)

type Service struct {
	db  *sql.DB
	hub *Hub
}

func NewService(db *sql.DB, hub *Hub) *Service {
	return &Service{db: db, hub: hub}
}

func (s *Service) NotifyUser(ctx context.Context, principalID uuid.UUID, payload map[string]any) error {
	if s.hub == nil {
		return nil
	}
	return s.hub.Send(principalID, payload)
}

func (s *Service) NotifyRole(ctx context.Context, tenantID uuid.UUID, role string, payload map[string]any) error {
	if s.hub == nil {
		return nil
	}
	if role == "*" {
		return s.hub.Broadcast(tenantID, payload)
	}
	return s.hub.SendToRole(ctx, tenantID, role, payload)
}

func (s *Service) SendEmail(_ context.Context, to string, subject string, body string) {
	log.Printf("notify email to=%s subject=%s body=%s", to, subject, body)
}

func MarshalPayload(payload map[string]any) []byte {
	raw, err := json.Marshal(payload)
	if err != nil {
		return []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error()))
	}
	return raw
}
