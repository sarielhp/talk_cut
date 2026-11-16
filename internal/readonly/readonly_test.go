package readonly

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		v    string
		want bool
	}{
		{"1", true}, {"true", true}, {"YES", true}, {"\ton\n", true},
		{"", false}, {"0", false}, {"false", false}, {"no", false}, {"off", false}, {"maybe", false},
	} {
		if got := parse(tt.v); got != tt.want {
			t.Errorf("parse(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

// withLock turns the lock on for one test; only this package's tests can do that.
func withLock(t *testing.T, on bool) {
	orig := enabled
	enabled = on
	t.Cleanup(func() { enabled = orig })
}

func TestCheck(t *testing.T) {
	withLock(t, false)
	if err := Check("x"); err != nil {
		t.Fatalf("lock off: %v", err)
	}
	withLock(t, true)
	if err := Check("SaveConfig"); !errors.Is(err, ErrReadOnly) || !strings.Contains(err.Error(), "SaveConfig") {
		t.Fatalf("lock on: %v", err)
	}
}

func TestGuardPassesOnlyReads(t *testing.T) {
	var writes atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writes.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	withLock(t, false)
	if Guard(ts.Client()) != ts.Client() {
		t.Error("lock off: Guard changed the client")
	}

	withLock(t, true)
	client := Guard(ts.Client())
	for _, m := range []string{http.MethodGet, http.MethodHead, ""} {
		req, _ := http.NewRequest(m, ts.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%q refused: %v", m, err)
		}
		resp.Body.Close()
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, "PURGE"} {
		req, _ := http.NewRequest(m, ts.URL, strings.NewReader("body"))
		if _, err := client.Do(req); !errors.Is(err, ErrReadOnly) {
			t.Errorf("%s: err = %v, want ErrReadOnly", m, err)
		}
	}
	if n := writes.Load(); n != 0 {
		t.Errorf("%d write request(s) reached the server", n)
	}
}
