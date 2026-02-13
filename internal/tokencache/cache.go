package tokencache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const cacheFileName = "ghapp-token-cache"

// DirOverride allows tests to redirect the cache to a temp directory.
var DirOverride string

// CacheEntry represents a cached token on disk.
type CacheEntry struct {
	Token          string    `json:"token"`
	Expiry         time.Time `json:"expiry"`
	InstallationID int64     `json:"installation_id"`
}

// CacheFilePath returns the platform-specific cache file path.
func CacheFilePath() string {
	if DirOverride != "" {
		return filepath.Join(DirOverride, cacheFileName)
	}
	var dir string
	if runtime.GOOS == "windows" {
		dir = os.Getenv("TEMP")
		if dir == "" {
			dir = os.TempDir()
		}
	} else {
		dir = os.Getenv("TMPDIR")
		if dir == "" {
			dir = "/tmp"
		}
	}
	return filepath.Join(dir, cacheFileName)
}

// ReadCache reads and validates the cached token. Returns nil if cache is
// missing, corrupt, wrong installation ID, or expiring within 5 minutes.
func ReadCache(installationID int64) *CacheEntry {
	data, err := os.ReadFile(CacheFilePath())
	if err != nil {
		return nil
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}

	if entry.InstallationID != installationID {
		return nil
	}

	if time.Until(entry.Expiry) < 5*time.Minute {
		return nil
	}

	return &entry
}

// WriteCache atomically writes a cache entry to disk.
func WriteCache(entry *CacheEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	path := CacheFilePath()
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp cache: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}

// RemoveCache deletes the cache file if it exists.
func RemoveCache() {
	_ = os.Remove(CacheFilePath())
}
