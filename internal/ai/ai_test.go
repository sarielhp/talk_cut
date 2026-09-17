package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
)

func TestSanitizeJSONContent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"{\"key\": \"val\"}", "{\"key\": \"val\"}"},
		{"```json\n{\"key\": \"val\"}\n```", "{\"key\": \"val\"}"},
		{"```\n{\"key\": \"val\"}\n```", "{\"key\": \"val\"}"},
	}

	for _, tt := range tests {
		got := sanitizeJSONContent(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeJSONContent(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDetectCutsMock(t *testing.T) {
	mockResponse := chatResponse{
		Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			{
				Message: struct {
					Content string `json:"content"`
				}{
					Content: `{"cuts": [{"start": "00:00:00.000", "end": "00:00:28.750", "reason": "Host intro"}]}`,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	cfg := config.Config{
		OpenRouterKey: "mock-key",
		BaseURL:       server.URL,
		Model:         "mock-model",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	cues := []model.SubtitleCue{
		{ID: 1, Start: 0, End: 5 * time.Second, Speaker: "Host", Text: "Welcome"},
		{ID: 2, Start: 28 * time.Second, End: 35 * time.Second, Speaker: "Speaker", Text: "Hello"},
	}

	cuts, err := DetectCuts(context.Background(), client, cues)
	if err != nil {
		t.Fatalf("DetectCuts failed: %v", err)
	}

	if len(cuts) != 1 {
		t.Fatalf("expected 1 cut, got %d", len(cuts))
	}

	if cuts[0].Start != 0 || cuts[0].End != 28*time.Second+750*time.Millisecond {
		t.Errorf("cut interval mismatch: %+v", cuts[0])
	}
}

func TestExtractTalkMetadataMock(t *testing.T) {
	mockResponse := chatResponse{
		Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			{
				Message: struct {
					Content string `json:"content"`
				}{
					Content: `{
						"title": "Minimum Dominating Sets",
						"speaker": "Karim Abu-Affash",
						"affiliation": "Shamoon College of Engineering",
						"abstract": "We study graph algorithms.",
						"tags": ["graphs", "algorithms"],
						"suggested_chapters": [
							{"timestamp": "00:00:28.750", "title": "Talk Start"}
						]
					}`,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	cfg := config.Config{
		OpenRouterKey: "mock-key",
		BaseURL:       server.URL,
		Model:         "mock-model",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	meta, err := ExtractTalkMetadata(context.Background(), client, "Seminar talk announcement", nil)
	if err != nil {
		t.Fatalf("ExtractTalkMetadata failed: %v", err)
	}

	if meta.Title != "Minimum Dominating Sets" {
		t.Errorf("title mismatch: %q", meta.Title)
	}
	if meta.Speaker != "Karim Abu-Affash" {
		t.Errorf("speaker mismatch: %q", meta.Speaker)
	}
	if len(meta.Chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(meta.Chapters))
	}
}
