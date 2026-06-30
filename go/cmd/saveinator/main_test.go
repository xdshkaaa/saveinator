package main

import (
	"testing"

	"saveinator/internal/app"
	"saveinator/internal/config"
)

func TestAppBootstrap(t *testing.T) {
	t.Parallel()
	if app.New(&config.Settings{}) == nil {
		t.Fatal("expected app instance")
	}
}
