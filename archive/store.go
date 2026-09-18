// Package archive stores immutable, content-addressed transcript snapshots.
package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/muratmirgun/compact-engine/compact"
)

// ErrNotFound indicates a snapshot or message that does not exist.
var ErrNotFound = errors.New("archive: not found")

// Store writes private JSON files. One directory belongs to one trust boundary.
// Concurrent writes of the same snapshot are safe; readers see complete files.
type Store struct{ dir string }

// New creates a private directory. It does not delete existing snapshots.
func New(dir string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve archive: %w", err)
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return nil, fmt.Errorf("create archive: %w", err)
	}
	return &Store{dir: abs}, nil
}

// Save writes a canonical JSON snapshot before returning its SHA-256 identifier.
func (s *Store) Save(ctx context.Context, req compact.Request) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("encode snapshot: %w", err)
	}
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:])
	f, err := os.CreateTemp(s.dir, ".snapshot-*")
	if err != nil {
		return "", fmt.Errorf("create snapshot: %w", err)
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }() // Cleanup is best effort after commit or a returned write error.
	if _, err := f.Write(data); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("sync snapshot: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close snapshot: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), filepath.Join(s.dir, id+".json")); err != nil {
		return "", fmt.Errorf("commit snapshot: %w", err)
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return "", fmt.Errorf("open archive directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return "", fmt.Errorf("sync archive directory: %w", err)
	}
	return id, nil
}

// Load verifies snapshot integrity before returning a fresh request value.
func (s *Store) Load(ctx context.Context, id string) (compact.Request, error) {
	if err := ctx.Err(); err != nil {
		return compact.Request{}, err
	}
	if !validID(id) {
		return compact.Request{}, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return compact.Request{}, ErrNotFound
	}
	if err != nil {
		return compact.Request{}, fmt.Errorf("read snapshot: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != id {
		return compact.Request{}, errors.New("archive: integrity check failed")
	}
	var req compact.Request
	if err := json.Unmarshal(data, &req); err != nil {
		return compact.Request{}, fmt.Errorf("decode snapshot: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return compact.Request{}, err
	}
	return req, nil
}

// Recall returns an original message without re-running its tool.
func (s *Store) Recall(ctx context.Context, snapshotID, messageID string) (compact.Message, error) {
	req, err := s.Load(ctx, snapshotID)
	if err != nil {
		return compact.Message{}, err
	}
	for _, m := range req.Messages {
		if m.ID == messageID {
			return m, nil
		}
	}
	return compact.Message{}, ErrNotFound
}

func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, r := range id {
		isDigit := r >= '0' && r <= '9'
		isHexLetter := r >= 'a' && r <= 'f'
		if !isDigit && !isHexLetter {
			return false
		}
	}
	return true
}
