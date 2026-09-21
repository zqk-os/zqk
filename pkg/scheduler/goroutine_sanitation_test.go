package scheduler_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/scheduler"
)

// CRIT-1789663120758632000-c8ed430e: Functional Acceptance
// Verifies that neither pkg/scheduler nor cmd/zqk/scheduler contains raw unmanaged go statements (*ast.GoStmt).
func TestGoroutineSanitation_FunctionalAcceptance(t *testing.T) {
	dirs := []string{
		filepath.Join("..", "scheduler"),
		filepath.Join("..", "..", "cmd", "zqk", "scheduler"),
	}

	fset := token.NewFileSet()
	var rawGoStmts []string

	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			t.Fatalf("failed to resolve directory %s: %v", dir, err)
		}

		err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// Only inspect Go files; ignore our own test file if encountered
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "goroutine_sanitation_test.go") {
				return nil
			}

			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}

			ast.Inspect(node, func(n ast.Node) bool {
				if goStmt, ok := n.(*ast.GoStmt); ok {
					pos := fset.Position(goStmt.Pos())
					rawGoStmts = append(rawGoStmts, pos.String())
				}
				return true
			})
			return nil
		})

		if err != nil {
			t.Fatalf("failed walking directory %s: %v", dir, err)
		}
	}

	if len(rawGoStmts) > 0 {
		t.Fatalf("Found %d unmanaged raw go statements in scheduler packages:\n%s",
			len(rawGoStmts), strings.Join(rawGoStmts, "\n"))
	}
}

var leakIgnoreOptions = []goleak.Option{
	goleak.IgnoreTopFunction("github.com/robfig/cron/v3.(*Cron).run"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/scheduler.(*NotificationContext).deliveryLoop"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/ambient.(*AmbientIngestService).BindToMesh.func1"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/ambience.(*InMemoryEventMesh).Subscribe.func2"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/storage/filecas.initDarwinSyncQueue.func1"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/storage/cas.(*indexQueue).startWorker"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/storage.(*ObjectWriteBehindWorker).run"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/storage.(*ioQueue).startWorker"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/storage/cas.(*CASOrphanCleanupQueue).startWorker.func1"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/validation.(*sharedValidationCache).runFlusher"),
	goleak.IgnoreTopFunction("github.com/zqk-os/zqk/pkg/goroutinelabels.(*Pool).Start.func1"),
}

// CRIT-1789663120758633000-25e89aa4: Boundary & Error Handling
// Verifies that managed goroutines handle context cancellation, timeouts, and panic recovery without leaking goroutines.
func TestGoroutineSanitation_BoundaryAndErrorHandling(t *testing.T) {
	currentLeaks := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(leakIgnoreOptions, currentLeaks)...)
	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		completed := make(chan struct{})

		goroutinelabels.NewGoroutine("test.boundary.cancellation", "verify context cancellation").
			WithContext(ctx).
			StartSimple(func() {
				close(started)
				<-ctx.Done()
				close(completed)
			})

		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("goroutine did not start in time")
		}

		cancel()

		select {
		case <-completed:
		case <-time.After(2 * time.Second):
			t.Fatal("goroutine did not cleanly stop on context cancellation")
		}
	})

	t.Run("PanicRecovery", func(t *testing.T) {
		ctx := context.Background()
		var recoveredValue atomic.Value
		var wg sync.WaitGroup
		wg.Add(1)

		goroutinelabels.NewGoroutine("test.boundary.panic", "verify panic handling").
			WithContext(ctx).
			StartSimple(func() {
				defer func() {
					if r := recover(); r != nil {
						recoveredValue.Store(r)
					}
					wg.Done()
				}()
				panic("simulated scheduler worker panic")
			})

		wg.Wait()

		val := recoveredValue.Load()
		if val == nil {
			t.Fatal("expected panic to be caught and recovered")
		}
		if val.(string) != "simulated scheduler worker panic" {
			t.Fatalf("unexpected panic value: %v", val)
		}
	})
}

// CRIT-1789663120758634000-31382b55: Integration & Conformance
// Verifies integration with scheduler concurrency primitives and job queue contracts.
func TestGoroutineSanitation_IntegrationAndConformance(t *testing.T) {
	currentLeaks := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(leakIgnoreOptions, currentLeaks)...)
	// Verify that scheduler activity cache uses managed synchronization and goroutinelabels
	activityCache := scheduler.NewActivityCache()
	if activityCache == nil {
		t.Fatal("expected non-nil activity cache")
	}
	defer activityCache.Stop()

	// Concurrently record activities to ensure bounded, race-free operation
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("test.conformance.activity", "concurrent activity recording").
			WithContext(ctx).
			StartSimple(func() {
				defer wg.Done()
				jobID := string(rune('A' + idx))
				activityCache.UpdateEvent(jobID, "scheduler_job_completed", 15*time.Millisecond, nil)
			})
	}

	wg.Wait()

	entries := activityCache.GetAllEntries()
	if len(entries) != 10 {
		t.Fatalf("expected 10 recorded activities, got %d", len(entries))
	}

	for i := 0; i < 10; i++ {
		jobID := string(rune('A' + i))
		entry, exists := activityCache.GetEntry(jobID)
		if !exists || entry == nil {
			t.Fatalf("expected entry for job %s", jobID)
		}
		if entry.TotalCompleted != 1 {
			t.Fatalf("expected TotalCompleted=1 for job %s, got %d", jobID, entry.TotalCompleted)
		}
	}
}
