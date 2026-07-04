package main

import (
	"saveinator/internal/botkit"
	pinterestplat "saveinator/internal/botkit/platforms/pinterest"
)

func main() {
	botkit.Main(botkit.BotConfig{
		Slug:            "pinterest",
		Languages:       []string{"en", "ru"},
		DefaultLang:     "en",
		WelcomeKey:      "onboarding.welcome_pinterest",
		NotSupportedKey: "pinterest.not_pinterest_link",
		Queue:           "pinterest",
		RegisterAPI:     true,
		Platforms:       []botkit.Platform{pinterestplat.New()},
	})
}
