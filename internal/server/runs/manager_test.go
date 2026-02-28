package runs

import (
	"context"
	"testing"
	"time"
)

func TestManagerStartAndComplete(t *testing.T) {
	m := NewManager()
	info := m.Start(context.Background(), "sess-1", "hello", "build", func(context.Context) error {
		return nil
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		curr, ok := m.Get(info.ID)
		if !ok {
			t.Fatalf("run not found: %s", info.ID)
		}
		if curr.Status == StatusCompleted {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run did not complete in time")
}

func TestManagerAbort(t *testing.T) {
	m := NewManager()
	info := m.Start(context.Background(), "sess-2", "work", "build", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	if _, err := m.Abort(info.ID); err != nil {
		t.Fatalf("abort run: %v", err)
	}

	curr, ok := m.Get(info.ID)
	if !ok {
		t.Fatalf("run not found after abort")
	}
	if curr.Status != StatusAborted {
		t.Fatalf("expected aborted status, got %s", curr.Status)
	}
}
