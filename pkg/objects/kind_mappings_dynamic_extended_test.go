package objects

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDynamicKindMapper_Extended(t *testing.T) {
	ResetGlobalKindMapperForTesting()
	mapper := GetGlobalKindMapper()
	if mapper == nil {
		t.Fatal("expected non-nil mapper")
	}

	tmpDir := t.TempDir()
	procDir := filepath.Join(tmpDir, "process")
	specsDir := filepath.Join(tmpDir, "specs")
	if err := fileutil.MkdirAll(procDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	mapper.SetDirectories(procDir, specsDir)
	gotProc, gotSpecs := mapper.GetDirectories()
	if gotProc != procDir || gotSpecs != specsDir {
		t.Errorf("GetDirectories: got (%s, %s), want (%s, %s)", gotProc, gotSpecs, procDir, specsDir)
	}

	ctx := context.Background()
	if err := mapper.EnsureReady(ctx); err != nil {
		t.Fatalf("EnsureReady failed: %v", err)
	}

	// Reload
	if err := mapper.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// GetAllKinds
	kinds := mapper.GetAllKinds()
	if kinds == nil {
		t.Fatal("expected non-nil kinds slice")
	}

	// inferDirectoryName branch testing (with config nil to test pure algorithmic fallback)
	mapper.config = nil
	d1 := mapper.inferDirectoryName("ticket_item", "")
	if d1 != "ticket" {
		t.Errorf("inferDirectoryName(ticket_item) = %s, want ticket", d1)
	}
	d2 := mapper.inferDirectoryName("performance_metric", "")
	if d2 != "metrics" {
		t.Errorf("inferDirectoryName(performance_metric) = %s, want metrics", d2)
	}
	d3 := mapper.inferDirectoryName("policy", "")
	if d3 != "policies" {
		t.Errorf("inferDirectoryName(policy) = %s, want policies", d3)
	}
	d4 := mapper.inferDirectoryName("day", "") // excluded from 'ies'
	if d4 != "days" {
		t.Errorf("inferDirectoryName(day) = %s, want days", d4)
	}
	d5 := mapper.inferDirectoryName("task", "")
	if d5 != "tasks" {
		t.Errorf("inferDirectoryName(task) = %s, want tasks", d5)
	}
	d6 := mapper.inferDirectoryName("metrics", "")
	if d6 != "metrics" {
		t.Errorf("inferDirectoryName(metrics) = %s, want metrics", d6)
	}

	ResetGlobalKindMapperForTesting()
}
