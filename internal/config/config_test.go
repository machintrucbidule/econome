package config

import "testing"

// TestLoadDefaults proves the test harness + -race flag work (the increment-0
// smoke test) and pins the documented defaults of technical/07 §3.
func TestLoadDefaults(t *testing.T) {
	t.Setenv("ECONOME_DATA_DIR", "")
	t.Setenv("ECONOME_LISTEN", "")
	t.Setenv("ECONOME_BEHIND_TLS", "")
	t.Setenv("ECONOME_DEFAULT_LOCALE", "")
	t.Setenv("ECONOME_LOG_LEVEL", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if c.DataDir != defaultDataDir {
		t.Errorf("DataDir = %q, want %q", c.DataDir, defaultDataDir)
	}
	if c.Listen != defaultListen {
		t.Errorf("Listen = %q, want %q", c.Listen, defaultListen)
	}
	if !c.BehindTLS {
		t.Errorf("BehindTLS = false, want true (prod default)")
	}
	if c.DefaultLocale != "fr" {
		t.Errorf("DefaultLocale = %q, want \"fr\"", c.DefaultLocale)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want \"info\"", c.LogLevel)
	}
}

func TestLoadRejectsUnknownLocale(t *testing.T) {
	t.Setenv("ECONOME_DEFAULT_LOCALE", "de")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted unknown locale, want error")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("ECONOME_DATA_DIR", "/srv/econome")
	t.Setenv("ECONOME_LISTEN", "127.0.0.1:9000")
	t.Setenv("ECONOME_BEHIND_TLS", "0")
	t.Setenv("ECONOME_DEFAULT_LOCALE", "EN")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if c.DataDir != "/srv/econome" {
		t.Errorf("DataDir = %q, want /srv/econome", c.DataDir)
	}
	if c.Listen != "127.0.0.1:9000" {
		t.Errorf("Listen = %q, want 127.0.0.1:9000", c.Listen)
	}
	if c.BehindTLS {
		t.Errorf("BehindTLS = true, want false (ECONOME_BEHIND_TLS=0)")
	}
	if c.DefaultLocale != "en" {
		t.Errorf("DefaultLocale = %q, want \"en\" (lowercased)", c.DefaultLocale)
	}
}
