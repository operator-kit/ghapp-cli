package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/jferrl/go-githubauth"
	"github.com/operator-kit/ghapp-cli/internal/config"
)

type TokenResult struct {
	Token     string
	ExpiresAt time.Time
}

// TokenGenerator generates installation tokens.
type TokenGenerator interface {
	Generate(appID, installationID int64, privateKey []byte) (*TokenResult, error)
}

// GitHubTokenGenerator is the real implementation using go-githubauth.
type GitHubTokenGenerator struct{}

func (g *GitHubTokenGenerator) Generate(appID, installationID int64, privateKey []byte) (*TokenResult, error) {
	return GenerateInstallationToken(appID, installationID, privateKey)
}

// LoadPrivateKeyFromConfig reads the PEM key from file or keyring.
func LoadPrivateKeyFromConfig(cfg *config.Config) ([]byte, error) {
	if cfg.KeyInKeyring {
		key, err := LoadPrivateKey()
		if err != nil {
			return nil, fmt.Errorf("load key from keyring: %w", err)
		}
		return key, nil
	}

	if cfg.PrivateKeyPath == "" {
		return nil, fmt.Errorf("no private key configured (set private_key_path or use --import-key)")
	}

	key, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key file: %w", err)
	}
	return key, nil
}

// GenerateInstallationToken creates a fresh GitHub App installation token.
func GenerateInstallationToken(appID, installationID int64, privateKey []byte) (*TokenResult, error) {
	appTokenSource, err := githubauth.NewApplicationTokenSource(appID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create app token source: %w", err)
	}

	installationTokenSource := githubauth.NewInstallationTokenSource(installationID, appTokenSource)

	token, err := installationTokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("generate installation token: %w", err)
	}

	return &TokenResult{
		Token:     token.AccessToken,
		ExpiresAt: token.Expiry,
	}, nil
}
