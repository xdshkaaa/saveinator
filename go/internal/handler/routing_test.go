package handler

import (
	"testing"

	"github.com/mymmrac/telego"

	"saveinator/internal/linkparser"
)

func TestMessageBody_prefersText(t *testing.T) {
	t.Parallel()
	msg := telego.Message{Text: "hello", Caption: "world"}
	if got := messageBody(msg); got != "hello" {
		t.Fatalf("messageBody() = %q, want hello", got)
	}
}

func TestMessageBody_fallsBackToCaption(t *testing.T) {
	t.Parallel()
	msg := telego.Message{Caption: "https://tiktok.com/@u/video/1"}
	if got := messageBody(msg); got != "https://tiktok.com/@u/video/1" {
		t.Fatalf("messageBody() = %q, want caption URL", got)
	}
}

func TestShouldSkipIncomingMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  telego.Message
		want bool
	}{
		{
			name: "empty",
			msg:  telego.Message{From: &telego.User{ID: 1}},
			want: true,
		},
		{
			name: "command",
			msg:  telego.Message{Text: "/start", From: &telego.User{ID: 1}},
			want: true,
		},
		{
			name: "caption command",
			msg:  telego.Message{Caption: "/settings", From: &telego.User{ID: 1}},
			want: true,
		},
		{
			name: "link text",
			msg:  telego.Message{Text: "https://youtu.be/abc", From: &telego.User{ID: 1}},
			want: false,
		},
		{
			name: "link caption",
			msg:  telego.Message{Caption: "https://tiktok.com/@u/video/1", From: &telego.User{ID: 1}},
			want: false,
		},
		{
			name: "no from",
			msg:  telego.Message{Text: "hello"},
			want: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldSkipIncomingMessage(tt.msg); got != tt.want {
				t.Fatalf("shouldSkipIncomingMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractMessageLinks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		msg      telego.Message
		platform linkparser.Platform
	}{
		{
			name:     "text link",
			msg:      telego.Message{Text: "check https://www.tiktok.com/@u/video/123"},
			platform: linkparser.PlatformTikTok,
		},
		{
			name:     "caption link",
			msg:      telego.Message{Caption: "https://www.tiktok.com/@u/video/456"},
			platform: linkparser.PlatformTikTok,
		},
		{
			name: "no links",
			msg:  telego.Message{Text: "hello"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			links := extractMessageLinks(tt.msg)
			if tt.platform == "" {
				if len(links) != 0 {
					t.Fatalf("expected no links, got %d", len(links))
				}
				return
			}
			if len(links) != 1 {
				t.Fatalf("expected 1 link, got %d", len(links))
			}
			if links[0].Platform != tt.platform {
				t.Fatalf("platform = %v, want %v", links[0].Platform, tt.platform)
			}
		})
	}
}
