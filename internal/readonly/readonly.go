// Package readonly is talk_cut's hard read-only lock.
//
// When TALK_CUT_READ_ONLY is set to a true value in the environment, the lock is on
// for the whole life of the process: it is read once at start-up and nothing can
// turn it off. While it is on, every remote API client that goes through Guard
// refuses any request other than GET or HEAD, and the write entry points of the
// YouTube, config and token code refuse with ErrReadOnly before doing anything.
// Files of the recording being edited (cuts, metadata, renders) are not locked.
package readonly

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// EnvVar turns the lock on: "1", "true", "yes" or "on" (any case).
const EnvVar = "TALK_CUT_READ_ONLY"

// ErrReadOnly is returned for every write refused by the lock.
var ErrReadOnly = errors.New("read-only mode (" + EnvVar + "): writes are disabled")

var enabled = parse(os.Getenv(EnvVar))

func parse(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Enabled reports whether the lock is on.
func Enabled() bool {
	return enabled
}

// Check returns an ErrReadOnly naming what was refused when the lock is on, and nil otherwise.
func Check(what string) error {
	if !enabled {
		return nil
	}
	return fmt.Errorf("%s refused: %w", what, ErrReadOnly)
}

// Guard returns client unchanged when the lock is off; when it is on, it returns a
// client that sends only GET and HEAD requests and fails every other one with
// ErrReadOnly before anything leaves the process. It wraps the client's whole
// transport, so an OAuth token refresh made inside that transport still works.
func Guard(client *http.Client) *http.Client {
	if !enabled || client == nil {
		return client
	}
	guarded := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	guarded.Transport = transport{base: base}
	return &guarded
}

type transport struct {
	base http.RoundTripper
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == "" || req.Method == http.MethodGet || req.Method == http.MethodHead {
		return t.base.RoundTrip(req)
	}
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%s %s refused: %w", req.Method, req.URL.Redacted(), ErrReadOnly)
}

// Banner is the notice printed at start-up while the lock is on.
const Banner = "talk_cut: READ-ONLY mode (" + EnvVar + "): no YouTube, AI or config writes"

// Tag is the marker the TUI shows in its title while the lock is on.
const Tag = " [READ-ONLY]"
