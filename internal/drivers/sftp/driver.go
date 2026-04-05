package sftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Driver struct {
	client   *sftp.Client
	sshConn  *ssh.Client
	basePath string
}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "sftp" }
func (d *Driver) DisplayName() string { return "SFTP" }

func (d *Driver) Connect(ctx context.Context, config drivers.FileConfig) error {
	_ = ctx
	host := config.Host
	if host == "" {
		return fmt.Errorf("sftp host is required")
	}
	port := config.Port
	if port == 0 {
		port = 22
	}
	auth, err := authMethods(config)
	if err != nil {
		return err
	}
	hostKeyCallback, err := sftpHostKeyCallback()
	if err != nil {
		return err
	}
	sshCfg := &ssh.ClientConfig{
		User:            config.Username,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}
	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshCfg)
	if err != nil {
		return fmt.Errorf("connect sftp ssh: %w", err)
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("create sftp client: %w", err)
	}
	d.client = client
	d.sshConn = conn
	if strings.TrimSpace(config.BasePath) == "" {
		d.basePath = "/"
	} else {
		d.basePath = path.Clean(config.BasePath)
	}
	return nil
}

func (d *Driver) List(ctx context.Context, p string) ([]drivers.FileEntry, error) {
	_ = ctx
	full, err := d.resolve(p)
	if err != nil {
		return nil, err
	}
	entries, err := d.client.ReadDir(full)
	if err != nil {
		return nil, fmt.Errorf("sftp list: %w", err)
	}
	out := make([]drivers.FileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, drivers.FileEntry{
			Path:    path.Join(full, e.Name()),
			Name:    e.Name(),
			Size:    e.Size(),
			IsDir:   e.IsDir(),
			ModTime: e.ModTime(),
		})
	}
	return out, nil
}

func (d *Driver) Read(ctx context.Context, p string) (io.ReadCloser, error) {
	_ = ctx
	full, err := d.resolve(p)
	if err != nil {
		return nil, err
	}
	file, err := d.client.Open(full)
	if err != nil {
		return nil, fmt.Errorf("sftp read: %w", err)
	}
	return file, nil
}

func (d *Driver) Write(ctx context.Context, p string, data io.Reader) error {
	_ = ctx
	full, err := d.resolve(p)
	if err != nil {
		return err
	}
	if err := d.client.MkdirAll(path.Dir(full)); err != nil {
		return fmt.Errorf("sftp mkdirall: %w", err)
	}
	file, err := d.client.Create(full)
	if err != nil {
		return fmt.Errorf("sftp create: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := io.Copy(file, data); err != nil {
		return fmt.Errorf("sftp write: %w", err)
	}
	return nil
}

func (d *Driver) Delete(ctx context.Context, p string) error {
	_ = ctx
	full, err := d.resolve(p)
	if err != nil {
		return err
	}
	if err := d.client.Remove(full); err != nil {
		return fmt.Errorf("sftp delete: %w", err)
	}
	return nil
}

func (d *Driver) Close() error {
	if d.client != nil {
		_ = d.client.Close()
	}
	if d.sshConn != nil {
		return d.sshConn.Close()
	}
	return nil
}

func authMethods(config drivers.FileConfig) ([]ssh.AuthMethod, error) {
	if strings.TrimSpace(config.KeyPath) != "" {
		raw, err := os.ReadFile(config.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	if config.Password != "" {
		return []ssh.AuthMethod{ssh.Password(config.Password)}, nil
	}
	return nil, fmt.Errorf("sftp auth requires key_path or password")
}

func sftpHostKeyCallback() (ssh.HostKeyCallback, error) {
	paths := make([]string, 0, 2)
	if configured := strings.TrimSpace(os.Getenv("ACERYX_SFTP_KNOWN_HOSTS")); configured != "" {
		paths = append(paths, configured)
	}
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		paths = append(paths, filepath.Join(homeDir, ".ssh", "known_hosts"))
	}
	paths = append(paths, "/etc/ssh/ssh_known_hosts")

	existing := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return nil, fmt.Errorf("no known_hosts file found; set ACERYX_SFTP_KNOWN_HOSTS or create ~/.ssh/known_hosts")
	}
	callback, err := knownhosts.New(existing...)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	return callback, nil
}

func (d *Driver) resolve(p string) (string, error) {
	if d.client == nil {
		return "", fmt.Errorf("sftp not connected")
	}
	clean := path.Clean("/" + strings.TrimSpace(p))
	if clean == "/" {
		return d.basePath, nil
	}
	full := path.Clean(path.Join(d.basePath, clean))
	base := path.Clean(d.basePath)
	if full != base && !strings.HasPrefix(full, base+"/") {
		return "", fmt.Errorf("path escapes base path")
	}
	return full, nil
}
