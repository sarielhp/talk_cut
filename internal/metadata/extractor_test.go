package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleNYUSeminarHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>The Minimum Dominating Set Problem on Bipartite Circle Graphs: Complexity and Approximation | Department of Mathematics | NYU Courant</title>
</head>
<body>
  <h1>Geometry Seminar</h1>
  <h4>The Minimum Dominating Set Problem on Bipartite Circle Graphs: Complexity and Approximation</h4>
  <p><b>Speaker:</b> <a href="https://example.com/abu_affash">Karim Abu-Affash</a>, Shamoon College of Engineering</p>
  <p><b>Location:</b> Online</p>
  <p><b>Date:</b> Tuesday, September 8, 2026, 2 p.m.</p>
  <p><b>Synopsis:</b></p>
  <p>A circle graph is the intersection graph of a set of chords in a circle. A dominating set of a graph G=(V,E) is a subset D.</p>
  <p>In this work, we study the minimum dominating set problem on bipartite circle graphs.</p>
  <p><b>Notes:</b></p>
  <p>Only on Zoom. Please contact organizer for details.</p>
</body>
</html>`

func TestExtractTalkInfoNYU(t *testing.T) {
	url := "https://math.nyu.edu/dynamic/calendars/seminars/geometry-seminar/4488/"
	info, err := ExtractTalkInfo(strings.NewReader(sampleNYUSeminarHTML), url)
	if err != nil {
		t.Fatalf("ExtractTalkInfo failed: %v", err)
	}

	expectedTitle := "The Minimum Dominating Set Problem on Bipartite Circle Graphs: Complexity and Approximation"
	if info.Title != expectedTitle {
		t.Errorf("title: expected %q, got %q", expectedTitle, info.Title)
	}

	if info.Speaker != "Karim Abu-Affash" {
		t.Errorf("speaker: expected Karim Abu-Affash, got %q", info.Speaker)
	}

	if info.Affiliation != "Shamoon College of Engineering" {
		t.Errorf("affiliation: expected Shamoon College of Engineering, got %q", info.Affiliation)
	}

	if info.Seminar != "Geometry Seminar" {
		t.Errorf("seminar: expected Geometry Seminar, got %q", info.Seminar)
	}

	if info.Date != "Tuesday, September 8, 2026, 2 p.m." {
		t.Errorf("date: expected Tuesday, September 8, 2026, 2 p.m., got %q", info.Date)
	}

	// Verify abstract contains paragraphs AND the URL
	if !strings.Contains(info.Abstract, "A circle graph is the intersection graph") {
		t.Errorf("abstract missing paragraph 1: %q", info.Abstract)
	}
	if !strings.Contains(info.Abstract, "In this work, we study") {
		t.Errorf("abstract missing paragraph 2: %q", info.Abstract)
	}
	if !strings.Contains(info.Abstract, url) {
		t.Errorf("abstract must contain provided url, got: %q", info.Abstract)
	}
	if strings.Contains(info.Abstract, "Only on Zoom") {
		t.Errorf("abstract should not leak notes section, got: %q", info.Abstract)
	}
}

func TestParseSpeakerVariants(t *testing.T) {
	tests := []struct {
		input       string
		wantSpeaker string
		wantAffil   string
	}{
		{"Karim Abu-Affash, Shamoon College of Engineering", "Karim Abu-Affash", "Shamoon College of Engineering"},
		{"Alice Smith (MIT)", "Alice Smith", "MIT"},
		{"Speaker: Bob Jones, Stanford University", "Bob Jones", "Stanford University"},
		{"Charlie Brown", "Charlie Brown", ""},
	}

	for _, tt := range tests {
		spk, aff := parseSpeaker(tt.input)
		if spk != tt.wantSpeaker || aff != tt.wantAffil {
			t.Errorf("parseSpeaker(%q) = (%q, %q), want (%q, %q)", tt.input, spk, aff, tt.wantSpeaker, tt.wantAffil)
		}
	}
}

func TestAppendURLToAbstract(t *testing.T) {
	url := "https://example.com/talk/123"
	// Empty abstract
	res1 := appendURLToAbstract("", url)
	if !strings.Contains(res1, url) {
		t.Errorf("expected URL in empty abstract, got: %q", res1)
	}

	// Abstract without URL
	res2 := appendURLToAbstract("This is a summary of the talk.", url)
	if !strings.Contains(res2, "This is a summary of the talk.") || !strings.Contains(res2, url) {
		t.Errorf("expected text and URL in abstract, got: %q", res2)
	}

	// Abstract that already has URL
	res3 := appendURLToAbstract("Talk announcement: "+url, url)
	if strings.Count(res3, url) != 1 {
		t.Errorf("expected URL not to be duplicated, got: %q", res3)
	}
}

func TestFetchTalkInfoServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(sampleNYUSeminarHTML))
	}))
	defer ts.Close()

	info, err := FetchTalkInfo(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("FetchTalkInfo failed: %v", err)
	}
	if info.Speaker != "Karim Abu-Affash" {
		t.Errorf("expected speaker Karim Abu-Affash, got %q", info.Speaker)
	}
	if !strings.Contains(info.Abstract, ts.URL) {
		t.Errorf("expected abstract to contain server URL %q, got %q", ts.URL, info.Abstract)
	}
}
