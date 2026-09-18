// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"talk_cut/internal/config"
)

// AuthorizerFunc represents the function signature for performing OAuth authorization.
type AuthorizerFunc func(ctx context.Context, cfg config.Config, channel, secretsPath, tokenPath string) error

// SetupOptions specifies configuration options for the guided setup flow.
type SetupOptions struct {
	Channel        string
	SecretsPath    string
	TokenPath      string
	NonInteractive bool
}

// RunGuidedSetup leads the user through an interactive CLI setup wizard for YouTube authorization.
func RunGuidedSetup(ctx context.Context, in io.Reader, out io.Writer, cfg config.Config, opts SetupOptions, authorizer AuthorizerFunc) error {
	reader := bufio.NewReader(in)

	fmt.Fprintln(out, "\n=== YouTube Guided Setup ===")
	fmt.Fprintln(out, "This wizard configures YouTube API credentials and authorization.")
	fmt.Fprintln(out, "(Run 'talk_cut youtube setup -H' anytime for the full step-by-step manual)")

	secPath, err := resolveSecretsStep(reader, out, &cfg, opts)
	if err != nil {
		return err
	}

	channel, tokPath, shouldAuth, err := resolveChannelStep(reader, out, cfg, opts)
	if err != nil {
		return err
	}
	if !shouldAuth {
		fmt.Fprintf(out, "\nSetup complete: keeping existing authorization for channel %q.\n", channel)
		return nil
	}

	return executeAuthStep(ctx, reader, out, cfg, channel, secPath, tokPath, opts, authorizer)
}

