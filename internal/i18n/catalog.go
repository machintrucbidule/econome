// Package i18n loads the embedded FR/EN message catalogs and provides
// locale-aware lookup and money formatting (technical/06). It and internal/view
// own all server-side formatting; the engine never formats.
package i18n

import (
	"embed"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"econome/internal/domain"
)

//go:embed locales/*.toml
var localesFS embed.FS

// Catalog holds the parsed message tables, keyed by language.
type Catalog struct {
	messages map[domain.Language]map[string]string
}

// Load parses the embedded TOML catalogs once at startup (technical/06 §3).
func Load() (*Catalog, error) {
	c := &Catalog{messages: map[domain.Language]map[string]string{}}
	for _, lang := range []domain.Language{domain.LangFR, domain.LangEN} {
		data, err := localesFS.ReadFile("locales/" + string(lang) + ".toml")
		if err != nil {
			return nil, fmt.Errorf("i18n: read %s catalog: %w", lang, err)
		}
		m := map[string]string{}
		if err := toml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("i18n: parse %s catalog: %w", lang, err)
		}
		c.messages[lang] = m
	}
	return c, nil
}

// T returns the localised message for key, substituting positional {0},{1},…
// arguments. A missing key falls back to the default locale (FR) and is logged
// once; if still absent the key itself is returned (so gaps surface, never crash).
func (c *Catalog) T(lang domain.Language, key string, args ...string) string {
	msg, ok := c.lookup(lang, key)
	if !ok {
		slog.Warn("i18n: missing message key", "key", key, "lang", string(lang))
		return key
	}
	for i, a := range args {
		msg = strings.ReplaceAll(msg, "{"+strconv.Itoa(i)+"}", a)
	}
	return msg
}

func (c *Catalog) lookup(lang domain.Language, key string) (string, bool) {
	if m, ok := c.messages[lang]; ok {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	if lang != domain.LangFR {
		if v, ok := c.messages[domain.LangFR][key]; ok {
			return v, true
		}
	}
	return "", false
}
