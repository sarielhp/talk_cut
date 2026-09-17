package metadata

import (
	"strings"
	"testing"
)

func TestExtractText(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<head>
	<title>Theory Seminar: Minimum Dominating Sets</title>
	<meta name="description" content="Talk on bipartite circle graphs by Karim Abu-Affash.">
	<style>body { font-family: sans-serif; }</style>
	<script>console.log("ignore");</script>
</head>
<body>
	<nav><a href="/">Home</a></nav>
	<main>
		<h1>Seminar Announcement</h1>
		<p>Speaker: Karim Abu-Affash (Shamoon College of Engineering)</p>
		<p>Abstract: In this talk we discuss the NP-hard minimum dominating set problem.</p>
	</main>
	<footer>Copyright 2026</footer>
</body>
</html>`

	content, err := ExtractText(strings.NewReader(rawHTML), "https://example.com/talk")
	if err != nil {
		t.Fatalf("ExtractText failed: %v", err)
	}

	if content.Title != "Theory Seminar: Minimum Dominating Sets" {
		t.Errorf("title mismatch: %q", content.Title)
	}

	if content.Description != "Talk on bipartite circle graphs by Karim Abu-Affash." {
		t.Errorf("meta description mismatch: %q", content.Description)
	}

	if strings.Contains(content.BodyText, "console.log") {
		t.Errorf("body text should not contain script contents")
	}

	if strings.Contains(content.BodyText, "Copyright 2026") {
		t.Errorf("body text should not contain footer contents")
	}

	if !strings.Contains(content.BodyText, "Shamoon College of Engineering") {
		t.Errorf("expected speaker info in body text, got: %q", content.BodyText)
	}
}
