package reddit

import (
	"regexp"
	"strings"
)

var (
	markdownLinkRe    = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)[^)]*\)`)
	markdownHeaderRe  = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	markdownQuoteRe   = regexp.MustCompile(`(?m)^>\s?`)
	// Note: no _..._ rule — paired underscores are too common inside
	// usernames and code to be safe to strip.
	markdownEmphasis  = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__|~~([^~]+)~~|\*([^*\n]+)\*|` + "`([^`\\n]+)`")
	paragraphSplitRe  = regexp.MustCompile(`\n{2,}`)
)

// normalizeSelftext trims a reddit text payload and treats removed content
// as empty.
func normalizeSelftext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[deleted]" || s == "[removed]" {
		return ""
	}
	return s
}

// ToPlainText flattens reddit markdown into plain text suitable for Telegraph
// nodes: links become "text (url)", headers and quotes lose their markers,
// paired emphasis markers are stripped.
func ToPlainText(md string) string {
	s := markdownLinkRe.ReplaceAllString(md, "$1 ($2)")
	s = markdownHeaderRe.ReplaceAllString(s, "")
	s = markdownQuoteRe.ReplaceAllString(s, "")
	s = markdownEmphasis.ReplaceAllString(s, "$1$2$3$4$5")
	return strings.TrimSpace(s)
}

// Paragraphs splits plain text into Telegraph paragraph blocks: double
// newlines start a new block, single newlines collapse into spaces, empty
// blocks are dropped.
func Paragraphs(text string) []string {
	var out []string
	for _, block := range paragraphSplitRe.Split(text, -1) {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		out = append(out, strings.ReplaceAll(block, "\n", " "))
	}
	return out
}
