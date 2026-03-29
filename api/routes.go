package api

import (
	"net/http"

	"github.com/neural-chilli/aceryx/api/handlers"
)

// NewRouter creates and configures the HTTP router.
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)

	// Case endpoints
	// mux.HandleFunc("GET /cases", handlers.ListCases)
	// mux.HandleFunc("POST /cases", handlers.CreateCase)
	// mux.HandleFunc("GET /cases/{id}", handlers.GetCase)

	// Task endpoints
	// mux.HandleFunc("GET /tasks", handlers.ListTasks)
	// mux.HandleFunc("POST /tasks/{id}/claim", handlers.ClaimTask)
	// mux.HandleFunc("POST /tasks/{id}/complete", handlers.CompleteTask)

	// Workflow endpoints
	// mux.HandleFunc("GET /workflows", handlers.ListWorkflows)
	// mux.HandleFunc("POST /workflows/{id}/publish", handlers.PublishWorkflow)

	return mux
}
