// Package whisper uploads extracted audio to a WhisperX /inference server and
// converts the returned timestamped segments into subtitle cues. WhisperX's
// diarization labels (SPEAKER_00, ...) are preserved; the same client also
// accepts plain whisper.cpp responses, which simply carry no speakers.
package whisper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"talk_cut/internal/model"
)

const (
	// defaultLanguage is used when the configuration does not specify one.
	defaultLanguage = "en"
	// requestTimeout bounds a single transcription request, independent of ctx.
	requestTimeout = 30 * time.Minute
	// defaultHealthInterval is the delay between readiness probes.
	defaultHealthInterval = 2 * time.Second
	// readyTimeout bounds how long we wait for a suspended server to wake.
	readyTimeout = 3 * time.Minute
	// healthProbeTimeout bounds a single readiness probe.
	healthProbeTimeout = 5 * time.Second
	// maxPostAttempts bounds retries while a server is still starting up.
	maxPostAttempts = 5
)

// Client talks to a Whisper server's /inference endpoint.
type Client struct {
	endpoint       string
	healthURL      string
	language       string
	prompt         string
	http           *http.Client
	healthInterval time.Duration
	readyTimeout   time.Duration
}

// Options configures a Client. URL is required; the rest are optional.
type Options struct {
	URL      string
	Language string
	Prompt   string
}

// NewClient builds a Client from configuration, normalising the base URL.
func NewClient(opt Options) (*Client, error) {
	base := strings.TrimSpace(opt.URL)
	if base == "" {
		return nil, fmt.Errorf("whisperx_url is not configured")
	}

	language := strings.TrimSpace(opt.Language)
	if language == "" {
		language = defaultLanguage
	}

	return &Client{
		endpoint:       inferenceEndpoint(base),
		healthURL:      healthEndpoint(base),
		language:       language,
		prompt:         strings.TrimSpace(opt.Prompt),
		http:           &http.Client{Timeout: requestTimeout},
		healthInterval: defaultHealthInterval,
		readyTimeout:   readyTimeout,
	}, nil
}

// inferenceEndpoint appends /inference unless the URL already targets it.
func inferenceEndpoint(base string) string {
	trimmed := stripInferenceSuffix(strings.TrimSpace(base))
	if trimmed == "" {
		return ""
	}
	return trimmed + "/inference"
}

// healthEndpoint derives the /health probe URL from a base URL.
func healthEndpoint(base string) string {
	trimmed := stripInferenceSuffix(strings.TrimSpace(base))
	if trimmed == "" {
		return ""
	}
	return trimmed + "/health"
}

// stripInferenceSuffix removes a trailing "/inference" and any trailing slash.
func stripInferenceSuffix(base string) string {
	trimmed := strings.TrimRight(base, "/")
	return strings.TrimSuffix(trimmed, "/inference")
}

// verboseResponse mirrors the subset of verbose_json output we use. Speaker is
// present for diarized servers (WhisperX) and absent otherwise.
type verboseResponse struct {
	Text     string `json:"text"`
	Segments []struct {
		Start   float64 `json:"start"`
		End     float64 `json:"end"`
		Text    string  `json:"text"`
		Speaker *string `json:"speaker"`
	} `json:"segments"`
}

// Transcribe waits for the server to be ready, uploads the audio file and
// returns the resulting cues. Sablier-suspended servers are woken first.
func (c *Client) Transcribe(ctx context.Context, audioPath string) ([]model.SubtitleCue, error) {
	c.waitForReady(ctx)

	data, err := c.postAudio(ctx, audioPath)
	if err != nil {
		return nil, err
	}

	var parsed verboseResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parsing whisper response: %w", err)
	}

	cues := cuesFromSegments(parsed)
	if len(cues) == 0 {
		return nil, fmt.Errorf("whisper server returned no speech segments")
	}
	return cues, nil
}

