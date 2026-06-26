package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSecret_GeneratesAndReuses(t *testing.T) {
	dir := t.TempDir()
	c := &Config{DataDir: dir}

	s1, err := c.EnsureSecret()
	if err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	if len(s1) != secretLen {
		t.Fatalf("secret len = %d, want %d", len(s1), secretLen)
	}
	if _, err := os.Stat(c.SecretPath()); err != nil {
		t.Fatalf("secret file not written: %v", err)
	}

	s2, err := c.EnsureSecret()
	if err != nil {
		t.Fatalf("EnsureSecret reuse: %v", err)
	}
	if string(s1) != string(s2) {
		t.Error("EnsureSecret should reuse the persisted secret, not regenerate it")
	}
}

func TestEnsureDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	c := &Config{DataDir: dir}
	if err := c.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("data dir not created: %v", err)
	}
}
