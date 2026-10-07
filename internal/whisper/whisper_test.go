package whisper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestInferenceEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://tqed:8095":           "http://tqed:8095/inference",
		"http://tqed:8095/":          "http://tqed:8095/inference",
		"http://tqed:8095/inference": "http://tqed:8095/inference",
		"  http://tqed:8095/  ":      "http://tqed:8095/inference",
	}
	for in, want := range cases {
		if got := inferenceEndpoint(in); got != want {
			t.Errorf("inferenceEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://tqed:8095":           "http://tqed:8095/health",
		"http://tqed:8095/":          "http://tqed:8095/health",
		"http://tqed:8095/inference": "http://tqed:8095/health",
		"":                           "",
	}
	for in, want := range cases {
		if got := healthEndpoint(in); got != want {
			t.Errorf("healthEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewClientRequiresURL(t *testing.T) {
	if _, err := NewClient(Options{}); err == nil {
		t.Fatalf("expected error for empty URL")
	}
}

func TestNewClientDefaultsLanguage(t *testing.T) {
	client, err := NewClient(Options{URL: "http://example:8095"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.language != "en" {
		t.Errorf("expected default language en, got %q", client.language)
	}
}

func TestTranscribe(t *testing.T) {
	got := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/inference":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Errorf("ParseMultipartForm: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			got["format"] = r.FormValue("response_format")
			got["language"] = r.FormValue("language")
			got["prompt"] = r.FormValue("prompt")
			got["diarize"] = r.FormValue("diarize")
			got["align"] = r.FormValue("align")
			if _, _, err := r.FormFile("file"); err != nil {
				t.Errorf("missing file part: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"text": " Hello there.",
				"segments": []map[string]any{
					{"start": 0.0, "end": 2.5, "text": " Hello there.", "speaker": "SPEAKER_00"},
					{"start": 2.5, "end": 3.0, "text": "   ", "speaker": "SPEAKER_00"},
					{"start": 3.0, "end": 7.0, "text": " General Kenobi.", "speaker": nil},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	client, err := NewClient(Options{URL: server.URL, Language: "en", Prompt: "names"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.healthInterval = 10 * time.Millisecond

	cues, err := client.Transcribe(context.Background(), audio)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	for key, want := range map[string]string{
		"format": "verbose_json", "language": "en", "prompt": "names",
		"diarize": "true", "align": "true",
	} {
		if got[key] != want {
			t.Errorf("field %s = %q, want %q", key, got[key], want)
		}
	}

	if len(cues) != 2 {
		t.Fatalf("expected 2 non-empty cues, got %d", len(cues))
	}
	if cues[0].Text != "Hello there." || cues[0].Speaker != "SPEAKER_00" {
		t.Errorf("unexpected first cue: %+v", cues[0])
	}
	if cues[0].Start != 0 || cues[0].End != 2500*time.Millisecond {
		t.Errorf("unexpected first timing: %v-%v", cues[0].Start, cues[0].End)
	}
	if cues[1].Text != "General Kenobi." || cues[1].Speaker != "" {
		t.Errorf("expected null speaker to become empty, got: %+v", cues[1])
	}
	if cues[0].ID != 1 || cues[1].ID != 2 {
		t.Errorf("expected sequential ids, got %d,%d", cues[0].ID, cues[1].ID)
	}
}

// TestTranscribeWakesSuspendedServer models Sablier: /health serves the holding
// page until the container is resumed.
func TestTranscribeWakesSuspendedServer(t *testing.T) {
	var healthCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			if atomic.AddInt32(&healthCalls, 1) <= 2 {
				_, _ = w.Write([]byte(`<html><title>Sablier</title><body>starting</body></html>`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/inference":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"segments": []map[string]any{{"start": 0.0, "end": 1.0, "text": "Hi.", "speaker": "SPEAKER_00"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.healthInterval = 5 * time.Millisecond
	client.readyTimeout = 2 * time.Second

	audio := writeTinyAudio(t)
	cues, err := client.Transcribe(context.Background(), audio)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if len(cues) != 1 || cues[0].Speaker != "SPEAKER_00" {
		t.Fatalf("unexpected cues: %+v", cues)
	}
}

// TestTranscribeRetriesStartingServer covers /inference returning the holding
// page (or a 503) before the real response.
func TestTranscribeRetriesStartingServer(t *testing.T) {
	var inferenceCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			http.NotFound(w, r)
		case "/inference":
			if atomic.AddInt32(&inferenceCalls, 1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`<html><title>Sablier</title></html>`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"segments": []map[string]any{{"start": 0.0, "end": 1.0, "text": "Hi."}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.healthInterval = 5 * time.Millisecond
	client.readyTimeout = 50 * time.Millisecond

	cues, err := client.Transcribe(context.Background(), writeTinyAudio(t))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if len(cues) != 1 {
		t.Fatalf("expected 1 cue, got %d", len(cues))
	}
}

func TestTranscribeErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inference" {
			http.Error(w, "boom", http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient(Options{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.healthInterval = 5 * time.Millisecond
	if _, err := client.Transcribe(context.Background(), writeTinyAudio(t)); err == nil {
		t.Fatalf("expected error for non-200 response")
	}
}

func TestTranscribeNoSegments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inference" {
			_ = json.NewEncoder(w).Encode(map[string]any{"text": "", "segments": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient(Options{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.healthInterval = 5 * time.Millisecond
	if _, err := client.Transcribe(context.Background(), writeTinyAudio(t)); err == nil {
		t.Fatalf("expected error when no speech segments are returned")
	}
}

func TestIsWaitingPage(t *testing.T) {
	if !isWaitingPage([]byte(`<html><title>Sablier</title></html>`)) {
		t.Errorf("expected Sablier page to be detected")
	}
	if isWaitingPage([]byte(`{"segments":[]}`)) {
		t.Errorf("did not expect a JSON response to be treated as a waiting page")
	}
}

func writeTinyAudio(t *testing.T) string {
	t.Helper()
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	return audio
}
