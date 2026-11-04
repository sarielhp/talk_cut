// Package ctxsleep provides a context-aware sleep.
package ctxsleep

import (
	"context"
	"time"
)

// Sleep waits for d, returning ctx.Err() early if ctx is done first.
func Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
