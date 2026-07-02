package handler

import (
	"strings"

	"github.com/mymmrac/telego"

	"saveinator/internal/linkparser"
)

func messageBody(msg telego.Message) string {
	if msg.Text != "" {
		return msg.Text
	}
	return msg.Caption
}

func shouldSkipIncomingMessage(msg telego.Message) bool {
	body := messageBody(msg)
	if body == "" || msg.From == nil {
		return true
	}
	return strings.HasPrefix(body, "/")
}

func extractMessageLinks(msg telego.Message) []linkparser.ParsedLink {
	return linkparser.ExtractURLs(messageBody(msg))
}