// promptInput prints a prompt and reads a trimmed line of text from the reader.
func promptInput(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// resolveSecretsStep discovers, prompts for, and verifies Google OAuth client credentials.
func resolveSecretsStep(reader *bufio.Reader, out io.Writer, cfg *config.Config, opts SetupOptions) (string, error) {
	secPath := cfg.YouTubeSecrets
	if opts.SecretsPath != "" {
		secPath = opts.SecretsPath
	}

	if creds, err := LoadClientCredentials(secPath); err == nil && creds.ClientID != "" {
		fmt.Fprintf(out, "\n[Step 1/3] Client Secrets: Found valid credentials (%s)\n", secPath)
		return secPath, nil
	}

	if opts.NonInteractive {
		return "", fmt.Errorf("client secrets file %q not found or invalid", secPath)
	}

	return promptForSecrets(reader, out, cfg, secPath)
}

// promptForSecrets displays instructions and guides the user to supply client secrets.
func promptForSecrets(reader *bufio.Reader, out io.Writer, cfg *config.Config, defaultPath string) (string, error) {
	fmt.Fprintln(out, "\n[Step 1/3] Google OAuth Client Secrets")
	fmt.Fprintf(out, "No valid client secrets file found at: %s\n\n", defaultPath)
	fmt.Fprintln(out, "To create your client secrets:")
	fmt.Fprintln(out, "  1. Open Google Cloud Console: https://console.cloud.google.com/")
	fmt.Fprintln(out, "  2. Enable 'YouTube Data API v3' (or: gcloud services enable youtube.googleapis.com)")
	fmt.Fprintln(out, "  3. Configure OAuth consent screen (add your email to Test Users)")
	fmt.Fprintln(out, "  4. Create Credentials -> OAuth client ID -> Desktop app")
	fmt.Fprintln(out, "  5. Download the JSON credentials file")
	fmt.Fprintln(out, "  (Tip: Run 'talk_cut youtube setup -H' for detailed instructions)")

	input, err := promptInput(reader, out, "\nEnter path to downloaded JSON file (or press Enter if placed at default path): ")
	if err != nil {
		return "", err
	}

	targetPath := defaultPath
	if input != "" {
		targetPath = input
	}

	creds, err := LoadClientCredentials(targetPath)
	if err != nil {
		return "", fmt.Errorf("validating client secrets at %q: %w", targetPath, err)
	}

	resolvedTarget, _ := config.ResolvePath(targetPath)
	resolvedDefault, _ := config.ResolvePath(defaultPath)
	if resolvedTarget != resolvedDefault {
		handleCopySecrets(reader, out, resolvedTarget, resolvedDefault)
	}

	fmt.Fprintf(out, "✔ Client credentials validated successfully (Client ID: %s...)\n", maskClientID(creds.ClientID))
	return targetPath, nil
}

// handleCopySecrets offers to copy custom client secrets to the standard default path.
func handleCopySecrets(reader *bufio.Reader, out io.Writer, src, dst string) {
	ans, err := promptInput(reader, out, fmt.Sprintf("Copy credentials to standard path %s? [Y/n]: ", dst))
	if err != nil || strings.HasPrefix(strings.ToLower(ans), "n") {
		return
	}

	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(out, "warning: could not read %s: %v\n", src, err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		fmt.Fprintf(out, "warning: creating directory for %s: %v\n", dst, err)
		return
	}

	if err := os.WriteFile(dst, data, 0600); err != nil {
		fmt.Fprintf(out, "warning: writing %s: %v\n", dst, err)
		return
	}

	fmt.Fprintf(out, "✔ Copied client secrets to %s\n", dst)
}

// resolveChannelStep determines the target channel and checks for existing tokens.
func resolveChannelStep(reader *bufio.Reader, out io.Writer, cfg config.Config, opts SetupOptions) (string, string, bool, error) {
	channel := opts.Channel
	if channel == "" && !opts.NonInteractive {
		defCh := cfg.DefaultChannel
		if defCh == "" {
			defCh = "default"
		}
		fmt.Fprintf(out, "\n[Step 2/3] Channel Profile\n")
		ans, err := promptInput(reader, out, fmt.Sprintf("Enter channel profile name (e.g. default, seminar, course) [%s]: ", defCh))
		if err != nil {
			return "", "", false, err
		}
		if ans != "" {
			channel = ans
		} else {
			channel = defCh
		}
	}
	if channel == "" {
		channel = "default"
	}

	tokPath := opts.TokenPath
	if tokPath == "" {
		tokPath = cfg.ResolveChannelTokenFile(channel)
	}

	resolvedTok, err := config.ResolvePath(tokPath)
	if err == nil {
		if _, statErr := os.Stat(resolvedTok); statErr == nil && !opts.NonInteractive {
			ans, promptErr := promptInput(reader, out, fmt.Sprintf("Found existing token for channel %q (%s). Re-authorize? [y/N]: ", channel, tokPath))
			if promptErr != nil {
				return "", "", false, promptErr
			}
			if !strings.HasPrefix(strings.ToLower(ans), "y") {
				return channel, tokPath, false, nil
			}
		}
	}

	return channel, tokPath, true, nil
}

// executeAuthStep performs interactive browser authorization and updates configuration.
func executeAuthStep(ctx context.Context, reader *bufio.Reader, out io.Writer, cfg config.Config, channel, secPath, tokPath string, opts SetupOptions, authorizer AuthorizerFunc) error {
	if !opts.NonInteractive {
		fmt.Fprintln(out, "\n[Step 3/3] Google OAuth Authorization")
		fmt.Fprintln(out, "A browser window will open to authorize talk_cut to access your YouTube channel.")
		fmt.Fprintln(out, "Permissions requested: upload videos, manage video captions.")
		ans, err := promptInput(reader, out, "Ready to proceed? [Y/n]: ")
		if err != nil {
			return err
		}
		if strings.HasPrefix(strings.ToLower(ans), "n") {
			fmt.Fprintln(out, "Setup paused. Run 'talk_cut youtube setup' anytime to continue.")
			return nil
		}
	}

	fmt.Fprintln(out, "\nLaunching browser for authentication...")
	if err := authorizer(ctx, cfg, channel, secPath, tokPath); err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	fmt.Fprintln(out, "\n✔ Successfully authorized YouTube channel!")
	fmt.Fprintf(out, "Channel:      %s\n", channel)
	fmt.Fprintf(out, "Token file:   %s\n", tokPath)
	fmt.Fprintln(out, "Config saved: ~/.config/talk_cut/config.json")
	return nil
}

// RunYouTubeStatus inspects and prints the current YouTube credentials and channel configurations.
func RunYouTubeStatus(out io.Writer, cfg config.Config) error {
	fmt.Fprintln(out, "=== YouTube Configuration Status ===")

	secPath := cfg.YouTubeSecrets
	resolvedSec, err := config.ResolvePath(secPath)
	if err == nil {
		if creds, loadErr := LoadClientCredentials(secPath); loadErr == nil {
			fmt.Fprintf(out, "Client Secrets:  ✔ Found (%s, Client ID: %s...)\n", resolvedSec, maskClientID(creds.ClientID))
		} else {
			fmt.Fprintf(out, "Client Secrets:  ✘ Invalid at %s (%v)\n", resolvedSec, loadErr)
		}
	} else {
		fmt.Fprintf(out, "Client Secrets:  ✘ %s (unresolvable)\n", secPath)
	}

	defCh := cfg.DefaultChannel
	if defCh == "" {
		defCh = "default"
	}
	fmt.Fprintf(out, "Default Channel: %s\n", defCh)

	defPriv := cfg.DefaultPrivacy
	if defPriv == "" {
		defPriv = "public"
	}
	fmt.Fprintf(out, "Default Privacy: %s\n", defPriv)

	channels := cfg.Channels
	if len(channels) == 0 {
		channels = map[string]string{defCh: cfg.ResolveChannelTokenFile(defCh)}
	}

	fmt.Fprintln(out, "\nConfigured Channels:")
	for ch, tokPath := range channels {
		resolvedTok, resErr := config.ResolvePath(tokPath)
		if resErr != nil {
			fmt.Fprintf(out, "  • %-12s: ✘ Unresolvable path %s\n", ch, tokPath)
			continue
		}

		tok, tokErr := LoadToken(tokPath)
		if tokErr != nil {
			fmt.Fprintf(out, "  • %-12s: ✘ Token missing or unreadable (%s)\n", ch, resolvedTok)
			continue
		}

		statusStr := "✔ Authorized"
		if tok.RefreshToken == "" {
			statusStr = "⚠ No refresh token"
		}
		fmt.Fprintf(out, "  • %-12s: %s (%s)\n", ch, statusStr, resolvedTok)
	}

	fmt.Fprintln(out, "\nActions:")
	fmt.Fprintln(out, "  Setup / Re-auth: talk_cut youtube setup")
	fmt.Fprintln(out, "  Detailed Guide:  talk_cut youtube setup -H")
	return nil
}

// maskClientID truncates a client ID for display to avoid leaking sensitive prefixes/suffixes.
func maskClientID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "..." + id[len(id)-4:]
}
