package server

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/neural-chilli/aceryx/api"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type manifestResponse struct {
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	StartURL        string         `json:"start_url"`
	Display         string         `json:"display"`
	BackgroundColor string         `json:"background_color"`
	ThemeColor      string         `json:"theme_color"`
	Icons           []manifestIcon `json:"icons"`
}

type manifestIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

func NewHandler(db *sql.DB, eng *engine.Engine, uiFS fs.FS) http.Handler {
	apiHandler := api.NewRouterWithServices(db, eng)
	spa := newSPAHandler(uiFS)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", apiHandler))
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	mux.HandleFunc("GET /health", forwardTo(apiHandler, "/health"))
	mux.HandleFunc("GET /healthz", forwardTo(apiHandler, "/healthz"))
	mux.HandleFunc("GET /readyz", forwardTo(apiHandler, "/readyz"))
	mux.HandleFunc("GET /metrics", forwardTo(apiHandler, "/metrics"))
	mux.HandleFunc("GET /ws", forwardTo(apiHandler, "/ws"))
	mux.HandleFunc("GET /manifest.json", manifestHandler(db))
	mux.Handle("/", spa)

	return mux
}

func forwardTo(handler http.Handler, targetPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(r.Context())
		clone.URL.Path = targetPath
		handler.ServeHTTP(w, clone)
	}
}

func manifestHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		payload := manifestResponse{
			Name:            "Aceryx",
			ShortName:       "Aceryx",
			StartURL:        "/",
			Display:         "standalone",
			BackgroundColor: "#1f2937",
			ThemeColor:      "#1f2937",
			Icons: []manifestIcon{
				{Src: "/logo-192.png", Sizes: "192x192", Type: "image/png"},
				{Src: "/logo-512.png", Sizes: "512x512", Type: "image/png"},
			},
		}

		slug := strings.TrimSpace(r.URL.Query().Get("slug"))
		if slug != "" && db != nil {
			var (
				companyName string
				primary     string
			)
			err := db.QueryRowContext(r.Context(), `
SELECT
  COALESCE(branding->>'company_name', name),
  COALESCE(branding->'colors'->>'primary', '#1f2937')
FROM tenants
WHERE slug = $1
`, slug).Scan(&companyName, &primary)
			if err == nil {
				payload.Name = companyName
				payload.ShortName = companyName
				payload.BackgroundColor = primary
				payload.ThemeColor = primary
			}
		}

		_ = json.NewEncoder(w).Encode(payload)
	}
}

func newSPAHandler(uiFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		requested := path.Clean(r.URL.Path)
		requested = strings.TrimPrefix(requested, "/")
		if requested == "" {
			requested = "index.html"
		}

		if f, err := uiFS.Open(requested); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		clone := r.Clone(r.Context())
		clone.URL.Path = "/"
		fileServer.ServeHTTP(w, clone)
	})
}
