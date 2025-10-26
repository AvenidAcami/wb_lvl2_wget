package storage

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

func EnsureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0o755)
}

func SaveFile(baseDir, relPath string, r io.Reader) (string, error) {
	relPath = strings.TrimPrefix(relPath, "/")
	relPath = strings.TrimPrefix(relPath, "\\")
	relPath = filepath.FromSlash(relPath)

	fullPath := filepath.Join(baseDir, relPath)
	if err := EnsureDir(fullPath); err != nil {
		return "", err
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return fullPath, nil
}
