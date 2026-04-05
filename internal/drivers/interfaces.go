package drivers

import (
	"context"
	"database/sql"
	"io"
	"time"
)

// DBDriver provides SQL driver connectivity for workflow query execution.
type DBDriver interface {
	ID() string
	DisplayName() string
	Connect(ctx context.Context, config DBConfig) (*sql.DB, error)
	Ping(ctx context.Context, db *sql.DB) error
	Close(db *sql.DB) error
}

type DBConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Database    string `yaml:"database"`
	User        string `yaml:"user"`
	Password    string `yaml:"password" secret:"true"`
	SSLMode     string `yaml:"ssl_mode"`
	MaxConns    int    `yaml:"max_conns"`
	IdleConns   int    `yaml:"idle_conns"`
	ReadOnly    bool   `yaml:"read_only"`
	TimeoutSecs int    `yaml:"timeout"`
	RowLimit    int    `yaml:"row_limit"`
}

type QueueDriver interface {
	ID() string
	DisplayName() string
	Connect(ctx context.Context, config QueueConfig) error
	Publish(ctx context.Context, topic string, message []byte, headers map[string]string) error
	Consume(ctx context.Context, topic string) (message []byte, metadata map[string]string, messageID string, err error)
	Ack(ctx context.Context, messageID string) error
	Nack(ctx context.Context, messageID string) error
	Close() error
}

type QueueConfig struct {
	Brokers       []string               `yaml:"brokers"`
	Username      string                 `yaml:"username"`
	Password      string                 `yaml:"password" secret:"true"`
	TLS           bool                   `yaml:"tls"`
	ConsumerGroup string                 `yaml:"consumer_group"`
	Extras        map[string]interface{} `yaml:"extras"`
}

type FileDriver interface {
	ID() string
	DisplayName() string
	Connect(ctx context.Context, config FileConfig) error
	List(ctx context.Context, path string) ([]FileEntry, error)
	Read(ctx context.Context, path string) (io.ReadCloser, error)
	Write(ctx context.Context, path string, data io.Reader) error
	Delete(ctx context.Context, path string) error
	Close() error
}

type FileConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password" secret:"true"`
	KeyPath  string `yaml:"key_path" secret:"true"`
	BasePath string `yaml:"base_path"`
	TLS      bool   `yaml:"tls"`
}

type FileEntry struct {
	Path    string
	Name    string
	Size    int64
	IsDir   bool
	ModTime time.Time
}

type ProtocolDriver interface {
	ID() string
	DisplayName() string
	Parse(data []byte) (map[string]interface{}, error)
	Format(data map[string]interface{}) ([]byte, error)
	Validate(data []byte) error
}

type SMTPDriver interface {
	ID() string
	DisplayName() string
	Send(ctx context.Context, config SMTPConfig, msg EmailMessage) error
}

type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password" secret:"true"`
	TLS      bool   `yaml:"tls"`
	From     string `yaml:"from"`
}

type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type EmailMessage struct {
	To          []string
	CC          []string
	Subject     string
	BodyText    string
	BodyHTML    string
	Attachments []Attachment
}

type IMAPDriver interface {
	ID() string
	DisplayName() string
	Connect(ctx context.Context, config IMAPConfig) error
	ListMailboxes(ctx context.Context) ([]string, error)
	Fetch(ctx context.Context, mailbox string, limit int) ([]IMAPMessage, error)
	MarkRead(ctx context.Context, mailbox, uid string) error
	Delete(ctx context.Context, mailbox, uid string) error
	Close() error
}

type IMAPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password" secret:"true"`
	TLS      bool   `yaml:"tls"`
}

type IMAPMessage struct {
	UID       string
	Subject   string
	From      string
	Date      time.Time
	BodyText  string
	BodyHTML  string
	Seen      bool
	RawHeader map[string]string
}
