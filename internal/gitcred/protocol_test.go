package gitcred

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		protocol string
		host     string
		path     string
	}{
		{
			name:     "standard",
			input:    "protocol=https\nhost=github.com\n\n",
			protocol: "https",
			host:     "github.com",
		},
		{
			name:     "with_path",
			input:    "protocol=https\nhost=github.com\npath=owner/repo.git\n\n",
			protocol: "https",
			host:     "github.com",
			path:     "owner/repo.git",
		},
		{
			name:     "eof_terminated",
			input:    "protocol=https\nhost=github.com",
			protocol: "https",
			host:     "github.com",
		},
		{
			name: "empty",
		},
		{
			name:     "unknown_fields_ignored",
			input:    "protocol=https\nhost=github.com\nusername=foo\npassword=bar\n\n",
			protocol: "https",
			host:     "github.com",
		},
		{
			name:     "malformed_lines_ignored",
			input:    "protocol=https\nno-equals-sign\nhost=github.com\n\n",
			protocol: "https",
			host:     "github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req, err := Parse(strings.NewReader(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.protocol, req.Protocol)
			assert.Equal(t, tt.host, req.Host)
			assert.Equal(t, tt.path, req.Path)
		})
	}
}

func TestWriteResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		password string
		expiry   int64
		expected string
	}{
		{
			name:     "standard",
			username: "x-access-token",
			password: "ghs_abc123",
			expiry:   1700000000,
			expected: "username=x-access-token\npassword=ghs_abc123\npassword_expiry_utc=1700000000\n",
		},
		{
			name:     "zero_expiry",
			username: "user",
			password: "pass",
			expiry:   0,
			expected: "username=user\npassword=pass\npassword_expiry_utc=0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := WriteResponse(&buf, tt.username, tt.password, tt.expiry)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}
