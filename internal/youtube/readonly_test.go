package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/readonly"
)

// The lock is read once at start-up and cannot be turned on from inside a process,
// so TestReadOnlyLock runs TestReadOnlyLockChild in a child process with it on.
func TestReadOnlyLock(t *testing.T) {
	if readonly.Enabled() {
		t.Skip("already the child")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestReadOnlyLockChild$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(), readonly.EnvVar+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: TestReadOnlyLockChild") {
		t.Fatalf("child failed (err=%v):\n%s", err, out)
	}
}

// apiRecorder answers every request with 200 and an empty JSON object, counting the writes it receives.
type apiRecorder struct {
	writes atomic.Int32
	reads  atomic.Int32
}

func (a *apiRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		a.reads.Add(1)
	} else {
		a.writes.Add(1)
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, `{"items":[]}`)
}

func pointAllEndpointsAt(t *testing.T, base string) {
	for _, v := range []*string{
		&videosEndpoint, &captionsEndpoint, &videoStatusEndpoint, &videoUpdateEndpoint, &channelsEndpoint,
		&playlistItemsEndpoint, &playlistsListEndpoint, &playlistsCreateEndpoint, &playlistItemsInsertEndpoint,
		&playlistItemsRemoveEndpoint, &playlistItemsDetailEndpoint, &playlistItemsUpdateEndpoint,
	} {
		override(t, v, base+"/api?x=1")
	}
}

func TestReadOnlyLockChild(t *testing.T) {
	if !readonly.Enabled() {
		t.Skip("runs only in the child started by TestReadOnlyLock")
	}
	api := &apiRecorder{}
	ts := httptest.NewServer(api)
	defer ts.Close()
	pointAllEndpointsAt(t, ts.URL)
	ctx := context.Background()

	// Every write entry point refuses, even with a client that does not go through the lock.
	raw := ts.Client()
	writes := map[string]error{
		"UploadVideo":             second(UploadVideo(ctx, raw, UploadOptions{VideoPath: "/nonexistent.mp4"})),
		"UploadCaption":           UploadCaption(ctx, raw, "vid", "/nonexistent.vtt", "en", "English"),
		"UpdateVideoMetadata":     second(UpdateVideoMetadata(ctx, raw, "vid", model.TalkMetadata{Title: "t"})),
		"CreatePlaylist":          second(CreatePlaylist(ctx, raw, "title", "", "public")),
		"AddVideoToPlaylist":      AddVideoToPlaylist(ctx, raw, "PL", "vid"),
		"RemoveVideoFromPlaylist": RemoveVideoFromPlaylist(ctx, raw, "PL", "vid"),
		"deletePlaylistItem":      deletePlaylistItem(ctx, raw, "item"),
		"movePlaylistItem":        movePlaylistItem(ctx, raw, "PL", PlaylistMove{Item: PlaylistItem{ItemID: "i", VideoID: "v"}}),
		"insertPlaylistItem":      second(insertPlaylistItem(ctx, raw, "PL", "vid")),
		"TidyPlaylist":            second(TidyPlaylist(ctx, raw, "PL", false, nil)),
		"Authorize":               Authorize(ctx, config.Config{}, "x", "", ""),
	}
	for name, err := range writes {
		if !errors.Is(err, readonly.ErrReadOnly) {
			t.Errorf("%s: err = %v, want ErrReadOnly", name, err)
		}
	}

	// Reads still work, and the dry run of a tidy is a read.
	if _, err := TidyPlaylist(ctx, raw, "PL", true, nil); err != nil {
		t.Errorf("dry-run tidy: %v", err)
	}
	if _, err := ListPlaylists(ctx, raw); err != nil {
		t.Errorf("ListPlaylists: %v", err)
	}

	checkLocalWritesRefused(t)
	checkRefreshStillWorks(t, ts.URL, api)

	if n := api.writes.Load(); n != 0 {
		t.Errorf("%d write request(s) reached the API", n)
	}
	if api.reads.Load() == 0 {
		t.Error("no reads reached the API; the test did not exercise it")
	}
}

func second[T any](_ T, err error) error { return err }

func checkLocalWritesRefused(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := config.SaveConfig(config.Config{DefaultChannel: "x"}); !errors.Is(err, readonly.ErrReadOnly) {
		t.Errorf("config.SaveConfig: %v", err)
	}
	if err := (config.Config{DefaultChannel: "x"}).SaveConfig(); !errors.Is(err, readonly.ErrReadOnly) {
		t.Errorf("Config.SaveConfig: %v", err)
	}
	tokenPath := filepath.Join(home, "token.json")
	if err := SaveToken(tokenPath, &oauth2.Token{AccessToken: "a"}); !errors.Is(err, readonly.ErrReadOnly) {
		t.Errorf("SaveToken: %v", err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Errorf("files were written under HOME: %v", entries)
	}
}

// checkRefreshStillWorks proves the lock sits outside the OAuth transport: an expired
// token is refreshed (a POST to the token endpoint) and the read then goes through.
func checkRefreshStillWorks(t *testing.T, apiURL string, api *apiRecorder) {
	var refreshed atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		refreshed.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"access_token":"fresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()
	override(t, &googleAuthEndpoint, oauth2.Endpoint{TokenURL: tokenSrv.URL})

	dir := t.TempDir()
	secrets := filepath.Join(dir, "secrets.json")
	token := filepath.Join(dir, "token.json")
	_ = os.WriteFile(secrets, []byte(`{"installed":{"client_id":"id","client_secret":"s"}}`), 0o600)
	expired := fmt.Sprintf(`{"access_token":"old","refresh_token":"r","token_type":"Bearer","expiry":%q}`,
		time.Now().Add(-time.Hour).Format(time.RFC3339))
	_ = os.WriteFile(token, []byte(expired), 0o600)

	cfg := config.Config{YouTubeSecrets: secrets, Channels: map[string]string{"c": token}}
	client, err := GetAuthenticatedClient(context.Background(), cfg, "c")
	if err != nil {
		t.Fatalf("GetAuthenticatedClient: %v", err)
	}
	before := api.reads.Load()
	if _, err := ListPlaylists(context.Background(), client); err != nil {
		t.Fatalf("read after refresh: %v", err)
	}
	if refreshed.Load() != 1 || api.reads.Load() != before+1 {
		t.Errorf("refreshes=%d reads=%d→%d", refreshed.Load(), before, api.reads.Load())
	}
	req, _ := http.NewRequest(http.MethodPost, apiURL+"/api", strings.NewReader("{}"))
	if _, err := client.Do(req); !errors.Is(err, readonly.ErrReadOnly) {
		t.Errorf("POST through the authenticated client: %v", err)
	}
}
