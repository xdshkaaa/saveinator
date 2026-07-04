package main

import (
	"saveinator/internal/botkit"
	pinterestplat "saveinator/internal/botkit/platforms/pinterest"
)

func main() {
	botkit.Main(botkit.BotConfig{
		Slug:            "pinterest_kz",
		Languages:       []string{"kk", "ru", "en"},
		DefaultLang:     "kk",
		WelcomeKey:      "onboarding.welcome_pinterest",
		NotSupportedKey: "pinterest.not_pinterest_link",
		Queue:           "pinterest_kz",
		RegisterAPI:     true,
		Platforms:       []botkit.Platform{pinterestplat.New()},
	})
}
