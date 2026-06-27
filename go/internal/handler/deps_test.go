package handler

import (
	"testing"

	"saveinator/internal/linkparser"
)

func TestShouldAcquireUserLock(t *testing.T) {
	t.Parallel()
	const adminID int64 = 100

	tests := []struct {
		name   string
		userID int64
		batch  bool
		want   bool
	}{
		{name: "normal user", userID: 1, batch: false, want: true},
		{name: "batch mode", userID: 1, batch: true, want: false},
		{name: "admin", userID: adminID, batch: false, want: false},
		{name: "admin batch", userID: adminID, batch: true, want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldAcquireUserLock(tt.userID, tt.batch, adminID); got != tt.want {
				t.Fatalf("shouldAcquireUserLock() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSceneForPlatform(t *testing.T) {
	t.Parallel()
	tests := []struct {
		platform string
		want     string
	}{
		{platform: "tiktok", want: "tiktok"},
		{platform: "pinterest", want: "pinterest"},
		{platform: "instagram", want: "instagram"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.platform, func(t *testing.T) {
			t.Parallel()
			if got := sceneForPlatform(tt.platform); got != tt.want {
				t.Fatalf("sceneForPlatform(%q) = %q, want %q", tt.platform, got, tt.want)
			}
		})
	}
}

func TestDispatchLink_platformRouting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		platform linkparser.Platform
		taskType string
		scene    string
	}{
		{platform: linkparser.PlatformTikTok, taskType: "download:tiktok", scene: "tiktok"},
		{platform: linkparser.PlatformPinterest, taskType: "download:pinterest", scene: "pinterest"},
		{platform: linkparser.PlatformInstagram, taskType: "download:send", scene: "instagram"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.platform), func(t *testing.T) {
			t.Parallel()
			scene := sceneForPlatform(string(tt.platform))
			if scene != tt.scene {
				t.Fatalf("scene = %q, want %q", scene, tt.scene)
			}
		})
	}
}
