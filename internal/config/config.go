// Package config loads EconoMe's runtime configuration from environment
// variables (12-factor). Every operational knob is an ECONOME_* variable; no
// configuration is baked into the image. See technical/07-deployment.md §3.
//
// The session secret is intentionally NOT a config field: it is auto-generated
// on first run and persisted to the data volume (technical/05 §4, 07 §3). Only
// its override path (ECONOME_SECRET_FILE) is read here.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the resolved runtime configuration. It holds only transport- and
// operations-level settings; security parameter tuning (Argon2, lockout,
// session lifetime) is added in the auth increments and read from the same
// environment, with the documented defaults.
type Config struct {
	// DataDir is the mounted volume holding the SQLite file, the secret, and
	// backups. The app refuses to start if it is not writable (07 §4).
	DataDir string
	// Listen is the internal HTTP listen address; TLS is terminated upstream.
	Listen string
	// BehindTLS toggles Secure cookies + HSTS (true in prod behind a proxy,
	// false for local http://localhost dev).
	BehindTLS bool
	// TrustedProxy is the proxy IP/CIDR whose X-Forwarded-For is trusted by the
	// per-IP login throttle (05 §6).
	TrustedProxy string
	// DefaultLocale is the signed-out locale fallback ("fr" | "en").
	DefaultLocale string
	// LogLevel controls structured logging verbosity.
	LogLevel string
	// SecretFile optionally overrides the on-volume secret path; empty means
	// the default DataDir/secret.key is used.
	SecretFile string
}

// Default values mirror technical/07 §3. BehindTLS defaults to true (prod); the
// local start.bat sets ECONOME_BEHIND_TLS=0.
const (
	defaultDataDir       = "/data"
	defaultListen        = ":8765"
	defaultBehindTLS     = true
	defaultDefaultLocale = "fr"
	defaultLogLevel      = "info"
)

// Load resolves the configuration from the process environment, applying the
// documented defaults for any unset variable. It validates the locale enum and
// returns a typed error rather than panicking, so cmd/ can fail fast with a
// clear message.
func Load() (*Config, error) {
	c := &Config{
		DataDir:       envOr("ECONOME_DATA_DIR", defaultDataDir),
		Listen:        envOr("ECONOME_LISTEN", defaultListen),
		BehindTLS:     envBool("ECONOME_BEHIND_TLS", defaultBehindTLS),
		TrustedProxy:  os.Getenv("ECONOME_TRUSTED_PROXY"),
		DefaultLocale: strings.ToLower(envOr("ECONOME_DEFAULT_LOCALE", defaultDefaultLocale)),
		LogLevel:      strings.ToLower(envOr("ECONOME_LOG_LEVEL", defaultLogLevel)),
		SecretFile:    os.Getenv("ECONOME_SECRET_FILE"),
	}

	switch c.DefaultLocale {
	case "fr", "en":
	default:
		return nil, fmt.Errorf("config: ECONOME_DEFAULT_LOCALE must be \"fr\" or \"en\", got %q", c.DefaultLocale)
	}

	return c, nil
}

// envOr returns the environment value for key, or def when unset/empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool parses a boolean-ish env value ("1"/"0"/"true"/"false"); unset or
// unparseable falls back to def.
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
