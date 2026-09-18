package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
)

func TestDetectChaptersMock(t *testing.T) {
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
						"chapters": [
							{"timestamp": "00:00:39", "title": "Introduction"},
							{"timestamp": "00:01:20", "title": "Definitions & Dominating Sets"},
							{"timestamp": "00:10:15", "title": "NP-Hardness Reduction"},
							{"timestamp": "00:24:00", "title": "2-Approximation Algorithm"}
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

	t.Setenv("OPENROUTER_API_KEY", "mock-key")
	cfg := config.Config{
		BaseURL: server.URL,
		Model:   "mock-model",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	cues := []model.SubtitleCue{
		{ID: 1, Start: 39 * time.Second, End: 45 * time.Second, Speaker: "Speaker", Text: "Welcome to my talk."},
		{ID: 2, Start: 80 * time.Second, End: 90 * time.Second, Speaker: "Speaker", Text: "Let G be an undirected graph."},
	}

	chapters, err := DetectChapters(context.Background(), client, cues, "Sample Talk", "An abstract about graph theory.")
	if err != nil {
		t.Fatalf("DetectChapters failed: %v", err)
	}

	if len(chapters) != 4 {
		t.Fatalf("expected 4 chapters, got %d", len(chapters))
	}

	if chapters[0].Title != "Introduction" || chapters[0].OriginalTime != 39*time.Second {
		t.Errorf("chapter 0 mismatch: %+v", chapters[0])
	}
	if chapters[1].Title != "Definitions & Dominating Sets" || chapters[1].OriginalTime != 80*time.Second {
		t.Errorf("chapter 1 mismatch: %+v", chapters[1])
	}
	if chapters[2].Title != "NP-Hardness Reduction" || chapters[2].OriginalTime != 10*time.Minute+15*time.Second {
		t.Errorf("chapter 2 mismatch: %+v", chapters[2])
	}
	if chapters[3].Title != "2-Approximation Algorithm" || chapters[3].OriginalTime != 24*time.Minute {
		t.Errorf("chapter 3 mismatch: %+v", chapters[3])
	}
}

func TestBuildChapterTranscript(t *testing.T) {
	cues := []model.SubtitleCue{
		{Start: 0, End: 5 * time.Second, Speaker: "Host", Text: "Welcome everyone."},
		{Start: 5 * time.Second, End: 10 * time.Second, Speaker: "Host", Text: "Today we have Karim."},
		{Start: 28 * time.Second, End: 35 * time.Second, Speaker: "Speaker", Text: "Thank you for having me."},
		{Start: 70 * time.Second, End: 80 * time.Second, Speaker: "Speaker", Text: "Let us define the problem."},
	}

	transcript := buildChapterTranscript(cues)
	if !strings.Contains(transcript, "Host: Welcome everyone. Today we have Karim.") {
		t.Errorf("expected merged host cues, got: %s", transcript)
	}
	if !strings.Contains(transcript, "Speaker: Thank you for having me.") {
		t.Errorf("expected speaker cue, got: %s", transcript)
	}
	if !strings.Contains(transcript, "Speaker: Let us define the problem.") {
		t.Errorf("expected time-split speaker cue, got: %s", transcript)
	}
}

func TestParseChapterProposalDeduplication(t *testing.T) {
	proposal := rawChapterProposal{
		Chapters: []struct {
			Timestamp string `json:"timestamp"`
			Title     string `json:"title"`
		}{
			{"00:05:00", "Part 2"},
			{"00:00:10", "Part 1"},
			{"00:00:15", "Duplicate Part 1"}, // Within 5 seconds, should be dropped
			{"invalid", "Invalid Time"},      // Should be skipped
			{"00:10:00", ""},                 // Empty title should be skipped
		},
	}

	chapters, err := parseChapterProposal(proposal)
	if err != nil {
		t.Fatalf("parseChapterProposal failed: %v", err)
	}

	if len(chapters) != 2 {
		t.Fatalf("expected 2 valid deduplicated chapters, got %d", len(chapters))
	}

	if chapters[0].Title != "Part 1" || chapters[0].OriginalTime != 10*time.Second {
		t.Errorf("first chapter mismatch: %+v", chapters[0])
	}
	if chapters[1].Title != "Part 2" || chapters[1].OriginalTime != 5*time.Minute {
		t.Errorf("second chapter mismatch: %+v", chapters[1])
	}
}
