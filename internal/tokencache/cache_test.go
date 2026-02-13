package tokencache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isolateCache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig := DirOverride
	DirOverride = dir
	t.Cleanup(func() { DirOverride = orig })
}

func TestCacheFilePath(t *testing.T) {
	t.Parallel()
	path := CacheFilePath()
	assert.Contains(t, filepath.Base(path), cacheFileName)
}

func TestWriteAndReadCache(t *testing.T) {
	isolateCache(t)

	entry := &CacheEntry{
		Token:          "ghs_test123",
		Expiry:         time.Now().Add(30 * time.Minute),
		InstallationID: 456,
	}

	require.NoError(t, WriteCache(entry))

	got := ReadCache(456)
	require.NotNil(t, got)
	assert.Equal(t, "ghs_test123", got.Token)
	assert.Equal(t, int64(456), got.InstallationID)
}

func TestReadCache_WrongInstallationID(t *testing.T) {
	isolateCache(t)

	entry := &CacheEntry{
		Token:          "ghs_test",
		Expiry:         time.Now().Add(30 * time.Minute),
		InstallationID: 456,
	}
	require.NoError(t, WriteCache(entry))

	got := ReadCache(789)
	assert.Nil(t, got)
}

func TestReadCache_Expired(t *testing.T) {
	isolateCache(t)

	entry := &CacheEntry{
		Token:          "ghs_old",
		Expiry:         time.Now().Add(2 * time.Minute), // < 5min threshold
		InstallationID: 456,
	}
	require.NoError(t, WriteCache(entry))

	got := ReadCache(456)
	assert.Nil(t, got)
}

func TestReadCache_Missing(t *testing.T) {
	isolateCache(t)
	got := ReadCache(456)
	assert.Nil(t, got)
}

func TestReadCache_Corrupt(t *testing.T) {
	isolateCache(t)

	path := CacheFilePath()
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	got := ReadCache(456)
	assert.Nil(t, got)
}

func TestRemoveCache(t *testing.T) {
	isolateCache(t)

	entry := &CacheEntry{
		Token:          "ghs_remove",
		Expiry:         time.Now().Add(30 * time.Minute),
		InstallationID: 456,
	}
	require.NoError(t, WriteCache(entry))

	RemoveCache()
	got := ReadCache(456)
	assert.Nil(t, got)
}

func TestWriteCache_AtomicOverwrite(t *testing.T) {
	isolateCache(t)

	require.NoError(t, WriteCache(&CacheEntry{
		Token:          "ghs_first",
		Expiry:         time.Now().Add(30 * time.Minute),
		InstallationID: 456,
	}))

	require.NoError(t, WriteCache(&CacheEntry{
		Token:          "ghs_second",
		Expiry:         time.Now().Add(30 * time.Minute),
		InstallationID: 456,
	}))

	got := ReadCache(456)
	require.NotNil(t, got)
	assert.Equal(t, "ghs_second", got.Token)
}

func TestCacheEntry_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	entry := CacheEntry{
		Token:          "ghs_json",
		Expiry:         time.Date(2026, 2, 19, 13, 45, 0, 0, time.UTC),
		InstallationID: 789012,
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded CacheEntry
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, entry.Token, decoded.Token)
	assert.Equal(t, entry.InstallationID, decoded.InstallationID)
	assert.True(t, entry.Expiry.Equal(decoded.Expiry))
}
