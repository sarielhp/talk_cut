package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"talk_cut/internal/config"
)

func TestPolishTranscriptMock(t *testing.T) {
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
					Content: `{"transcript": "Speaker: Cleaned text."}`,
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
	client, err := NewClient(config.Config{BaseURL: server.URL, Model: "mock-model"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	got, err := PolishTranscript(context.Background(), client, "raw um text", "A Talk")
	if err != nil {
		t.Fatalf("PolishTranscript failed: %v", err)
	}
	if got != "Speaker: Cleaned text." {
		t.Errorf("unexpected polished transcript: %q", got)
	}
}

func TestPolishTranscriptEmptyInput(t *testing.T) {
	got, err := PolishTranscript(context.Background(), nil, "   ", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty output, got %q", got)
	}
}
