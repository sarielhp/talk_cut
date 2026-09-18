// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"talk_cut/internal/config"
)

func TestPrintDetailedSetupGuide(t *testing.T) {
	var buf bytes.Buffer
	PrintDetailedSetupGuide(&buf)

	output := buf.String()
	requiredSnippets := []string{
		"TALK_CUT YOUTUBE SETUP GUIDE",
		"gcloud services enable youtube.googleapis.com",
		"OAuth consent screen",
		"Test Users (CRITICAL STEP)",
		"Desktop app",
		"access_denied",
		"redirect_uri_mismatch",
		"talk_cut youtube setup",
	}

	for _, snip := range requiredSnippets {
		if !strings.Contains(output, snip) {
			t.Errorf("expected guide to contain %q, but was missing", snip)
		}
	}
}

func TestMaskClientID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "short"},
		{"123456789012", "123456789012"},
		{"1234567890123456", "12345678...3456"},
		{"my-client-id-googleusercontent.com", "my-clien....com"},
	}

	for _, tt := range tests {
		got := maskClientID(tt.input)
		if got != tt.want {
			t.Errorf("maskClientID(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

func TestRunGuidedSetup_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	secFile := filepath.Join(tmpDir, "secrets.json")
	tokFile := filepath.Join(tmpDir, "token.json")

	// Create valid mock client secrets
	secJSON := []byte(`{"installed": {"client_id": "client_abc123456789", "client_secret": "sec_secret"}}`)
	if err := os.WriteFile(secFile, secJSON, 0600); err != nil {
		t.Fatalf("writing mock secrets: %v", err)
	}

	cfg := config.Config{
		YouTubeSecrets: secFile,
	}

	// User inputs: channel "seminar", confirm auth "y"
	input := "seminar\ny\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	authorizerCalled := false
	mockAuthorizer := func(ctx context.Context, c config.Config, channel, sPath, tPath string) error {
		authorizerCalled = true
		if channel != "seminar" {
			t.Errorf("expected channel 'seminar', got %q", channel)
		}
		if sPath != secFile {
			t.Errorf("expected secPath %q, got %q", secFile, sPath)
		}
		return nil
	}

	opts := SetupOptions{
		SecretsPath: secFile,
		TokenPath:   tokFile,
	}

	err := RunGuidedSetup(context.Background(), in, &out, cfg, opts, mockAuthorizer)
	if err != nil {
		t.Fatalf("unexpected error in RunGuidedSetup: %v", err)
	}

	if !authorizerCalled {
		t.Errorf("mockAuthorizer was not called")
	}

	outStr := out.String()
	if !strings.Contains(outStr, "Successfully authorized YouTube channel") {
		t.Errorf("output missing success confirmation: %s", outStr)
	}
}

func TestRunGuidedSetup_DeclineBrowserAuth(t *testing.T) {
	tmpDir := t.TempDir()
	secFile := filepath.Join(tmpDir, "secrets.json")
	tokFile := filepath.Join(tmpDir, "token.json")

	secJSON := []byte(`{"installed": {"client_id": "client_abc123456789", "client_secret": "sec_secret"}}`)
	if err := os.WriteFile(secFile, secJSON, 0600); err != nil {
		t.Fatalf("writing mock secrets: %v", err)
	}

	cfg := config.Config{
		YouTubeSecrets: secFile,
	}

	// User inputs: channel "testch", decline browser auth "n"
	input := "testch\nn\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	authorizerCalled := false
	mockAuthorizer := func(ctx context.Context, c config.Config, channel, sPath, tPath string) error {
		authorizerCalled = true
		return nil
	}

	opts := SetupOptions{
		SecretsPath: secFile,
		TokenPath:   tokFile,
	}

	err := RunGuidedSetup(context.Background(), in, &out, cfg, opts, mockAuthorizer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if authorizerCalled {
		t.Errorf("expected authorizer NOT to be called when user declines")
	}

	if !strings.Contains(out.String(), "Setup paused") {
		t.Errorf("expected pause message in output: %s", out.String())
	}
}

func TestRunGuidedSetup_ExistingTokenDeclineReauth(t *testing.T) {
	tmpDir := t.TempDir()
	secFile := filepath.Join(tmpDir, "secrets.json")
	tokFile := filepath.Join(tmpDir, "token.json")

	secJSON := []byte(`{"installed": {"client_id": "client_abc123456789", "client_secret": "sec_secret"}}`)
	_ = os.WriteFile(secFile, secJSON, 0600)
	_ = os.WriteFile(tokFile, []byte(`{"access_token": "ya29.test"}`), 0600)

	cfg := config.Config{
		YouTubeSecrets: secFile,
	}

	// User inputs: channel "default", decline reauth "n"
	input := "default\nn\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	authorizerCalled := false
	mockAuthorizer := func(ctx context.Context, c config.Config, channel, sPath, tPath string) error {
		authorizerCalled = true
		return nil
	}

	opts := SetupOptions{
		SecretsPath: secFile,
		TokenPath:   tokFile,
	}

	err := RunGuidedSetup(context.Background(), in, &out, cfg, opts, mockAuthorizer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if authorizerCalled {
		t.Errorf("expected authorizer NOT to be called when keeping existing token")
	}

	if !strings.Contains(out.String(), "keeping existing authorization") {
		t.Errorf("expected keeping existing auth message: %s", out.String())
	}
}

func TestRunYouTubeStatus(t *testing.T) {
	tmpDir := t.TempDir()
	secFile := filepath.Join(tmpDir, "secrets.json")
	tokFile := filepath.Join(tmpDir, "token.json")

	secJSON := []byte(`{"installed": {"client_id": "client_123456789012", "client_secret": "sec"}}`)
	_ = os.WriteFile(secFile, secJSON, 0600)

	token := &oauth2.Token{
		AccessToken:  "token123",
		RefreshToken: "refresh123",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	if err := SaveToken(tokFile, token); err != nil {
		t.Fatalf("saving token: %v", err)
	}

	cfg := config.Config{
		YouTubeSecrets: secFile,
		DefaultChannel: "seminar",
		Channels: map[string]string{
			"seminar": tokFile,
		},
	}

	var buf bytes.Buffer
	err := RunYouTubeStatus(&buf, cfg)
	if err != nil {
		t.Fatalf("unexpected status error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Client Secrets:  ✔ Found") {
		t.Errorf("status missing client secrets confirmation: %s", out)
	}
	if !strings.Contains(out, "Default Channel: seminar") {
		t.Errorf("status missing default channel: %s", out)
	}
	if !strings.Contains(out, "Default Privacy: public") {
		t.Errorf("status missing default privacy: %s", out)
	}
	if !strings.Contains(out, "✔ Authorized") {
		t.Errorf("status missing token authorized badge: %s", out)
	}
}
