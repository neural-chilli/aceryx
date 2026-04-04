package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

func TestHubWebSocketValidationAndSend(t *testing.T) {
	principalID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	hub := NewHub(nil, func(_ context.Context, token string) (uuid.UUID, uuid.UUID, error) {
		if token != "valid-token" {
			return uuid.Nil, uuid.Nil, context.Canceled
		}
		return principalID, tenantID, nil
	})

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	wsURL := "ws://" + u.Host + "/ws?token=valid-token"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial valid websocket: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()

	invalidURL := "ws://" + u.Host + "/ws?token=invalid"
	if _, _, err := websocket.Dial(ctx, invalidURL, nil); err == nil {
		t.Fatal("expected invalid websocket token to fail")
	}

	if err := hub.Send(principalID, map[string]any{"type": "task_update", "action": "created"}); err != nil {
		t.Fatalf("hub send: %v", err)
	}
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()
	_, payload, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read websocket payload: %v", err)
	}
	if string(payload) == "" {
		t.Fatal("expected websocket payload")
	}
}

func TestHubMultipleConnectionsAndCleanup(t *testing.T) {
	principalID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tenantID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	hub := NewHub(nil, func(_ context.Context, _ string) (uuid.UUID, uuid.UUID, error) {
		return principalID, tenantID, nil
	})

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	wsURL := "ws://" + u.Host + "/ws?token=ok"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn1, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket conn1: %v", err)
	}
	defer func() { _ = conn1.Close(websocket.StatusNormalClosure, "bye") }()
	conn2, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket conn2: %v", err)
	}
	defer func() { _ = conn2.Close(websocket.StatusNormalClosure, "bye") }()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	for hub.TotalConnections() < 2 {
		select {
		case <-waitCtx.Done():
			t.Fatalf("expected 2 websocket connections, got %d", hub.TotalConnections())
		case <-time.After(10 * time.Millisecond):
		}
	}

	if err := hub.Send(principalID, map[string]any{"type": "task_update", "action": "created"}); err != nil {
		t.Fatalf("hub send both: %v", err)
	}
	read := func(conn *websocket.Conn) {
		t.Helper()
		rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rcancel()
		if _, _, err := conn.Read(rctx); err != nil {
			t.Fatalf("read websocket payload: %v", err)
		}
	}
	read(conn1)
	read(conn2)

	_ = conn1.Close(websocket.StatusNormalClosure, "bye")
	time.Sleep(100 * time.Millisecond)
	if err := hub.Send(principalID, map[string]any{"type": "task_update", "action": "claimed"}); err != nil {
		t.Fatalf("hub send after one disconnect: %v", err)
	}
	rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rcancel()
	if _, _, err := conn2.Read(rctx); err != nil {
		t.Fatalf("read from remaining websocket connection: %v", err)
	}
}
