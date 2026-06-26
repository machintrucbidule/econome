package config

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

// secretLen is the length of the session/CSRF secret (technical/05 §4).
const secretLen = 32

// SecretPath returns the on-volume secret file path: the explicit override
// (ECONOME_SECRET_FILE) or DataDir/secret.key (technical/07 §3).
func (c *Config) SecretPath() string {
	if c.SecretFile != "" {
		return c.SecretFile
	}
	return filepath.Join(c.DataDir, "secret.key")
}

// EnsureDataDir creates the data directory if missing and verifies it is
// writable, refusing startup otherwise (technical/07 §4 — an unmounted/unwritable
// volume must not start with an empty database).
func (c *Config) EnsureDataDir() error {
	if err := os.MkdirAll(c.DataDir, 0o750); err != nil {
		return fmt.Errorf("config: create data dir %q: %w", c.DataDir, err)
	}
	probe := filepath.Join(c.DataDir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("config: data dir %q is not writable: %w", c.DataDir, err)
	}
	_ = os.Remove(probe)
	return nil
}

// EnsureSecret loads the session secret from the data volume, generating and
// persisting a fresh 32-byte secret (mode 0600) on first run. Living on the
// volume, it survives container recreation so sessions/CSRF stay valid across
// updates (technical/05 §4, 07 §3).
func (c *Config) EnsureSecret() ([]byte, error) {
	path := c.SecretPath()
	//nolint:gosec // path is the operator-configured secret file (data volume), never user input
	if b, err := os.ReadFile(path); err == nil {
		if len(b) < secretLen {
			return nil, fmt.Errorf("config: secret file %q is too short (%d bytes)", path, len(b))
		}
		return b, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("config: read secret %q: %w", path, err)
	}

	secret := make([]byte, secretLen)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("config: generate secret: %w", err)
	}
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, fmt.Errorf("config: write secret %q: %w", path, err)
	}
	return secret, nil
}
