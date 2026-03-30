package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerServesStaticAndFallsBackToIndex(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":     {Data: []byte("<html><body>app</body></html>")},
		"assets/app.js":  {Data: []byte("console.log('ok')")},
		"assets/app.css": {Data: []byte("body{}")},
		"logo-192.png":   {Data: []byte("png")},
		"manifest.json":  {Data: []byte("{}")},
		"favicon.svg":    {Data: []byte("<svg></svg>")},
	}
	var uiFS fs.FS = ui
	h := newSPAHandler(uiFS)

	t.Run("serves existing static file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "console.log") {
			t.Fatalf("expected js body, got %q", rr.Body.String())
		}
	})

	t.Run("falls back to index for client-side route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/cases/123", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "<html>") {
			t.Fatalf("expected index html body, got %q", rr.Body.String())
		}
	})
}
