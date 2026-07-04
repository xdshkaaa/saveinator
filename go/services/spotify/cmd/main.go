package main

import (
	"saveinator/internal/botkit"
	spotifyplat "saveinator/internal/botkit/platforms/spotify"
)

func main() {
	botkit.Main(botkit.BotConfig{
		Slug:            "spotify",
		Languages:       []string{"en", "ru", "kk"},
		DefaultLang:     "en",
		WelcomeKey:      "onboarding.welcome_spotify",
		NotSupportedKey: "spotify.not_spotify_link",
		Queue:           "spotify",
		Platforms:       []botkit.Platform{spotifyplat.New()},
	})
}
