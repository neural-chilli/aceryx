package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type extractedArchive struct {
	dir       string
	meta      Metadata
	dumpPath  string
	vaultPath string
}

func extractArchive(inputPath string) (extractedArchive, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return extractedArchive{}, fmt.Errorf("open backup archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return extractedArchive{}, fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tmpDir, err := os.MkdirTemp("", "aceryx-restore-*")
	if err != nil {
		return extractedArchive{}, fmt.Errorf("create restore temp dir: %w", err)
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("read archive: %w", err)
		}
		if hdr == nil {
			continue
		}

		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("archive contains invalid path %q", hdr.Name)
		}

		targetPath := filepath.Join(tmpDir, clean)
		if !strings.HasPrefix(targetPath, tmpDir) {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("archive path escapes extraction root %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create extracted directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create extracted file parent: %w", err)
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create extracted file: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("write extracted file: %w", err)
			}
			if err := out.Close(); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("close extracted file: %w", err)
			}
		}
	}

	metaPath := filepath.Join(tmpDir, "backup_meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return extractedArchive{}, fmt.Errorf("read backup_meta.json: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		_ = os.RemoveAll(tmpDir)
		return extractedArchive{}, fmt.Errorf("decode backup_meta.json: %w", err)
	}

	dumpPath := filepath.Join(tmpDir, "postgres.dump")
	if _, err := os.Stat(dumpPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return extractedArchive{}, fmt.Errorf("backup missing postgres.dump: %w", err)
	}

	vaultPath := filepath.Join(tmpDir, "vault")
	if _, err := os.Stat(vaultPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(vaultPath, 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create empty extracted vault dir: %w", err)
			}
		} else {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("stat extracted vault path: %w", err)
		}
	}

	return extractedArchive{dir: tmpDir, meta: meta, dumpPath: dumpPath, vaultPath: vaultPath}, nil
}

func buildArchive(outputPath, sourceDir string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output parent directory: %w", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create backup archive: %w", err)
	}
	defer func() { _ = out.Close() }()

	gz := gzip.NewWriter(out)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	entries := []string{"backup_meta.json", "postgres.dump", "vault"}
	for _, name := range entries {
		full := filepath.Join(sourceDir, name)
		if _, err := os.Stat(full); err != nil {
			return fmt.Errorf("required backup item %q missing: %w", name, err)
		}
		if err := addPathToTar(tw, full, name); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close backup archive: %w", err)
	}

	return nil
}

func addPathToTar(tw *tar.Writer, fullPath, archivePath string) error {
	info, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("stat backup item %q: %w", fullPath, err)
	}

	if info.IsDir() {
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create tar header for directory %q: %w", fullPath, err)
		}
		hdr.Name = filepath.ToSlash(archivePath)
		if !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar directory header %q: %w", fullPath, err)
		}

		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return fmt.Errorf("read backup directory %q: %w", fullPath, err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			childFull := filepath.Join(fullPath, entry.Name())
			childArchive := filepath.Join(archivePath, entry.Name())
			if err := addPathToTar(tw, childFull, childArchive); err != nil {
				return err
			}
		}
		return nil
	}

	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header for file %q: %w", fullPath, err)
	}
	hdr.Name = filepath.ToSlash(archivePath)
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar file header %q: %w", fullPath, err)
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("open backup file %q: %w", fullPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("write backup file %q to archive: %w", fullPath, err)
	}

	return nil
}

func writeMetadata(path string, meta Metadata) error {
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup metadata: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write backup metadata file: %w", err)
	}
	return nil
}
