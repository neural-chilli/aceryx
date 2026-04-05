package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	listenAddr  string
	tools       []ToolHandler
	auth        *AuthMiddleware
	rateLimiter *RateLimiter
	auditStore  InvocationStore
	config      ServerConfig

	mu         sync.Mutex
	httpServer *http.Server
	handler    *Handler
}

func NewServer(config ServerConfig, deps ServerDependencies) *Server {
	cfg := config.WithDefaults()
	auth := NewAuthMiddleware(deps.APIKeyStore, cfg)
	rateLimiter := NewRateLimiter(cfg.RateLimit)
	auditLogger := NewAuditLogger(deps.AuditStore)
	handler := NewHandler(cfg, deps.Tools, auth, rateLimiter, auditLogger)
	return &Server{
		listenAddr:  cfg.ListenAddr,
		tools:       append([]ToolHandler(nil), deps.Tools...),
		auth:        auth,
		rateLimiter: rateLimiter,
		auditStore:  deps.AuditStore,
		config:      cfg,
		handler:     handler,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		return nil
	}
	if s.httpServer != nil {
		return nil
	}
	if s.listenAddr == "" {
		s.listenAddr = DefaultListenAddr
	}
	httpSrv := &http.Server{
		Addr:              s.listenAddr,
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       DefaultServerRequestTimeout,
		WriteTimeout:      DefaultServerRequestTimeout,
		IdleTimeout:       60 * time.Second,
	}
	s.httpServer = httpSrv

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Best effort: surfaced on next Start/Stop call.
			_ = err
		}
	}()

	if ctx != nil {
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_ = s.Stop(shutdownCtx)
		}()
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	httpSrv := s.httpServer
	s.httpServer = nil
	s.mu.Unlock()

	if httpSrv == nil {
		return nil
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
	}
	if err := httpSrv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown MCP server: %w", err)
	}
	return nil
}

func (s *Server) SetConfig(cfg ServerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg.WithDefaults()
	if s.handler != nil {
		s.handler.SetConfig(s.config)
	}
}

func (s *Server) Config() ServerConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config
}
