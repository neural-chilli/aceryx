package backup

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Service) snapshotVault(destRoot string, tenantID *uuid.UUID) error {
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return fmt.Errorf("create temporary vault snapshot directory: %w", err)
	}

	if s.vaultPath == "" {
		return nil
	}

	source := s.vaultPath
	if tenantID != nil {
		source = filepath.Join(source, tenantID.String())
	}
	st, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat vault source %q: %w", source, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("vault source %q is not a directory", source)
	}

	if tenantID != nil {
		return copyTree(source, filepath.Join(destRoot, tenantID.String()))
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read vault root directory: %w", err)
	}
	for _, entry := range entries {
		if err := copyTree(filepath.Join(source, entry.Name()), filepath.Join(destRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source path %q: %w", src, err)
	}
	if st.IsDir() {
		if err := os.MkdirAll(dst, st.Mode().Perm()); err != nil {
			return fmt.Errorf("create destination directory %q: %w", dst, err)
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("read source directory %q: %w", src, err)
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination file parent %q: %w", dst, err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy source file %q: %w", src, err)
	}
	return nil
}

func replaceVaultContents(destRoot, srcRoot string) error {
	if strings.TrimSpace(destRoot) == "" {
		return fmt.Errorf("vault path is required for restore")
	}
	if err := os.RemoveAll(destRoot); err != nil {
		return fmt.Errorf("remove existing vault contents: %w", err)
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return fmt.Errorf("create vault destination root: %w", err)
	}
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return fmt.Errorf("read extracted vault directory: %w", err)
	}
	for _, entry := range entries {
		if err := copyTree(filepath.Join(srcRoot, entry.Name()), filepath.Join(destRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func verifyRestored(ctx context.Context, db *sql.DB, vaultPath string) (int64, int64, int, int, error) {
	var casesCount int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases`).Scan(&casesCount); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count restored cases: %w", err)
	}
	var docCount int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE deleted_at IS NULL`).Scan(&docCount); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count restored documents: %w", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT storage_uri FROM vault_documents WHERE deleted_at IS NULL`)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("load restored vault uris: %w", err)
	}
	defer func() { _ = rows.Close() }()

	uris := make([]string, 0)
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return 0, 0, 0, 0, fmt.Errorf("scan restored vault uri: %w", err)
		}
		uris = append(uris, uri)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("iterate restored vault uris: %w", err)
	}

	sampled := len(uris)
	if sampled > 25 {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		r.Shuffle(len(uris), func(i, j int) { uris[i], uris[j] = uris[j], uris[i] })
		sampled = 25
		uris = uris[:sampled]
	}

	verified := 0
	for _, uri := range uris {
		if _, err := os.Stat(filepath.Join(vaultPath, filepath.FromSlash(uri))); err == nil {
			verified++
		}
	}

	return casesCount, docCount, verified, sampled, nil
}

func countVaultFiles(vaultDir string) (int, int64, error) {
	count := 0
	var bytes int64
	err := filepath.WalkDir(vaultDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		count++
		bytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("walk vault files: %w", err)
	}
	return count, bytes, nil
}
