package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
)

func TestEnrichMetadataFromAI(t *testing.T) {
	target := model.TalkMetadata{
		Title:   "Manual Title",
		Speaker: "",
	}
	aiMeta := model.TalkMetadata{
		Title:       "AI Suggested Title",
		Speaker:     "Dr. Alice",
		Affiliation: "UIUC",
		Abstract:    "Abstract from AI.",
		Tags:        []string{"geometry", "algorithms"},
	}

	enrichMetadataFromAI(&target, aiMeta)

	// Manual Title should NOT be overwritten
	if target.Title != "Manual Title" {
		t.Errorf("expected Title %q, got %q", "Manual Title", target.Title)
	}

	// Empty fields should be supplemented
	if target.Speaker != "Dr. Alice" {
		t.Errorf("expected Speaker %q, got %q", "Dr. Alice", target.Speaker)
	}
	if target.Affiliation != "UIUC" {
		t.Errorf("expected Affiliation %q, got %q", "UIUC", target.Affiliation)
	}
	if target.Abstract != "Abstract from AI." {
		t.Errorf("expected Abstract %q, got %q", "Abstract from AI.", target.Abstract)
	}
	if len(target.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(target.Tags))
	}
}

func TestFetchAndExtractFromURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		html := `<!doctype html><html><head><title>Seminar: Geometric Spanners</title></head><body>
		<h1>Geometry Colloquium</h1>
		<h4>Geometric Spanners and Applications</h4>
		<p><b>Speaker:</b> Michiel Smid, Carleton University</p>
		<p><b>Abstract:</b> We discuss spanners in geometric spaces.</p>
		</body></html>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := config.Config{}
	meta, err := fetchAndExtractFromURL(ctx, server.URL, cfg)
	if err != nil {
		t.Fatalf("fetchAndExtractFromURL error: %v", err)
	}

	if meta.Speaker != "Michiel Smid" {
		t.Errorf("expected speaker Michiel Smid, got %q", meta.Speaker)
	}
	if meta.Affiliation != "Carleton University" {
		t.Errorf("expected affiliation Carleton University, got %q", meta.Affiliation)
	}
	if meta.URL != server.URL {
		t.Errorf("expected URL %q, got %q", server.URL, meta.URL)
	}
}
