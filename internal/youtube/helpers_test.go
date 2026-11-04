package youtube

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// override sets the package variable *v to value until the test finishes.
func override[T any](t *testing.T, v *T, value T) {
	t.Helper()
	orig := *v
	*v = value
	t.Cleanup(func() { *v = orig })
}

// jsonServer answers requests that use method with 200 and body, and any
// other method with 405; it is closed when the test finishes.
func jsonServer(t *testing.T, method, body string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "expected "+method, http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, body)
	}))
	t.Cleanup(ts.Close)
	return ts
}
