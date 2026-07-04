package botkit

import (
	"fmt"
	"slices"

	"saveinator/internal/locale"
)

// BotConfig describes one bot instance: which languages it offers, which
// platforms it serves and which asynq queue its tasks go to. Everything else
// (tokens, DB, Redis, webhook) comes from config.Settings.
type BotConfig struct {
	// Slug identifies the bot in logs and queue names, e.g. "pinterest_kz".
	Slug string
	// Languages offered in the language keyboards, in button order.
	Languages []string
	// DefaultLang is used when a user has no stored language.
	DefaultLang string
	// WelcomeKey is the locale key of the welcome message.
	WelcomeKey string
	// NotSupportedKey is the locale key sent for links of foreign platforms.
	NotSupportedKey string
	// Queue is the asynq queue name; defaults to Slug.
	Queue string
	// RegisterAPI enables the HTTP download API routes on the webhook mux.
	RegisterAPI bool
	// Platforms this bot serves; dispatch order matters.
	Platforms []Platform
}

func (c *BotConfig) normalize() error {
	if c.Slug == "" {
		return fmt.Errorf("botkit: Slug is required")
	}
	if len(c.Languages) == 0 {
		c.Languages = []string{"en", "ru"}
	}
	if c.DefaultLang == "" {
		c.DefaultLang = c.Languages[0]
	}
	if !slices.Contains(c.Languages, c.DefaultLang) {
		return fmt.Errorf("botkit: DefaultLang %q not in Languages", c.DefaultLang)
	}
	for _, lang := range c.Languages {
		if !locale.Supported(lang) {
			return fmt.Errorf("botkit: no locale file for language %q", lang)
		}
	}
	if c.Queue == "" {
		c.Queue = c.Slug
	}
	if c.WelcomeKey == "" {
		return fmt.Errorf("botkit: WelcomeKey is required")
	}
	if c.NotSupportedKey == "" {
		c.NotSupportedKey = "errors.unsupported"
	}
	if len(c.Platforms) == 0 {
		return fmt.Errorf("botkit: at least one Platform is required")
	}
	return nil
}

func (c *BotConfig) langAllowed(lang string) bool {
	return slices.Contains(c.Languages, lang)
}
