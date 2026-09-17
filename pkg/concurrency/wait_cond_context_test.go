package concurrency

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestWaitCondContext_AlreadyReady(t *testing.T) {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	err := WaitCondContext(context.Background(), cond, &mu, func() bool { return true })
	if err != nil {
		t.Fatalf("WaitCondContext: %v", err)
	}
}

func TestWaitCondContext_CompletesWhenPredicateTrueAfterBroadcast(t *testing.T) {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	var n int
	ctx := context.Background()

	goroutinelabels.NewGoroutine("concurrency_test", "predicate trigger").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		n = 1
		cond.Broadcast()
		mu.Unlock()
	})

	err := WaitCondContext(ctx, cond, &mu, func() bool {
		return n >= 1
	})
	if err != nil {
		t.Fatalf("WaitCondContext: %v", err)
	}
}

func TestWaitCondContext_CancelUnblocks(t *testing.T) {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	ctx, cancel := context.WithCancel(context.Background())
	goroutinelabels.NewGoroutine("concurrency_test", "cancel unblock").StartSimple(func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	})
	err := WaitCondContext(ctx, cond, &mu, func() bool { return false })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v want %v", err, context.Canceled)
	}
}
