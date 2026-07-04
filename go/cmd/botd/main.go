package main

import (
	"os"

	"saveinator/internal/botkit"

	// Platforms register themselves with botkit by name.
	_ "saveinator/internal/botkit/platforms/pinterest"
	_ "saveinator/internal/botkit/platforms/soundcloud"
	_ "saveinator/internal/botkit/platforms/spotify"
)

func main() {
	path := os.Getenv("BOTS_CONFIG")
	if path == "" {
		path = "bots.yaml"
	}
	botkit.FleetMain(path)
}
