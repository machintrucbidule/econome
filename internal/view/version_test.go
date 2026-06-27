package view

import (
	"testing"

	"econome/internal/domain"
	"econome/internal/i18n"
)

// The version shown in the UI is always "v<semver>": the default var is "0.0.1"
// (no leading v) but a release injects the git tag "vX.Y.Z" via -ldflags. New
// strips a leading "v" so the template's "v" prefix never doubles it.
func TestRendererNormalisesVersion(t *testing.T) {
	cat, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ in, want string }{
		{"0.0.1", "0.0.1"},
		{"v0.0.1", "0.0.1"},
		{"v1.2.3", "1.2.3"},
		{"dev", "dev"},
	} {
		r, err := New(cat, tc.in)
		if err != nil {
			t.Fatalf("New(%q): %v", tc.in, err)
		}
		b := r.NewBase(domain.LangFR, "EUR", domain.ThemeLight, "")
		if b.Version != tc.want {
			t.Errorf("version %q → Base.Version %q, want %q (template renders v%s)", tc.in, b.Version, tc.want, tc.want)
		}
	}
}
