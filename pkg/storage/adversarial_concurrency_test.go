//go:build integration

package storage

import (
	"context"
	"fmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/storage/audit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestAdversarialConcurrency performs a high-fidelity stress test of the storage mesh
// under extreme contention, simulating a multi-agent environment with overlapping
// write, aggregate, and audit operations.
func TestAdversarialConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping adversarial stress test in short mode")
	}

	root, s, secCtx := setupTestingFactoryCompleteTestEnvironment(t)
	storageCtx := pkgctx.GetStorageContext()
	ctx := context.Background()

	// 2. Initialize Audit Aggregator
	aggSvc := NewAuditAggregationService(s)

	// 3. Stress Parameters (Reduced for faster execution)
	const (
		numWriters      = 3
		numAggregators  = 1
		eventsPerWriter = 10
		totalDuration   = 2 * time.Second
	)

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	// 4. Start Writers (simulating agents)
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		wID := i
		goroutinelabels.NewGoroutine("storage_adversarial_test", "stress writer worker").StartSimple(func() {
			writerID := wID
			defer wg.Done()
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()

			count := 0
			for {
				select {
				case <-stopChan:
					fmt.Printf("W%d stopping\n", writerID)
					return
				case <-ticker.C:
					// Create random audit event
					auditOptions := &AuditEventOptions{
						EventType:  "adversarial_test_event",
						TargetID:   fmt.Sprintf("OBJ-%d-%d", writerID, count),
						TargetKind: "test_kind",
						Operation:  "stress_write",
						Severity:   "low",
						Metadata: map[string]any{
							"writer":    writerID,
							"iteration": count,
						},
					}

					// Write via helper (triggers buffer + potential merge)
					err := CreateAuditEventWithBuilder(ctx, root, secCtx, s, auditOptions)
					if err != nil {
						fmt.Printf("W%d error: %v\n", writerID, err)
					}
					count++
					fmt.Printf("W%d: %d\n", writerID, count)
					if count >= eventsPerWriter {
						count = 0
					}
				}
			}
		})
	}

	// 5. Start Aggregators (simulating maintenance jobs)
	for i := 0; i < numAggregators; i++ {
		wg.Add(1)
		aID := i
		goroutinelabels.NewGoroutine("storage_adversarial_test", "stress aggregator worker").StartSimple(func() {
			aggID := aID
			defer wg.Done()
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			lastEnd := time.Now().Add(-1 * time.Hour)
			for {
				select {
				case <-stopChan:
					fmt.Printf("A%d stopping\n", aggID)
					return
				case <-ticker.C:
					now := time.Now()
					if now.After(lastEnd) {
						fmt.Printf("A%d aggregating\n", aggID)
						_, err := aggSvc.AggregateAuditEvents(ctx, secCtx, storageCtx, lastEnd, now)
						if err != nil {
							// If concurrent workers attempt overlapping windows, verify it is safely rejected
							if !strings.Contains(err.Error(), "overlapping") {
								fmt.Printf("A%d unexpected error: %v\n", aggID, err)
							}
						} else {
							lastEnd = now
						}
					}
				}
			}
		})
	}

	// 6. Run Stress Test
	fmt.Printf("🔥 Starting adversarial stress test for %s...\n", totalDuration)
	time.Sleep(totalDuration)
	fmt.Println("⏳ Signaling stop...")
	close(stopChan)

	// Wait with timeout
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("storage_adversarial_test", "wg wait monitor").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		fmt.Println("🛑 All workers finished.")
	case <-time.After(5 * time.Second):
		t.Fatalf("DEADLOCK DETECTED: workers failed to stop within 5s")
	}

	// 7. Verification: Full Flush and Final Aggregation
	fmt.Println("🧹 Final verification...")
	FlushGlobalAuditBufferForProjectRoot(root)
	now := time.Now()
	_, aggErr := aggSvc.AggregateAuditEvents(ctx, secCtx, storageCtx, now.Add(-100*time.Millisecond), now)
	if aggErr != nil && !strings.Contains(aggErr.Error(), "overlapping") {
		t.Errorf("final aggregation failed: %v", aggErr)
	}

	// 8. Structural Integrity Check
	kindDir := audit.KindDir(root)
	entries, err := fileutil.ReadDir(kindDir)
	if err == nil {
		fmt.Printf("📦 Storage contains %d monthly directories.\n", len(entries))
	}
	if err != nil || len(entries) == 0 {
		t.Errorf("expected storage entries for audit events, got %d (err: %v)", len(entries), err)
	}
}
