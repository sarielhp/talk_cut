package ctxsleep

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSleep(t *testing.T) {
	if err := Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("Sleep returned %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep on cancelled ctx returned %v", err)
	}
}
