package testpackageconcurrency

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLimits_ReadAndMerge(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	bundlesDir := filepath.Join(tmp, paths.ProjectDataDir, paths.TestBundlesDir)
	if err := fileutil.MkdirAll(bundlesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Missing file returns empty/nil
	limits, err := ReadLimitsMap(tmp)
	if err != nil {
		t.Fatalf("unexpected error on missing file: %v", err)
	}
	if limits != nil {
		t.Fatalf("expected nil for missing limits, got: %+v", limits)
	}

	// 2. Write valid limits
	fileData := limitsFile{
		SchemaVersion: 1,
		UpdatedAt:     "2026-08-21T08:00:00Z",
		MaxConcurrentJobsByPackage: map[string]int{
			"./pkg/storage":   4,
			"pkg/mcp":         8,
			"./pkg/scheduler": 2,
			"pkg/unbounded":   64, // should be capped at 32
		},
	}
	raw, err := json.Marshal(fileData)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(bundlesDir, LimitsFileName), raw, 0600); err != nil {
		t.Fatal(err)
	}

	read, err := ReadLimitsMap(tmp)
	if err != nil {
		t.Fatalf("ReadLimitsMap: %v", err)
	}
	if read["pkg/storage"] != 4 {
		t.Fatalf("expected pkg/storage=4, got %d", read["pkg/storage"])
	}
	if read["pkg/mcp"] != 8 {
		t.Fatalf("expected pkg/mcp=8, got %d", read["pkg/mcp"])
	}
	if read["pkg/unbounded"] != 32 {
		t.Fatalf("expected pkg/unbounded capped at 32, got %d", read["pkg/unbounded"])
	}

	// 3. Merge limit maps (take minimum where both present)
	override := map[string]int{
		"pkg/storage": 2,  // lower than 4 -> 2 wins
		"pkg/mcp":     12, // higher than 8 -> 8 wins
		"pkg/cli":     6,  // only in override -> 6
	}
	merged := MergeLimitMaps(read, override)
	if merged["pkg/storage"] != 2 {
		t.Fatalf("expected pkg/storage=2, got %d", merged["pkg/storage"])
	}
	if merged["pkg/mcp"] != 8 {
		t.Fatalf("expected pkg/mcp=8, got %d", merged["pkg/mcp"])
	}
	if merged["pkg/cli"] != 6 {
		t.Fatalf("expected pkg/cli=6, got %d", merged["pkg/cli"])
	}
}
