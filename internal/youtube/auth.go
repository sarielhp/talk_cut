// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"

	"talk_cut/internal/config"
)

var googleAuthEndpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.google.com/o/oauth2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
}

const (
	youtubeUploadScope = "https://www.googleapis.com/auth/youtube.upload"
	youtubeForceScope  = "https://www.googleapis.com/auth/youtube.force-ssl"
)

// ClientCredentials stores Google OAuth client credentials.
type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Authorize performs the interactive browser OAuth2 loopback authentication flow.
func Authorize(ctx context.Context, cfg config.Config, customSecrets, customToken string) error {
	secPath := cfg.YouTubeSecrets
	if customSecrets != "" {
		secPath = customSecrets
	}
	tokPath := cfg.YouTubeTokenFile
	if customToken != "" {
		tokPath = customToken
	}

	creds, err := LoadClientCredentials(secPath)
	if err != nil {
		return fmt.Errorf("loading client secrets: %w", err)
	}

	listener, port, err := startLoopbackListener()
	if err != nil {
		return fmt.Errorf("starting local auth server: %w", err)
	}
	defer listener.Close()

	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/oauth2callback", port)
	oauthConf := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     googleAuthEndpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{youtubeUploadScope, youtubeForceScope},
	}

	token, err := runLoopbackFlow(ctx, listener, oauthConf)
	if err != nil {
		return fmt.Errorf("authorizing token: %w", err)
	}

	if err := SaveToken(tokPath, token); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}

	cfg.YouTubeSecrets = secPath
	cfg.YouTubeTokenFile = tokPath
	if err := cfg.SaveConfig(); err != nil {
		return fmt.Errorf("updating config: %w", err)
	}

	return nil
}

// LoadClientCredentials reads and parses a Google client secrets JSON file.
func LoadClientCredentials(path string) (*ClientCredentials, error) {
	resolved, err := config.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("reading secrets file %q: %w", resolved, err)
	}

	return parseSecretsData(data)
}

// parseSecretsData extracts client ID and secret from various Google JSON formats.
func parseSecretsData(data []byte) (*ClientCredentials, error) {
	var wrapper struct {
		Installed *ClientCredentials `json:"installed"`
		Web       *ClientCredentials `json:"web"`
		ID        string             `json:"client_id"`
		Secret    string             `json:"client_secret"`
	}

	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parsing client secrets JSON: %w", err)
	}

	if wrapper.Installed != nil && wrapper.Installed.ClientID != "" {
		return wrapper.Installed, nil
	}
	if wrapper.Web != nil && wrapper.Web.ClientID != "" {
		return wrapper.Web, nil
	}
	if wrapper.ID != "" && wrapper.Secret != "" {
		return &ClientCredentials{ClientID: wrapper.ID, ClientSecret: wrapper.Secret}, nil
	}

	return nil, fmt.Errorf("no client_id and client_secret found in secrets file")
}

// startLoopbackListener finds an available loopback port on 127.0.0.1.
func startLoopbackListener() (net.Listener, int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		l.Close()
		return nil, 0, fmt.Errorf("failed to get TCP address")
	}
	return l, tcpAddr.Port, nil
}

// runLoopbackFlow manages opening the browser and waiting for the OAuth callback.
func runLoopbackFlow(ctx context.Context, listener net.Listener, conf *oauth2.Config) (*oauth2.Token, error) {
	state := fmt.Sprintf("talk_cut_%d", time.Now().UnixNano())
	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))

	fmt.Println("Opening browser for YouTube authorization...")
	fmt.Printf("If the browser does not open automatically, visit:\n%s\n\n", authURL)
	_ = openBrowser(authURL)

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			errChan <- fmt.Errorf("state mismatch")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing code parameter", http.StatusBadRequest)
			errChan <- fmt.Errorf("missing code in callback")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, "<h2>talk_cut: Authorization Successful!</h2><p>You can close this window and return to your terminal.</p>")
		codeChan <- code
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Shutdown(ctx)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errChan:
		return nil, err
	case code := <-codeChan:
		return conf.Exchange(ctx, code)
	}
}

// openBrowser attempts to open a URL in the desktop browser.
func openBrowser(url string) error {
	cmd := exec.Command("xdg-open", url)
	return cmd.Start()
}

// SaveToken persists an OAuth2 token to disk with restricted permissions (0600).
func SaveToken(path string, token *oauth2.Token) error {
	resolved, err := config.ResolvePath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(resolved)
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("creating token dir %q: %w", dir, mkdirErr)
	}

	data, marshalErr := json.MarshalIndent(token, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling token: %w", marshalErr)
	}

	if writeErr := os.WriteFile(resolved, data, 0o600); writeErr != nil {
		return fmt.Errorf("writing token file %q: %w", resolved, writeErr)
	}

	return nil
}

// LoadToken reads an OAuth2 token from disk.
func LoadToken(path string) (*oauth2.Token, error) {
	resolved, err := config.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("reading token file %q: %w", resolved, err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("unmarshaling token: %w", err)
	}

	return &token, nil
}

// GetAuthenticatedClient returns an authorized http.Client with automatic token refresh.
func GetAuthenticatedClient(ctx context.Context, cfg config.Config) (*http.Client, error) {
	creds, err := LoadClientCredentials(cfg.YouTubeSecrets)
	if err != nil {
		return nil, fmt.Errorf("loading client secrets: %w", err)
	}

	token, err := LoadToken(cfg.YouTubeTokenFile)
	if err != nil {
		return nil, fmt.Errorf("loading token from %q: %w (run 'talk_cut auth' to authorize)", cfg.YouTubeTokenFile, err)
	}

	conf := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     googleAuthEndpoint,
		Scopes:       []string{youtubeUploadScope, youtubeForceScope},
	}

	return conf.Client(ctx, token), nil
}
