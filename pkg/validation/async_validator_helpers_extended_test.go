package validation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestAsyncValidator_HelpersExtended(t *testing.T) {
	tmpDir := t.TempDir()
	av := NewAsyncValidator(context.Background(), tmpDir, 2, 0)

	// GetRunIssues
	issues := av.GetRunIssues()
	if issues.WorkerStopTimedOut || issues.CacheSaveTimedOut {
		t.Errorf("expected clean run issues initially")
	}

	// WaitForValidationCompletion when none running
	if !av.WaitForValidationCompletion(100 * time.Millisecond) {
		t.Errorf("expected WaitForValidationCompletion to return true when none running")
	}

	// checkHashRegistryUpdated
	objDir := filepath.Join(tmpDir, "objects")
	if err := os.MkdirAll(objDir, 0755); err != nil {
		t.Fatalf("failed to create objDir: %v", err)
	}
	objFile := filepath.Join(objDir, "obj1.yaml")
	if err := os.WriteFile(objFile, []byte("id: obj1"), 0644); err != nil {
		t.Fatalf("failed to write objFile: %v", err)
	}

	// No hash registry exists
	pastTime := time.Now().Add(-1 * time.Hour)
	if av.checkHashRegistryUpdated(objects.KindBacklogItem, objFile, pastTime) {
		t.Errorf("expected false when hash registry does not exist")
	}

	// Create hash registry file
	hashFile := filepath.Join(objDir, "."+objects.KindBacklogItem+".hashes")
	if err := os.WriteFile(hashFile, []byte("hash1"), 0644); err != nil {
		t.Fatalf("failed to write hashFile: %v", err)
	}

	// Cache time before hash registry file modification
	if !av.checkHashRegistryUpdated(objects.KindBacklogItem, objFile, pastTime) {
		t.Errorf("expected true when hash registry was updated after cacheTime")
	}

	// Cache time after hash registry file modification
	futureTime := time.Now().Add(1 * time.Hour)
	if av.checkHashRegistryUpdated(objects.KindBacklogItem, objFile, futureTime) {
		t.Errorf("expected false when hash registry was updated before cacheTime")
	}

	// noopAsyncValidatorShutdownMetrics
	var m noopAsyncValidatorShutdownMetrics
	m.RecordStage(context.Background(), "stage", "kind", 10*time.Millisecond, nil)
	m.RecordStageWithBuckets(context.Background(), "stage", "kind", 10*time.Millisecond, nil, nil)

	// sendProgressUpdate
	av.sendProgressUpdate("OBJ-1", "completed")
	select {
	case p := <-av.GetProgress():
		if p.CurrentObject != "OBJ-1" || p.Status != "completed" {
			t.Errorf("unexpected progress: %+v", p)
		}
	default:
		t.Errorf("expected progress on progressChan")
	}

	// InitiateShutdown & Drain
	if err := av.InitiateShutdown(); err != nil {
		t.Errorf("InitiateShutdown failed: %v", err)
	}
	if err := av.Drain(context.Background()); err != nil {
		t.Errorf("Drain failed: %v", err)
	}
}


