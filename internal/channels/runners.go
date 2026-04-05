package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type EmailChannelRunner struct {
	ChannelID   uuid.UUID
	TenantID    uuid.UUID
	Config      EmailConfig
	Pipeline    *Pipeline
	IMAP        drivers.IMAPDriver
	SecretStore connectors.SecretStore

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (ec *EmailChannelRunner) Start(ctx context.Context) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.cancel != nil {
		return nil
	}
	cfg := ec.Config.WithDefaults()
	runCtx, cancel := context.WithCancel(ctx)
	ec.cancel = cancel
	go ec.loop(runCtx, cfg)
	return nil
}

func (ec *EmailChannelRunner) Stop() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.cancel != nil {
		ec.cancel()
		ec.cancel = nil
	}
	if ec.IMAP != nil {
		_ = ec.IMAP.Close()
	}
	return nil
}

func (ec *EmailChannelRunner) loop(ctx context.Context, cfg EmailConfig) {
	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSecs) * time.Second)
	defer ticker.Stop()
	for {
		ec.pollOnce(ctx, cfg)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (ec *EmailChannelRunner) pollOnce(ctx context.Context, cfg EmailConfig) {
	if ec.IMAP == nil || ec.Pipeline == nil || ec.SecretStore == nil {
		return
	}
	username, err := ec.SecretStore.Get(ctx, ec.TenantID, cfg.UsernameSecret)
	if err != nil {
		slog.Error("email channel failed to resolve username secret", "channel_id", ec.ChannelID, "error", err)
		return
	}
	password, err := ec.SecretStore.Get(ctx, ec.TenantID, cfg.PasswordSecret)
	if err != nil {
		slog.Error("email channel failed to resolve password secret", "channel_id", ec.ChannelID, "error", err)
		return
	}
	if err := ec.IMAP.Connect(ctx, drivers.IMAPConfig{Host: cfg.Host, Port: cfg.Port, TLS: cfg.TLS, Username: username, Password: password}); err != nil {
		slog.Error("email channel IMAP connect failed", "channel_id", ec.ChannelID, "error", err)
		return
	}
	messages, err := ec.IMAP.Fetch(ctx, cfg.Mailbox, 50)
	if err != nil {
		slog.Error("email channel IMAP fetch failed", "channel_id", ec.ChannelID, "error", err)
		return
	}
	for _, msg := range messages {
		if msg.Seen {
			continue
		}
		payload := map[string]any{
			"subject":    msg.Subject,
			"body_text":  msg.BodyText,
			"body_html":  msg.BodyHTML,
			"from":       msg.From,
			"date":       msg.Date,
			"message_id": strings.TrimSpace(msg.RawHeader["Message-ID"]),
		}
		raw, _ := json.Marshal(payload)
		_, err := ec.Pipeline.Process(ctx, PipelineRequest{TenantID: ec.TenantID, ChannelID: ec.ChannelID, Data: raw, Source: "email"})
		if err != nil {
			slog.Error("email channel pipeline failure", "channel_id", ec.ChannelID, "error", err)
			continue
		}
		if cfg.MarkAsRead {
			_ = ec.IMAP.MarkRead(ctx, cfg.Mailbox, msg.UID)
		}
		if cfg.DeleteAfterProcess {
			_ = ec.IMAP.Delete(ctx, cfg.Mailbox, msg.UID)
		}
	}
}

type FileDropChannelRunner struct {
	ChannelID uuid.UUID
	TenantID  uuid.UUID
	Config    FileDropConfig
	Pipeline  *Pipeline

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (fd *FileDropChannelRunner) Start(ctx context.Context) error {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	if fd.cancel != nil {
		return nil
	}
	cfg := fd.Config.WithDefaults()
	if err := os.MkdirAll(cfg.ProcessedPath, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfg.WatchPath, "errors"), 0o755); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	fd.cancel = cancel
	go fd.loop(runCtx, cfg)
	return nil
}

func (fd *FileDropChannelRunner) Stop() error {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	if fd.cancel != nil {
		fd.cancel()
		fd.cancel = nil
	}
	return nil
}

func (fd *FileDropChannelRunner) loop(ctx context.Context, cfg FileDropConfig) {
	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSecs) * time.Second)
	defer ticker.Stop()
	for {
		fd.pollOnce(ctx, cfg)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (fd *FileDropChannelRunner) pollOnce(ctx context.Context, cfg FileDropConfig) {
	if fd.Pipeline == nil {
		return
	}
	entries, err := os.ReadDir(cfg.WatchPath)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !matchesPatterns(name, cfg.FilePatterns) {
			continue
		}
		src := filepath.Join(cfg.WatchPath, name)
		dst := filepath.Join(cfg.ProcessedPath, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		contentType := mime.TypeByExtension(filepath.Ext(name))
		payload := map[string]any{"filename": name, "size": len(data), "content_type": contentType}
		raw, _ := json.Marshal(payload)
		_, err = fd.Pipeline.Process(ctx, PipelineRequest{
			TenantID:  fd.TenantID,
			ChannelID: fd.ChannelID,
			Data:      raw,
			Source:    "file_drop",
			Attachments: []AttachmentInput{{
				Filename:    name,
				ContentType: contentType,
				Data:        data,
			}},
		})
		if err != nil {
			_ = moveToError(cfg.WatchPath, src, err)
			continue
		}
		_ = os.Rename(src, dst)
	}
}

func matchesPatterns(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		match, _ := filepath.Match(strings.TrimSpace(pattern), name)
		if match {
			return true
		}
	}
	return false
}

func moveToError(watchPath, src string, cause error) error {
	errorDir := filepath.Join(watchPath, "errors")
	if err := os.MkdirAll(errorDir, 0o755); err != nil {
		return err
	}
	base := filepath.Base(src)
	dst := filepath.Join(errorDir, base)
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	logPath := filepath.Join(errorDir, base+".error.log")
	return os.WriteFile(logPath, []byte(fmt.Sprintf("%s\n", cause.Error())), fs.FileMode(0o644))
}
