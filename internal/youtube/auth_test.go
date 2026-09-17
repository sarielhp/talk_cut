// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestParseSecretsData(t *testing.T) {
	installedJSON := []byte(`{
		"installed": {
			"client_id": "client_123.apps.googleusercontent.com",
			"client_secret": "secret_abc"
		}
	}`)
	creds, err := parseSecretsData(installedJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientID != "client_123.apps.googleusercontent.com" || creds.ClientSecret != "secret_abc" {
		t.Errorf("mismatch in parsed creds: %+v", creds)
	}

	webJSON := []byte(`{
		"web": {
			"client_id": "client_web.apps.googleusercontent.com",
			"client_secret": "secret_web"
		}
	}`)
	credsWeb, err := parseSecretsData(webJSON)
	if err != nil {
		t.Fatalf("unexpected web error: %v", err)
	}
	if credsWeb.ClientID != "client_web.apps.googleusercontent.com" {
		t.Errorf("mismatch in web creds: %+v", credsWeb)
	}

	flatJSON := []byte(`{
		"client_id": "flat_id",
		"client_secret": "flat_secret"
	}`)
	credsFlat, err := parseSecretsData(flatJSON)
	if err != nil {
		t.Fatalf("unexpected flat error: %v", err)
	}
	if credsFlat.ClientID != "flat_id" {
		t.Errorf("mismatch in flat creds: %+v", credsFlat)
	}

	invalidJSON := []byte(`{"foo": "bar"}`)
	if _, err := parseSecretsData(invalidJSON); err == nil {
		t.Errorf("expected error on invalid secrets json")
	}
}

func TestSaveAndLoadToken(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "test_token.json")

	token := &oauth2.Token{
		AccessToken:  "ya29.test_access_token",
		TokenType:    "Bearer",
		RefreshToken: "1//test_refresh_token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	if err := SaveToken(tokenPath, token); err != nil {
		t.Fatalf("saving token failed: %v", err)
	}

	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file mode 0600, got %v", info.Mode().Perm())
	}

	loaded, err := LoadToken(tokenPath)
	if err != nil {
		t.Fatalf("loading token failed: %v", err)
	}
	if loaded.AccessToken != token.AccessToken || loaded.RefreshToken != token.RefreshToken {
		t.Errorf("loaded token mismatch: %+v vs %+v", loaded, token)
	}
}
