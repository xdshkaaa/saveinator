package main

import (
	"saveinator/internal/botkit"
	soundcloudplat "saveinator/internal/botkit/platforms/soundcloud"
)

func main() {
	botkit.Main(botkit.BotConfig{
		Slug:            "soundcloud",
		Languages:       []string{"en", "ru", "kk"},
		DefaultLang:     "en",
		WelcomeKey:      "onboarding.welcome_soundcloud",
		NotSupportedKey: "soundcloud.not_soundcloud_link",
		Queue:           "soundcloud",
		Platforms:       []botkit.Platform{soundcloudplat.New()},
	})
}
