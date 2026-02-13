package auth

import "github.com/zalando/go-keyring"

const keyringService = "ghapp-cli"
const keyringUser = "private-key"

// KeyringProvider abstracts OS keyring access for testing.
type KeyringProvider interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

// osKeyring is the real implementation using go-keyring.
type osKeyring struct{}

func (k osKeyring) Set(s, u, p string) error        { return keyring.Set(s, u, p) }
func (k osKeyring) Get(s, u string) (string, error)  { return keyring.Get(s, u) }
func (k osKeyring) Delete(s, u string) error          { return keyring.Delete(s, u) }

// ring is the package-level keyring provider, replaceable in tests.
var ring KeyringProvider = osKeyring{}

func StorePrivateKey(pem []byte) error {
	return ring.Set(keyringService, keyringUser, string(pem))
}

func LoadPrivateKey() ([]byte, error) {
	s, err := ring.Get(keyringService, keyringUser)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

func DeletePrivateKey() error {
	return ring.Delete(keyringService, keyringUser)
}
