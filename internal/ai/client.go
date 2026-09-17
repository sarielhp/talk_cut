// Package ai provides an OpenRouter client and structured prompts for cut detection and metadata extraction.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"talk_cut/internal/config"
)

// Client sends requests to an OpenAI-compatible API endpoint (such as OpenRouter).
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// NewClient initializes an AI client from user configuration.
func NewClient(cfg config.Config) (*Client, error) {
	key := strings.TrimSpace(cfg.OpenRouterKey)
	if key == "" {
		return nil, fmt.Errorf("no OpenRouter API key found (set OPENROUTER_API_KEY or configure ~/.config/talk_cut/config.json)")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	model := cfg.Model
	if model == "" {
		model = "google/gemini-2.5-flash-lite"
	}

	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    baseURL,
		apiKey:     key,
		model:      model,
	}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Temperature    float64         `json:"temperature"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

// CompleteJSON sends a system and user prompt and parses the model's JSON response into dest.
func (c *Client) CompleteJSON(ctx context.Context, systemPrompt, userPrompt string, dest any) error {
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
		Temperature:    0.1,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling chat request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/chat/completions", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	httpReq.Header.Set("HTTP-Referer", "https://github.com/sariel/talk_cut")
	httpReq.Header.Set("X-Title", "talk_cut")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("calling AI API at %q: %w", endpoint, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AI API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return fmt.Errorf("unmarshaling chat response: %w", err)
	}

	if chatResp.Error != nil {
		return fmt.Errorf("AI API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return fmt.Errorf("AI API returned 0 choices")
	}

	rawJSON := chatResp.Choices[0].Message.Content
	rawJSON = sanitizeJSONContent(rawJSON)

	if err := json.Unmarshal([]byte(rawJSON), dest); err != nil {
		return fmt.Errorf("parsing structured JSON output: %w (raw response: %s)", err, rawJSON)
	}

	return nil
}

// sanitizeJSONContent strips markdown code fences if the model wraps JSON in ```json ... ```
func sanitizeJSONContent(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
