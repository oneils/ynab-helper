package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	tempUploadDir = "/tmp/ynab-helper-uploads"
	fileMaxAge    = 1 * time.Hour
)

// TempFileStore handles temporary file storage for CSV uploads during preview.
type TempFileStore struct {
	baseDir string
}

// NewTempFileStore creates a new temporary file store.
func NewTempFileStore() (*TempFileStore, error) {
	if err := os.MkdirAll(tempUploadDir, 0755); err != nil {
		return nil, fmt.Errorf("creating temp upload directory: %w", err)
	}
	return &TempFileStore{baseDir: tempUploadDir}, nil
}

// SaveUpload saves uploaded file data and returns a UUID for later retrieval.
func (fs *TempFileStore) SaveUpload(data []byte, filename string) (string, error) {
	// Generate UUID
	uuid, err := generateUUID()
	if err != nil {
		return "", fmt.Errorf("generating UUID: %w", err)
	}

	// Save file with UUID as filename
	filePath := filepath.Join(fs.baseDir, uuid)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("writing temp file: %w", err)
	}

	return uuid, nil
}

// GetUpload retrieves file data by UUID.
func (fs *TempFileStore) GetUpload(uuid string) ([]byte, error) {
	filePath := filepath.Join(fs.baseDir, uuid)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found or expired")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading temp file: %w", err)
	}

	return data, nil
}

// DeleteUpload removes a temporary file by UUID.
func (fs *TempFileStore) DeleteUpload(uuid string) error {
	filePath := filepath.Join(fs.baseDir, uuid)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting temp file: %w", err)
	}

	return nil
}

// CleanupOldFiles removes files older than fileMaxAge.
func (fs *TempFileStore) CleanupOldFiles() error {
	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return fmt.Errorf("reading temp directory: %w", err)
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Delete files older than maxAge
		if now.Sub(info.ModTime()) > fileMaxAge {
			filePath := filepath.Join(fs.baseDir, entry.Name())
			_ = os.Remove(filePath) // Ignore errors on cleanup
		}
	}

	return nil
}

// generateUUID generates a random UUID-like string.
func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
