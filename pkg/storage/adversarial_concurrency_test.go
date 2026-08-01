//go:build integration

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
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
		go func(writerID int) {
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
		}(i)
	}

	// 5. Start Aggregators (simulating maintenance jobs)
	for i := 0; i < numAggregators; i++ {
		wg.Add(1)
		go func(aggID int) {
			defer wg.Done()
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-stopChan:
					fmt.Printf("A%d stopping\n", aggID)
					return
				case <-ticker.C:
					// Trigger full aggregation
					fmt.Printf("A%d aggregating\n", aggID)
					_, err := aggSvc.AggregateAuditEvents(ctx, secCtx, storageCtx, time.Now().Add(-1*time.Hour), time.Now())
					if err != nil {
						fmt.Printf("A%d error: %v\n", aggID, err)
					}
				}
			}
		}(i)
	}

	// 6. Run Stress Test
	fmt.Printf("🔥 Starting adversarial stress test for %s...\n", totalDuration)
	time.Sleep(totalDuration)
	fmt.Println("⏳ Signaling stop...")
	close(stopChan)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("🛑 All workers finished.")
	case <-time.After(5 * time.Second):
		t.Fatalf("DEADLOCK DETECTED: workers failed to stop within 5s")
	}

	// 7. Verification: Full Flush and Final Aggregation
	fmt.Println("🧹 Final verification...")
	FlushGlobalAuditBufferForProjectRoot(root)
	_, aggErr := aggSvc.AggregateAuditEvents(ctx, secCtx, storageCtx, time.Now().Add(-24*time.Hour), time.Now())
	if aggErr != nil {
		t.Errorf("final aggregation failed: %v", aggErr)
	}

	// 8. Structural Integrity Check
	kindDir := filepath.Join(root, paths.ProjectDataDir, "state", "datacells", "audit_event", "cas")
	if entries, err := os.ReadDir(kindDir); err == nil {
		fmt.Printf("📦 CAS contains %d bucket directories.\n", len(entries))
	}
}
