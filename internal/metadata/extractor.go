// Package metadata fetches and extracts structured information from talk announcement URLs.
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

// TalkPageInfo holds structured fields extracted from a seminar announcement page.
type TalkPageInfo struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Speaker     string   `json:"speaker"`
	Affiliation string   `json:"affiliation,omitempty"`
	Abstract    string   `json:"abstract"`
	Seminar     string   `json:"seminar,omitempty"`
	Date        string   `json:"date,omitempty"`
	Location    string   `json:"location,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	RawBody     string   `json:"raw_body,omitempty"`
}

// FetchTalkInfo downloads and extracts structured talk metadata from a URL.
func FetchTalkInfo(ctx context.Context, pageURL string) (TalkPageInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return TalkPageInfo{}, fmt.Errorf("creating request for %q: %w", pageURL, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (talk_cut; seminar metadata extractor)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return TalkPageInfo{}, fmt.Errorf("fetching %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TalkPageInfo{}, fmt.Errorf("fetching %q returned status %d", pageURL, resp.StatusCode)
	}

	return ExtractTalkInfo(resp.Body, pageURL)
}

// ExtractTalkInfo parses an HTML document and extracts structured talk information.
func ExtractTalkInfo(r io.Reader, pageURL string) (TalkPageInfo, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return TalkPageInfo{}, fmt.Errorf("parsing html: %w", err)
	}

	info := TalkPageInfo{URL: pageURL}
	collectPageMetadata(doc, &info)

	if info.Abstract != "" || pageURL != "" {
		info.Abstract = appendURLToAbstract(info.Abstract, pageURL)
	}
	if info.Seminar != "" {
		info.Tags = append(info.Tags, info.Seminar)
	}

	return info, nil
}

// collectPageMetadata traverses the HTML DOM to populate TalkPageInfo fields.
func collectPageMetadata(root *html.Node, info *TalkPageInfo) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			processElement(n, info)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
}

// processElement inspects an individual HTML element node for talk attributes.
func processElement(n *html.Node, info *TalkPageInfo) {
	tag := strings.ToLower(n.Data)
	switch tag {
	case "title":
		if info.Title == "" {
			info.Title = cleanTitle(extractAllText(n))
		}
	case "h1", "h2":
		txt := extractAllText(n)
		if isSeminarHeading(txt) && info.Seminar == "" {
			info.Seminar = txt
		}
	case "h3", "h4":
		txt := extractAllText(n)
		if len(txt) > 15 && !isSeminarHeading(txt) && (info.Title == "" || len(txt) > len(info.Title)) {
			info.Title = cleanTitle(txt)
		}
	case "p", "div", "li":
		processParagraph(n, info)
	}
}

// isSeminarHeading returns true if the text matches a seminar series title.
func isSeminarHeading(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "seminar") ||
		strings.Contains(lower, "colloquium") ||
		strings.Contains(lower, "workshop")
}

// processParagraph checks a block node for labelled metadata lines.
func processParagraph(n *html.Node, info *TalkPageInfo) {
	label, rest := extractLabeledText(n)
	if label == "" {
		return
	}

	switch label {
	case "speaker":
		if info.Speaker == "" {
			info.Speaker, info.Affiliation = parseSpeaker(rest)
		}
	case "date":
		if info.Date == "" {
			info.Date = rest
		}
	case "location":
		if info.Location == "" {
			info.Location = rest
		}
	case "synopsis", "abstract":
		if info.Abstract == "" {
			info.Abstract = collectAbstract(n, rest)
		}
	}
}

// extractLabeledText extracts a leading label (e.g. "Speaker:") and trailing text from a block.
func extractLabeledText(n *html.Node) (label, rest string) {
	txt := extractAllText(n)
	lower := strings.ToLower(txt)

	for _, candidate := range []string{"speaker", "date", "location", "synopsis", "abstract"} {
		prefix := candidate + ":"
		if strings.HasPrefix(lower, prefix) {
			return candidate, strings.TrimSpace(txt[len(prefix):])
		}
	}

	return "", ""
}

// collectAbstract gathers abstract text from the label node and subsequent siblings.
func collectAbstract(labelNode *html.Node, initialText string) string {
	var paragraphs []string
	if initialText != "" {
		paragraphs = append(paragraphs, initialText)
	}

	cur := labelNode.NextSibling
	for cur != nil {
		if cur.Type == html.ElementNode && cur.Data == "p" {
			lbl, _ := extractLabeledText(cur)
			if lbl != "" || isSectionHeader(cur) {
				break
			}
			pText := extractAllText(cur)
			if pText != "" {
				paragraphs = append(paragraphs, pText)
			}
		}
		cur = cur.NextSibling
	}

	return strings.Join(paragraphs, "\n\n")
}

// isSectionHeader checks if a paragraph is a section break.
func isSectionHeader(n *html.Node) bool {
	txt := strings.ToLower(extractAllText(n))
	for _, term := range []string{"notes:", "bio:", "contact:", "organizer:"} {
		if strings.HasPrefix(txt, term) {
			return true
		}
	}
	return false
}

// extractAllText recursively concatenates all text under a node.
func extractAllText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(strings.Join(strings.Fields(sb.String()), " "))
}

// parseSpeaker splits raw speaker string into speaker name and affiliation.
func parseSpeaker(raw string) (speaker, affiliation string) {
	s := strings.TrimSpace(raw)
	for _, prefix := range []string{"speaker:", "speaker", "by:", "presenter:"} {
		if strings.HasPrefix(strings.ToLower(s), prefix) {
			s = strings.TrimSpace(s[len(prefix):])
			break
		}
	}

	if open := strings.Index(s, "("); open > 0 {
		if closeIdx := strings.LastIndex(s, ")"); closeIdx > open {
			return strings.TrimSpace(s[:open]), strings.TrimSpace(s[open+1 : closeIdx])
		}
	}

	if idx := strings.Index(s, ","); idx > 0 {
		return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
	}
	return s, ""
}

// cleanTitle strips standard institutional suffixes from page title tags.
func cleanTitle(raw string) string {
	s := strings.TrimSpace(raw)
	for _, sep := range []string{" | ", " - ", " — "} {
		if idx := strings.Index(s, sep); idx > 0 {
			s = strings.TrimSpace(s[:idx])
			break
		}
	}
	for _, pref := range []string{"seminar talk:", "colloquium:", "talk:"} {
		if strings.HasPrefix(strings.ToLower(s), pref) {
			s = strings.TrimSpace(s[len(pref):])
			break
		}
	}
	return s
}

// appendURLToAbstract guarantees the provided URL is included in the abstract body.
func appendURLToAbstract(abstract, pageURL string) string {
	cleanURL := strings.TrimSpace(pageURL)
	if cleanURL == "" {
		return strings.TrimSpace(abstract)
	}
	if strings.Contains(abstract, cleanURL) {
		return strings.TrimSpace(abstract)
	}
	trimmed := strings.TrimSpace(abstract)
	if trimmed == "" {
		return "Talk announcement: " + cleanURL
	}
	return trimmed + "\n\nTalk announcement: " + cleanURL
}