// waitForReady blocks until the server answers (waking a Sablier-suspended
// container) or the readiness deadline passes. It never returns an error: a
// failure here surfaces later as a clear error from postAudio.
func (c *Client) waitForReady(ctx context.Context) {
	if c.healthURL == "" {
		return
	}

	deadline := time.Now().Add(c.readyTimeout)
	for time.Now().Before(deadline) {
		if c.serverReady(ctx) {
			return
		}
		if err := sleepContext(ctx, c.healthInterval); err != nil {
			return
		}
	}
}

// serverReady reports whether the server is reachable and out of its waiting
// page. A 404 means the server has no /health route but is nonetheless up.
func (c *Client) serverReady(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, c.healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false
	}
	if isWaitingPage(data) {
		return false
	}
	return resp.StatusCode < http.StatusInternalServerError
}

// postAudio uploads the audio, retrying while the server is still starting up.
func (c *Client) postAudio(ctx context.Context, audioPath string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxPostAttempts; attempt++ {
		data, retryable, err := c.postOnce(ctx, audioPath)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retryable || attempt == maxPostAttempts {
			break
		}
		if sleepContext(ctx, c.healthInterval) != nil {
			break
		}
	}
	return nil, lastErr
}

// postOnce performs a single transcription request. The bool reports whether
// the request may succeed on retry (server still starting).
func (c *Client) postOnce(ctx context.Context, audioPath string) ([]byte, bool, error) {
	body, contentType, err := c.buildRequest(audioPath)
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		return nil, false, fmt.Errorf("building whisper request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("calling whisper server at %q: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, false, fmt.Errorf("reading whisper response: %w", readErr)
	}
	if isWaitingPage(data) {
		return nil, true, fmt.Errorf("whisper server is still starting up")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, isRetryableStatus(resp.StatusCode), fmt.Errorf("whisper server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, false, nil
}

// buildRequest assembles the multipart body with the audio file and options.
// Fields understood by only one server are ignored by the other.
func (c *Client) buildRequest(audioPath string) (io.Reader, string, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening audio %q: %w", audioPath, err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return nil, "", fmt.Errorf("creating multipart file field: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", fmt.Errorf("writing audio to request: %w", err)
	}

	for key, value := range c.formFields() {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("writing multipart field %q: %w", key, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("closing multipart body: %w", err)
	}
	return &buf, writer.FormDataContentType(), nil
}

// formFields returns the request fields shared by whisper.cpp and WhisperX.
func (c *Client) formFields() map[string]string {
	fields := map[string]string{
		"response_format": "verbose_json",
		"temperature":     "0.0",
		"temperature_inc": "0.2",
		"no_speech_thold": "0.6",
		"diarize":         "true",
		"align":           "true",
		"language":        c.language,
	}
	if c.prompt != "" {
		fields["prompt"] = c.prompt
	}
	return fields
}

// cuesFromSegments converts returned segments into SubtitleCues, skipping blanks.
func cuesFromSegments(resp verboseResponse) []model.SubtitleCue {
	cues := make([]model.SubtitleCue, 0, len(resp.Segments))
	for _, seg := range resp.Segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		cues = append(cues, model.SubtitleCue{
			ID:      len(cues) + 1,
			Start:   secondsToDuration(seg.Start),
			End:     secondsToDuration(seg.End),
			Speaker: normalizeSpeaker(seg.Speaker),
			Text:    text,
		})
	}
	return cues
}

// normalizeSpeaker cleans a speaker label, treating absent or null as empty.
func normalizeSpeaker(raw *string) string {
	if raw == nil {
		return ""
	}
	label := strings.TrimSpace(*raw)
	if label == "" || strings.EqualFold(label, "null") {
		return ""
	}
	return label
}

// secondsToDuration converts a server's float seconds to a duration.
func secondsToDuration(sec float64) time.Duration {
	if sec < 0 {
		sec = 0
	}
	return time.Duration(sec * float64(time.Second))
}

// isWaitingPage reports whether a response body is Sablier's holding page,
// served while a suspended container is being resumed.
func isWaitingPage(data []byte) bool {
	return bytes.Contains(data, []byte("Sablier"))
}

// isRetryableStatus reports whether a status code indicates a server that is
// starting up rather than a permanent failure.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// sleepContext sleeps for d, returning early if ctx is cancelled.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
