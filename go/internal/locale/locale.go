package locale

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	cacheMu sync.RWMutex
	cache   = map[string]map[string]any{}
)

// Lang describes one available locale file.
type Lang struct {
	Code string // "en"
	Name string // self name from lang.self_name, e.g. "Қазақша"
}

var (
	langsOnce sync.Once
	langs     []Lang
)

// Languages lists locales discovered in the embedded files, sorted by code.
// Adding a language is adding one JSON file — no code changes.
func Languages() []Lang {
	langsOnce.Do(func() {
		entries, err := localeFS.ReadDir("locales")
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			code := strings.TrimSuffix(name, ".json")
			langs = append(langs, Lang{Code: code, Name: SelfName(code)})
		}
		sort.Slice(langs, func(i, j int) bool { return langs[i].Code < langs[j].Code })
	})
	return langs
}

// Supported reports whether a locale file exists for the code.
func Supported(code string) bool {
	for _, l := range Languages() {
		if l.Code == code {
			return true
		}
	}
	return false
}

// SelfName returns the language's own name (lang.self_name), falling back to
// the code itself.
func SelfName(code string) string {
	value, err := lookup("lang.self_name", code)
	if err != nil {
		return code
	}
	return fmt.Sprint(value)
}

func Get(key, lang string, vars map[string]string) string {
	value, err := lookup(key, lang)
	if err != nil {
		value, err = lookup(key, "en")
		if err != nil {
			return key
		}
	}
	text := fmt.Sprint(value)
	for k, v := range vars {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	return text
}

func lookup(key, lang string) (any, error) {
	data, err := load(lang)
	if err != nil {
		return nil, err
	}
	var current any = data
	for _, part := range strings.Split(key, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("missing locale key: %s", key)
		}
		next, ok := m[part]
		if !ok {
			return nil, fmt.Errorf("missing locale key: %s", key)
		}
		current = next
	}
	return current, nil
}

func load(lang string) (map[string]any, error) {
	cacheMu.RLock()
	if data, ok := cache[lang]; ok {
		cacheMu.RUnlock()
		return data, nil
	}
	cacheMu.RUnlock()

	raw, err := localeFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()
	if data, ok := cache[lang]; ok {
		return data, nil
	}
	cache[lang] = parsed
	return parsed, nil
}
