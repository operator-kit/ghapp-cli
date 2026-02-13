package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/operator-kit/ghapp-cli/internal/config"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func readTestPEM(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(testdataPath("test.pem"))
	require.NoError(t, err)
	return data
}

// --- fakeKeyring for testing ---

type fakeKeyring struct {
	store  map[string]string
	setErr error
	getErr error
	delErr error
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{store: make(map[string]string)}
}

func (f *fakeKeyring) Set(service, user, password string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.store[service+"/"+user] = password
	return nil
}

func (f *fakeKeyring) Get(service, user string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.store[service+"/"+user]
	if !ok {
		return "", fmt.Errorf("secret not found")
	}
	return v, nil
}

func (f *fakeKeyring) Delete(service, user string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.store, service+"/"+user)
	return nil
}

func withFakeKeyring(t *testing.T) *fakeKeyring {
	t.Helper()
	orig := ring
	fk := newFakeKeyring()
	ring = fk
	t.Cleanup(func() { ring = orig })
	return fk
}

// --- LoadPrivateKeyFromConfig tests (file-based, safe for parallel) ---

func TestLoadPrivateKeyFromConfig(t *testing.T) {
	t.Parallel()

	testPEM := readTestPEM(t)

	tests := []struct {
		name    string
		setup   func(t *testing.T) *config.Config
		want    []byte
		wantErr string
	}{
		{
			name: "from_file",
			setup: func(t *testing.T) *config.Config {
				dir := t.TempDir()
				keyPath := filepath.Join(dir, "test.pem")
				require.NoError(t, os.WriteFile(keyPath, testPEM, 0o600))
				return &config.Config{PrivateKeyPath: keyPath}
			},
			want: testPEM,
		},
		{
			name: "missing_file",
			setup: func(t *testing.T) *config.Config {
				return &config.Config{PrivateKeyPath: "/nonexistent/key.pem"}
			},
			wantErr: "read private key file",
		},
		{
			name: "no_key_configured",
			setup: func(t *testing.T) *config.Config {
				return &config.Config{}
			},
			wantErr: "no private key configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := tt.setup(t)
			got, err := LoadPrivateKeyFromConfig(cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// NO t.Parallel() — mutates package-level `ring`
func TestLoadPrivateKeyFromConfig_Keyring(t *testing.T) {
	tests := []struct {
		name    string
		keyring func() *fakeKeyring
		wantErr string
	}{
		{
			name: "success",
			keyring: func() *fakeKeyring {
				fk := newFakeKeyring()
				fk.store[keyringService+"/"+keyringUser] = "pem-data"
				return fk
			},
		},
		{
			name: "keyring_error",
			keyring: func() *fakeKeyring {
				fk := newFakeKeyring()
				fk.getErr = fmt.Errorf("keyring locked")
				return fk
			},
			wantErr: "load key from keyring",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := ring
			ring = tt.keyring()
			defer func() { ring = orig }()

			cfg := &config.Config{KeyInKeyring: true}
			got, err := LoadPrivateKeyFromConfig(cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []byte("pem-data"), got)
		})
	}
}

// --- Keyring function tests (NO parallel — mutates package-level `ring`) ---

func TestStorePrivateKey(t *testing.T) {
	fk := withFakeKeyring(t)

	require.NoError(t, StorePrivateKey([]byte("my-pem")))
	assert.Equal(t, "my-pem", fk.store[keyringService+"/"+keyringUser])
}

func TestLoadPrivateKey(t *testing.T) {
	fk := withFakeKeyring(t)
	fk.store[keyringService+"/"+keyringUser] = "stored-pem"

	got, err := LoadPrivateKey()
	require.NoError(t, err)
	assert.Equal(t, []byte("stored-pem"), got)
}

func TestDeletePrivateKey(t *testing.T) {
	fk := withFakeKeyring(t)
	fk.store[keyringService+"/"+keyringUser] = "to-delete"

	require.NoError(t, DeletePrivateKey())
	_, ok := fk.store[keyringService+"/"+keyringUser]
	assert.False(t, ok)
}
