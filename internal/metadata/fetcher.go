// Package metadata fetches and sanitizes web content from talk announcement URLs.
package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// WebContent stores extracted title and readable body text from an HTML page.
type WebContent struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	BodyText    string `json:"body_text"`
}

// FetchTalkPage downloads and cleans an announcement web page.
func FetchTalkPage(ctx context.Context, pageURL string) (WebContent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return WebContent{}, fmt.Errorf("creating request for %q: %w", pageURL, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (talk_cut; seminar metadata crawler)")

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return WebContent{}, fmt.Errorf("fetching %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return WebContent{}, fmt.Errorf("fetching %q returned status %d", pageURL, resp.StatusCode)
	}

	return ExtractText(resp.Body, pageURL)
}

// ExtractText parses an HTML stream into clean, readable text.
func ExtractText(r io.Reader, pageURL string) (WebContent, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return WebContent{}, fmt.Errorf("parsing html: %w", err)
	}

	content := WebContent{URL: pageURL}
	var sb strings.Builder

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "script", "style", "noscript", "svg", "nav", "header", "footer":
				return
			case "title":
				content.Title = extractNodeText(n)
			case "meta":
				extractMetaTag(n, &content)
			}
		}

		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}

		if n.Type == html.ElementNode && isBlockElement(n.Data) {
			sb.WriteString("\n")
		}
	}

	traverse(doc)
	content.BodyText = cleanExtractedText(sb.String())
	return content, nil
}

// extractMetaTag inspects meta tags for description or og:description.
func extractMetaTag(n *html.Node, content *WebContent) {
	var name, prop, val string
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "name":
			name = strings.ToLower(attr.Val)
		case "property":
			prop = strings.ToLower(attr.Val)
		case "content":
			val = strings.TrimSpace(attr.Val)
		}
	}

	if (name == "description" || prop == "og:description") && content.Description == "" {
		content.Description = val
	}
	if prop == "og:title" && content.Title == "" {
		content.Title = val
	}
}

// extractNodeText concatenates all text nodes beneath an element.
func extractNodeText(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return strings.TrimSpace(sb.String())
}

// isBlockElement returns true for HTML elements that indicate a paragraph or section break.
func isBlockElement(tag string) bool {
	switch strings.ToLower(tag) {
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "article", "section", "br":
		return true
	default:
		return false
	}
}

// cleanExtractedText collapses consecutive spaces and excessive empty lines.
func cleanExtractedText(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string

	for _, line := range lines {
		trimmed := strings.Join(strings.Fields(line), " ")
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return strings.Join(cleaned, "\n")
}
