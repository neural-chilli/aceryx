package channels

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type fakeIMAP struct {
	connectCalls  int
	fetchCalls    int
	markReadCalls int
	closeCalls    int
}

func (f *fakeIMAP) ID() string          { return "imap" }
func (f *fakeIMAP) DisplayName() string { return "IMAP" }
func (f *fakeIMAP) Connect(context.Context, drivers.IMAPConfig) error {
	f.connectCalls++
	return nil
}
func (f *fakeIMAP) ListMailboxes(context.Context) ([]string, error) { return []string{"INBOX"}, nil }
func (f *fakeIMAP) Fetch(context.Context, string, int) ([]drivers.IMAPMessage, error) {
	f.fetchCalls++
	return []drivers.IMAPMessage{{
		UID:      "1",
		Subject:  "App-123",
		BodyText: "body",
		From:     "sender@example.com",
		Seen:     false,
		RawHeader: map[string]string{
			"Message-ID": "m-1",
		},
	}}, nil
}
func (f *fakeIMAP) MarkRead(context.Context, string, string) error {
	f.markReadCalls++
	return nil
}
func (f *fakeIMAP) Delete(context.Context, string, string) error { return nil }
func (f *fakeIMAP) Close() error {
	f.closeCalls++
	return nil
}

type fakeSecretStore struct{}

func (fakeSecretStore) Get(context.Context, uuid.UUID, string) (string, error) { return "secret", nil }

type noopWorkflowRunner struct{}

func (noopWorkflowRunner) EvaluateDAG(context.Context, uuid.UUID) error { return nil }

func TestEmailChannelPollCreatesCaseAndMarksRead(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	channelID := uuid.New()
	store := &fakeChannelStore{
		channel: &Channel{
			ID:            channelID,
			TenantID:      tenantID,
			Type:          ChannelEmail,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(noopWorkflowRunner{}, store, nil)
	imap := &fakeIMAP{}
	runner := &EmailChannelRunner{
		ChannelID:   channelID,
		TenantID:    tenantID,
		Config:      EmailConfig{Host: "imap.example.com", Port: 993, TLS: true, Mailbox: "INBOX", MarkAsRead: true}.WithDefaults(),
		Pipeline:    pipeline,
		IMAP:        imap,
		SecretStore: fakeSecretStore{},
	}

	runner.pollOnce(context.Background(), runner.Config)

	if len(store.cases) != 1 {
		t.Fatalf("expected case to be created from email, got %d", len(store.cases))
	}
	if len(store.events) != 1 || store.events[0].Status != EventProcessed {
		t.Fatalf("expected one processed event, got %#v", store.events)
	}
	if imap.markReadCalls != 1 {
		t.Fatalf("expected mark-as-read call")
	}
	if imap.connectCalls != 1 || imap.closeCalls != 1 {
		t.Fatalf("expected connect-per-poll and close-per-poll, got connect=%d close=%d", imap.connectCalls, imap.closeCalls)
	}
}

func TestFileDropPollMatchingMovesToProcessed(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	watchPath := filepath.Join(base, "incoming")
	processedPath := filepath.Join(watchPath, "processed")
	if err := os.MkdirAll(processedPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(watchPath, "application.pdf")
	if err := os.WriteFile(src, []byte("pdf-data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := &fakeChannelStore{
		channel: &Channel{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Type:          ChannelFileDrop,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(noopWorkflowRunner{}, store, &fakeAttachmentService{})
	runner := &FileDropChannelRunner{
		ChannelID: store.channel.ID,
		TenantID:  store.channel.TenantID,
		Config: FileDropConfig{
			WatchPath:     watchPath,
			ProcessedPath: processedPath,
			FilePatterns:  []string{"*.pdf"},
		},
		Pipeline: pipeline,
	}

	runner.pollOnce(context.Background(), runner.Config.WithDefaults())

	if len(store.cases) != 1 {
		t.Fatalf("expected one case, got %d", len(store.cases))
	}
	if _, err := os.Stat(filepath.Join(processedPath, "application.pdf")); err != nil {
		t.Fatalf("expected file moved to processed path: %v", err)
	}
}

func TestFileDropPollNonMatchingIgnored(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	watchPath := filepath.Join(base, "incoming")
	processedPath := filepath.Join(watchPath, "processed")
	if err := os.MkdirAll(processedPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(watchPath, "notes.txt")
	if err := os.WriteFile(src, []byte("note"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := &fakeChannelStore{
		channel: &Channel{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Type:          ChannelFileDrop,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(noopWorkflowRunner{}, store, nil)
	runner := &FileDropChannelRunner{
		ChannelID: store.channel.ID,
		TenantID:  store.channel.TenantID,
		Config: FileDropConfig{
			WatchPath:     watchPath,
			ProcessedPath: processedPath,
			FilePatterns:  []string{"*.pdf"},
		},
		Pipeline: pipeline,
	}

	runner.pollOnce(context.Background(), runner.Config.WithDefaults())
	if len(store.cases) != 0 {
		t.Fatalf("expected no case for non-matching file")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("expected file to remain untouched: %v", err)
	}
}

func TestFileDropPollFailureMovesToErrors(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	watchPath := filepath.Join(base, "incoming")
	processedPath := filepath.Join(watchPath, "processed")
	if err := os.MkdirAll(processedPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(watchPath, "application.pdf")
	if err := os.WriteFile(src, []byte("pdf-data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := &fakeChannelStore{
		channel: &Channel{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Type:          ChannelFileDrop,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(noopWorkflowRunner{}, store, &fakeAttachmentService{fail: true})
	runner := &FileDropChannelRunner{
		ChannelID: store.channel.ID,
		TenantID:  store.channel.TenantID,
		Config: FileDropConfig{
			WatchPath:     watchPath,
			ProcessedPath: processedPath,
			FilePatterns:  []string{"*.pdf"},
		},
		Pipeline: pipeline,
	}

	runner.pollOnce(context.Background(), runner.Config.WithDefaults())
	if _, err := os.Stat(filepath.Join(watchPath, "errors", "application.pdf")); err != nil {
		t.Fatalf("expected file moved to errors on failure: %v", err)
	}
}

func TestIsSafeSubpath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	if !isSafeSubpath(base, filepath.Join(base, "a", "b")) {
		t.Fatalf("expected nested path to be safe")
	}
	if isSafeSubpath(base, filepath.Join(base, "..", "escape")) {
		t.Fatalf("expected traversal path to be unsafe")
	}
}

func TestTruncateRawPayloadOverLimitProducesMarker(t *testing.T) {
	t.Parallel()
	payload := map[string]any{"data": strings.Repeat("x", 1024*1024)}
	raw, _ := json.Marshal(payload)
	out := truncateRawPayload(raw)
	if len(out) > 1024*1024 {
		t.Fatalf("expected truncated payload <= 1MB")
	}
}
