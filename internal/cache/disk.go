package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	dirPerm  iofs.FileMode = 0o700
	filePerm iofs.FileMode = 0o600
)

// safeKeyChars restricts cache filenames to an alphanumeric whitelist + `_` / `.` / `-`.
// Any other byte — including `\`, `/`, `:`, control characters, path traversal segments
// — is collapsed into `_`. Prevents attacker-influenced namespace/arg values from
// escaping the cache directory (e.g. a GitLab project named `..\..\..\..\passwd` on
// Windows, or branch names containing null bytes).
var safeKeyChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

type diskPayload[T any] struct {
	CreatedAt float64 `json:"created_at"`
	Data      T       `json:"data"`
}

func sanitizeKey(key string) string {
	safe := safeKeyChars.ReplaceAllString(key, "_")
	safe = strings.TrimLeft(safe, ".")
	if safe == "" {
		return "_"
	}

	return safe
}

func diskPath(dir, key string) string {
	return filepath.Join(dir, "api_"+sanitizeKey(key)+".json")
}

func toUnixFloat(t time.Time) float64 {
	return float64(t.UnixNano()) / 1e9
}

func fromUnixFloat(f float64) time.Time {
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)

	return time.Unix(sec, nsec)
}

// ensureCacheDir creates dir with 0700 permissions if missing. Called once per
// Cache lifetime via sync.Once; avoids a redundant stat syscall on every write.
func ensureCacheDir(fsys afero.Fs, dir string) error {
	if err := fsys.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("cache: mkdir %q: %w", dir, err)
	}

	return nil
}

func loadDisk[T any](fsys afero.Fs, dir, key string) (data T, createdAt time.Time, ok bool) {
	var zero T
	path := diskPath(dir, key)

	raw, err := afero.ReadFile(fsys, path)
	if err != nil {
		return zero, time.Time{}, false
	}

	var payload diskPayload[T]
	if err := json.Unmarshal(raw, &payload); err != nil {
		return zero, time.Time{}, false
	}

	return payload.Data, fromUnixFloat(payload.CreatedAt), true
}

// saveDisk writes the key as JSON. The cache directory must already exist —
// callers in production go through Cache.put which creates it once via sync.Once.
func saveDisk(fsys afero.Fs, dir, key string, createdAt time.Time, data any) error {
	payload := diskPayload[any]{
		CreatedAt: toUnixFloat(createdAt),
		Data:      data,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cache: marshal: %w", err)
	}
	path := diskPath(dir, key)
	if err := afero.WriteFile(fsys, path, buf, filePerm); err != nil {
		return fmt.Errorf("cache: write %q: %w", path, err)
	}

	return nil
}

func removeDiskFile(fsys afero.Fs, dir, key string) error {
	path := diskPath(dir, key)
	err := fsys.Remove(path)
	if err == nil || errors.Is(err, iofs.ErrNotExist) {
		return nil
	}

	return fmt.Errorf("cache: remove %q: %w", path, err)
}
