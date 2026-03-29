package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

type TokenValidator func(ctx context.Context, token string) (principalID uuid.UUID, tenantID uuid.UUID, err error)

type connMeta struct {
	principalID uuid.UUID
	tenantID    uuid.UUID
}

type Hub struct {
	db          *sql.DB
	validate    TokenValidator
	mu          sync.RWMutex
	byPrincipal map[uuid.UUID]map[*websocket.Conn]struct{}
	byConn      map[*websocket.Conn]connMeta
}

func NewHub(db *sql.DB, validate TokenValidator) *Hub {
	return &Hub{
		db:          db,
		validate:    validate,
		byPrincipal: map[uuid.UUID]map[*websocket.Conn]struct{}{},
		byConn:      map[*websocket.Conn]connMeta{},
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		hdr := r.Header.Get("Authorization")
		if len(hdr) > 7 && hdr[:7] == "Bearer " {
			token = hdr[7:]
		}
	}
	if h.validate == nil || token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	principalID, tenantID, err := h.validate(r.Context(), token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	h.add(principalID, tenantID, conn)

	go h.readLoop(conn)
}

func (h *Hub) readLoop(conn *websocket.Conn) {
	defer h.remove(conn)
	ctx := context.Background()
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return
		}
	}
}

func (h *Hub) add(principalID, tenantID uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.byPrincipal[principalID]; !ok {
		h.byPrincipal[principalID] = map[*websocket.Conn]struct{}{}
	}
	h.byPrincipal[principalID][conn] = struct{}{}
	h.byConn[conn] = connMeta{principalID: principalID, tenantID: tenantID}
}

func (h *Hub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	meta, ok := h.byConn[conn]
	if !ok {
		return
	}
	delete(h.byConn, conn)
	if m, ok := h.byPrincipal[meta.principalID]; ok {
		delete(m, conn)
		if len(m) == 0 {
			delete(h.byPrincipal, meta.principalID)
		}
	}
	_ = conn.Close(websocket.StatusNormalClosure, "closed")
}

func (h *Hub) Send(principalID uuid.UUID, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	h.mu.RLock()
	conns := make([]*websocket.Conn, 0)
	for c := range h.byPrincipal[principalID] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := c.Write(ctx, websocket.MessageText, raw)
		cancel()
		if err != nil {
			h.remove(c)
		}
	}
	return nil
}

func (h *Hub) SendToRole(ctx context.Context, tenantID uuid.UUID, role string, payload map[string]any) error {
	rows, err := h.db.QueryContext(ctx, `
SELECT p.id
FROM principals p
JOIN principal_roles pr ON pr.principal_id = p.id
JOIN roles r ON r.id = pr.role_id
WHERE p.tenant_id = $1 AND p.status='active' AND r.name = $2
`, tenantID, role)
	if err != nil {
		return fmt.Errorf("query principals by role for ws send: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var principalID uuid.UUID
		if err := rows.Scan(&principalID); err != nil {
			return err
		}
		_ = h.Send(principalID, payload)
	}
	return rows.Err()
}

func (h *Hub) Broadcast(tenantID uuid.UUID, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	h.mu.RLock()
	toSend := make([]*websocket.Conn, 0)
	for conn, meta := range h.byConn {
		if meta.tenantID == tenantID {
			toSend = append(toSend, conn)
		}
	}
	h.mu.RUnlock()
	for _, c := range toSend {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		werr := c.Write(ctx, websocket.MessageText, raw)
		cancel()
		if werr != nil {
			h.remove(c)
		}
	}
	return nil
}

func DefaultTokenValidator(authenticator func(ctx context.Context, token string) (uuid.UUID, uuid.UUID, error)) TokenValidator {
	return func(ctx context.Context, token string) (uuid.UUID, uuid.UUID, error) {
		if authenticator == nil {
			return uuid.Nil, uuid.Nil, errors.New("missing authenticator")
		}
		return authenticator(ctx, token)
	}
}
