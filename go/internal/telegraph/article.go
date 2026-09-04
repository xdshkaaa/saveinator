package telegraph

import (
	"encoding/json"
	"fmt"

	"saveinator/internal/reddit"
)

// Telegraph API limits.
const (
	maxTitleLen   = 256
	maxContentLen = 64 << 10
)

// ArticleOptions tweaks the generated article.
type ArticleOptions struct {
	// CommentsHeading labels the comment section (localized by the caller);
	// empty means no comment section is rendered.
	CommentsHeading string
	// SourceLabel is the text of the "source" link pointing at the thread;
	// empty omits the line.
	SourceLabel string
}

// Article converts a Reddit thread into a Telegraph title + node list:
// a meta line, a source link, the post text and the top comments.
func Article(t *reddit.Thread, opts ArticleOptions) (string, []Node) {
	title := truncate(t.Title, maxTitleLen)
	if title == "" {
		title = "Reddit"
	}

	var nodes []Node
	nodes = append(nodes, MetaLine(t))
	if opts.SourceLabel != "" {
		nodes = append(nodes, SourceLine(opts.SourceLabel, t.Permalink))
	}
	nodes = append(nodes, HR())

	for _, p := range reddit.Paragraphs(reddit.ToPlainText(t.Selftext)) {
		nodes = append(nodes, Paragraph(p))
	}

	if opts.CommentsHeading != "" && len(t.Comments) > 0 {
		nodes = append(nodes, HR(), Heading(opts.CommentsHeading))
		for _, c := range t.Comments {
			nodes = append(nodes, CommentNodes(c)...)
		}
	}

	// Telegraph rejects pages over ~64KB of content: drop comments from the
	// tail first, then post paragraphs, until the payload fits.
	for len(nodes) > 1 && sizeOf(nodes) > maxContentLen {
		if last := lastNodeTag(nodes, "blockquote"); last >= 0 {
			nodes = append(nodes[:last], nodes[last+1:]...)
			// also drop the comment header line right before it
			if last > 0 && nodes[last-1].Tag == "p" {
				nodes = append(nodes[:last-1], nodes[last:]...)
			}
			continue
		}
		if last := lastNodeTag(nodes, "p"); last >= 0 {
			nodes = append(nodes[:last], nodes[last+1:]...)
			continue
		}
		break
	}

	return title, nodes
}

// MetaLine renders "r/sub · u/author · ▲ score · 💬 N" as the first line.
func MetaLine(t *reddit.Thread) Node {
	sub := t.Subreddit
	if sub == "" {
		sub = "reddit"
	}
	author := t.Author
	if author == "" || author == "[deleted]" {
		author = "unknown"
	}
	meta := fmt.Sprintf("r/%s · u/%s · ▲ %d · 💬 %d", sub, author, t.Score, t.NumComments)
	return Node{Tag: "p", Children: []any{Node{Tag: "b", Children: []any{meta}}}}
}

// SourceLine renders "<label>: r/<sub>" linking to the thread.
func SourceLine(label, href string) Node {
	return Node{
		Tag: "p",
		Children: []any{
			Node{Tag: "a", Attrs: map[string]string{"href": href}, Children: []any{label}},
		},
	}
}

// CommentNodes returns the nodes for one comment: header line + blockquote.
func CommentNodes(c reddit.Comment) []Node {
	header := fmt.Sprintf("u/%s (+%d)", c.Author, c.Score)
	block := Node{Tag: "blockquote"}
	for _, p := range reddit.Paragraphs(reddit.ToPlainText(c.Body)) {
		block.Children = append(block.Children, Node{Tag: "p", Children: []any{p}})
	}
	if len(block.Children) == 0 {
		block.Children = append(block.Children, "")
	}
	return []Node{
		{Tag: "p", Children: []any{Node{Tag: "b", Children: []any{header}}}},
		block,
	}
}

// Paragraph returns a plain <p> node.
func Paragraph(text string) Node {
	return Node{Tag: "p", Children: []any{text}}
}

// Heading returns an <h3> node.
func Heading(text string) Node {
	return Node{Tag: "h3", Children: []any{text}}
}

// HR returns a horizontal rule node.
func HR() Node {
	return Node{Tag: "hr"}
}

func sizeOf(nodes []Node) int {
	b, err := json.Marshal(nodes)
	if err != nil {
		return 0
	}
	return len(b)
}

func lastNodeTag(nodes []Node, tag string) int {
	for i := len(nodes) - 1; i >= 0; i-- {
		if nodes[i].Tag == tag {
			return i
		}
	}
	return -1
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
