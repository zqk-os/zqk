package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cliapp"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestAutoFixMissingHash_BucketedObject tests that --auto-fix correctly resolves missing-hash
// for bucketed objects (e.g. audit_event). Current behavior may fix via CAS migration
// (Migrated and indexed ... to CAS) rather than in-place .kind.hashes; we assert only
// that at least one fix was applied.
func TestAutoFixMissingHash_BucketedObject(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory structure
	testRoot := t.TempDir()
	projectRoot := filepath.Join(testRoot, "project")

	// Create directory structure for bucketed audit_event
	auditDir := filepath.Join(datacell.StreamCurrentKindDir(projectRoot, objects.KindAuditEvent), "2026-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	// Create a test audit_event file (without hash in registry)
	testFile := filepath.Join(auditDir, "AUD-TEST-001.yaml")
	testContent := `id: AUD-TEST-001
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
status: completed
event_type: command_execution
target_id: TEST-001
target_kind: test
created_at: "2026-01-03T15:00:00Z"
created_by: ACC-TEST
updated_at: "2026-01-03T15:00:00Z"
updated_by: ACC-TEST
origin_project: zqk
origin_system: zqk
`
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify registry doesn't exist yet
	// Registry file is named .{kind}.hashes (e.g., .audit_event.hashes)
	registryFile := filepath.Join(auditDir, ".audit_event.hashes")
	if _, err := fileutil.Stat(registryFile); err == nil {
		t.Fatalf("Registry file should not exist yet, but it does")
	}

	// Create CLI context
	ctx := cli.ContextForProjectAndProfile(projectRoot, "test")

	// Create a dummy command with --auto-fix flag
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", true, "Auto-fix issues")

	// Create a hash registry cache
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Parse the test file
	yamlParser := parser.NewYAMLParser()
	obj, err := yamlParser.ParseFile(testFile)
	if err != nil {
		t.Fatalf("Failed to parse test file: %v", err)
	}

	// Create a registry for the bucketed object (in subdirectory)
	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", auditDir)
	t.Cleanup(func() {
		runHashRegistryAndGlobalTeardown(t, []*storage.HashRegistry{registry})
	})
	// Don't load - simulate missing hash scenario

	// Create issues with missing hash
	issues := []Issue{
		{
			Tier:        2,
			Category:    "integrity",
			Message:     "No integrity hash recorded for this file",
			AutoFixable: true,
		},
	}

	// Call autoFixIssues
	t.Logf("Calling autoFixIssues with:")
	t.Logf("  testFile: %s", testFile)
	t.Logf("  kind: audit_event")
	t.Logf("  registry: %v (nil=%v)", registry, registry == nil)
	t.Logf("  issues: %+v", issues)

	fixed := autoFixIssues(ctx, cmd, obj, testFile, "audit_event", issues, registry, hashRegistryCache, nil, nil)

	t.Logf("Fixed messages: %v", fixed)

	// Verify at least one fix was applied (CAS migration or in-place registry update)
	if len(fixed) == 0 {
		t.Error("Expected auto-fix to resolve missing hash, but no fixes were applied")
		t.Logf("Issues: %+v", issues)
		return
	}

	// Accept current success messages: in-place "Updated integrity hash for ..." or CAS "Migrated and indexed ... to CAS"
	acceptMsg := false
	for _, msg := range fixed {
		if strings.Contains(msg, "Updated integrity hash for AUD-TEST-001") ||
			strings.Contains(msg, "Migrated and indexed object AUD-TEST-001") {
			acceptMsg = true
			break
		}
	}
	if !acceptMsg {
		t.Errorf("Expected fix message to indicate success (Updated integrity hash or Migrated and indexed), got: %v", fixed)
	}
}
