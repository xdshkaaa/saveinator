package locale

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	cacheMu sync.RWMutex
	cache   = map[string]map[string]any{}
)

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
