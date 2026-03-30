package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type fakeHealthState struct {
	pingErr        error
	migrationCount int
	migrationErr   error
}

type fakeHealthDriver struct{}

type fakeHealthConn struct {
	state *fakeHealthState
}

type fakeHealthRows struct {
	cols []string
	vals [][]driver.Value
	i    int
}

var (
	fakeHealthDrivers sync.Map
)

func init() {
	sql.Register("fakehealth", fakeHealthDriver{})
}

func (d fakeHealthDriver) Open(name string) (driver.Conn, error) {
	v, ok := fakeHealthDrivers.Load(name)
	if !ok {
		return nil, errors.New("missing fake health state")
	}
	return &fakeHealthConn{state: v.(*fakeHealthState)}, nil
}

func (c *fakeHealthConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (c *fakeHealthConn) Close() error              { return nil }
func (c *fakeHealthConn) Begin() (driver.Tx, error) { return nil, errors.New("not implemented") }

func (c *fakeHealthConn) Ping(context.Context) error {
	return c.state.pingErr
}

func (c *fakeHealthConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.state.migrationErr != nil {
		return nil, c.state.migrationErr
	}
	if query == "SELECT COUNT(*) FROM schema_migrations" {
		return &fakeHealthRows{cols: []string{"count"}, vals: [][]driver.Value{{int64(c.state.migrationCount)}}, i: -1}, nil
	}
	return nil, errors.New("unexpected query")
}

func (r *fakeHealthRows) Columns() []string { return r.cols }
func (r *fakeHealthRows) Close() error      { return nil }
func (r *fakeHealthRows) Next(dest []driver.Value) error {
	r.i++
	if r.i >= len(r.vals) {
		return io.EOF
	}
	copy(dest, r.vals[r.i])
	return nil
}

func openFakeHealthDB(t *testing.T, state *fakeHealthState) *sql.DB {
	t.Helper()
	name := t.Name()
	fakeHealthDrivers.Store(name, state)
	t.Cleanup(func() { fakeHealthDrivers.Delete(name) })
	db, err := sql.Open("fakehealth", name)
	if err != nil {
		t.Fatalf("open fake health db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestHealth_HealthyWhenPostgresConnected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ACERYX_VAULT_ROOT", dir)
	db := openFakeHealthDB(t, &fakeHealthState{migrationCount: 1})
	h := NewHealthHandlers(db, nil, nil)

	rr := httptest.NewRecorder()
	h.Health(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "healthy" {
		t.Fatalf("expected healthy status, got %v", body["status"])
	}
}

func TestHealth_DegradedWhenPostgresDisconnected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ACERYX_VAULT_ROOT", dir)
	db := openFakeHealthDB(t, &fakeHealthState{pingErr: errors.New("connection refused")})
	h := NewHealthHandlers(db, nil, nil)

	rr := httptest.NewRecorder()
	h.Health(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestReadinessBeforeAndAfterMigrations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ACERYX_VAULT_ROOT", dir)

	notReadyDB := openFakeHealthDB(t, &fakeHealthState{migrationCount: 0})
	notReady := NewHealthHandlers(notReadyDB, nil, nil)
	rr := httptest.NewRecorder()
	notReady.Readiness(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before migrations, got %d", rr.Code)
	}

	readyDB := openFakeHealthDB(t, &fakeHealthState{migrationCount: 3})
	ready := NewHealthHandlers(readyDB, nil, nil)
	rr2 := httptest.NewRecorder()
	ready.Readiness(rr2, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 after migrations, got %d", rr2.Code)
	}
}

func TestHealth_VaultUnhealthyWhenPathInvalid(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "missing", "vault")
	_ = os.RemoveAll(badPath)
	t.Setenv("ACERYX_VAULT_ROOT", badPath)
	db := openFakeHealthDB(t, &fakeHealthState{migrationCount: 1})
	h := NewHealthHandlers(db, nil, nil)

	rr := httptest.NewRecorder()
	h.Health(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unhealthy vault path, got %d", rr.Code)
	}
}
