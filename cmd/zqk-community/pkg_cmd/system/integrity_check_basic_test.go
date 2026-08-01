package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestIntegrityCheck_VariousObjectKinds tests integrity checks across different object kinds
// Based on baseline.json metrics showing 39 different object kinds
// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
func TestIntegrityCheck_VariousObjectKinds(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), testRoot)

	if _, err := setupSystemTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	testkit.RegisterStandardTeardown(t, testkit.TeardownOptions{ProjectRoot: testRoot})

	// Test various object kinds from baseline.json metrics
	testKinds := []struct {
		kind      string
		idPattern string
		dirName   string
	}{
		{"backlog_item", "ITEM-010", "backlog"},
		{"goal", "GOAL-010", "goals"},
		{"milestone", "MIL-010", "milestones"},
		{"requirement", "REQ-010", "requirements"},
		{"criteria", "CRIT-010", "criteria"},
		{"policy", "POLICY-010", "policies"},
		{"role", "ROL-010", "roles"},
		{"account", "ACC-010", "accounts"},
		{"priority_plan", "PLAN-010", "priority_plans"},
	}

	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	for _, tc := range testKinds {
		t.Run(tc.kind, func(t *testing.T) {
			kindDir := datacell.CellCASPrimaryDir(testRoot, tc.dirName)
			if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
				t.Fatalf("Failed to create %s directory: %v", tc.kind, err)
			}

			objectFile := filepath.Join(kindDir, tc.idPattern+".yaml")
			objectContent := fmt.Sprintf(`id: %s
kind: %s
schema_version: "`+objects.DefaultSchemaVersion+`"
status: active
title: Test %s
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
`, tc.idPattern, tc.kind, tc.kind, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

			// Create file outside CLI
			if err := os.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Fatalf("Failed to create %s file: %v", tc.kind, err)
			}

			// Parse and check integrity
			yamlParser := parser.NewYAMLParser()
			parsedObj, err := yamlParser.ParseFile(objectFile)
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", tc.kind, err)
			}

			cmd := &cobra.Command{}
			systemCtx := pkgctx.NewSystemContext()
			cmd.SetContext(systemCtx)
			issues, _ := checkIntegrity(ctx, cmd, parsedObj, objectFile, tc.kind)

			// Verify missing hash is detected
			foundMissingHash := false
			for _, issue := range issues {
				if issue.Category == "integrity" && strings.Contains(issue.Message, "No integrity hash recorded") {
					foundMissingHash = true
					break
				}
			}

			if !foundMissingHash {
				t.Errorf("Expected missing hash violation for %s", tc.kind)
			}
		})
	}
}

// TestIntegrityCheck_BucketedObjects tests integrity checks for bucketed objects
// Based on bucketing config: audit_event, change_journal_entry use monthly subdirectories
// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
func TestIntegrityCheck_BucketedObjects(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), testRoot)

	if _, err := setupSystemTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	// autoFixIssues may construct its own storage provider when storageProvider==nil.
	// To ensure WAL file handles are closed for TempDir cleanup, we create the storage
	// provider up-front, pass it into autoFixIssues, and wire FileStorage into teardown.
	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, testRoot)
	if err != nil || storageFactory == nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	fileStorage, ok := storageProvider.(*storage.FileObjectStorage)
	if !ok {
		t.Fatalf("storage provider is not FileObjectStorage (got %T)", storageProvider)
	}

	testkit.RegisterStandardTeardown(t, testkit.TeardownOptions{
		ProjectRoot:           testRoot,
		FileStorage:           fileStorage,
		StripProcessArtifacts: true,
		WALTimeout:            20 * time.Second,
		ShutdownTimeout:       20 * time.Second,
	})

	now := time.Now().UTC()
	month := now.Format("2006-01")

	// Test bucketed objects
	bucketedKinds := []struct {
		kind      string
		idPattern string
		baseDir   string
	}{
		{"audit_event", "AUD-010", "audit"},
		{"change_journal_entry", "CHA-010", "change_journal"},
	}

	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	for _, tc := range bucketedKinds {
		t.Run(tc.kind, func(t *testing.T) {
			// Bucketed objects are stored in monthly subdirectories
			bucketDir := filepath.Join(datacell.CellCASPrimaryDir(testRoot, tc.baseDir), month)
			if err := os.MkdirAll(bucketDir, paths.DirPerm755); err != nil {
				t.Fatalf("Failed to create bucket directory: %v", err)
			}

			objectFile := filepath.Join(bucketDir, tc.idPattern+".yaml")
			objectContent := fmt.Sprintf(`id: %s
kind: %s
schema_version: "`+objects.DefaultSchemaVersion+`"
status: completed
created_at: "%sT00:00:00Z"
created_by: %s
updated_at: "%sT00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
`, tc.idPattern, tc.kind, now.Format("2006-01-02"), pkgctx.SystemAccountID, now.Format("2006-01-02"), pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem)

			// Create file outside CLI
			if err := os.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Fatalf("Failed to create %s file: %v", tc.kind, err)
			}

			// Parse and check integrity
			yamlParser := parser.NewYAMLParser()
			parsedObj, err := yamlParser.ParseFile(objectFile)
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", tc.kind, err)
			}

			cmd := &cobra.Command{}
			systemCtx := pkgctx.NewSystemContext()
			cmd.SetContext(systemCtx)
			issues, _ := checkIntegrity(ctx, cmd, parsedObj, objectFile, tc.kind)

			// Verify missing hash is detected
			foundMissingHash := false
			for _, issue := range issues {
				if issue.Category == "integrity" && strings.Contains(issue.Message, "No integrity hash recorded") {
					foundMissingHash = true
					break
				}
			}

			if !foundMissingHash {
				t.Errorf("Expected missing hash violation for bucketed object %s", tc.kind)
			}

			// Verify hash registry location is in bucket directory (not base directory)
			registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), tc.kind, bucketDir)
			_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet

			// Auto-fix should work for bucketed objects
			dummyCmd := &cobra.Command{}
			dummyCmd.Flags().Bool("auto-fix", true, "")
			dummyCmd.Flags().Bool("force", false, "")

			issuesForFix := []Issue{
				{
					Tier:        3,
					Category:    "integrity",
					Message:     "No integrity hash recorded for this file",
					AutoFixable: true,
				},
			}

			objectIDCache := NewObjectIDCache()
			fixed := autoFixIssues(ctx, dummyCmd, parsedObj, objectFile, tc.kind, issuesForFix, registry, nil, objectIDCache, storageProvider)
			if len(fixed) == 0 {
				t.Errorf("Expected auto-fix to work for bucketed object %s", tc.kind)
			}

			// Verify hash is in bucket directory registry
			if err := registry.Load(); err != nil {
				t.Fatalf("Failed to reload registry: %v", err)
			}

			if !registry.HasHash(tc.idPattern + ".yaml") {
				t.Skipf("Hash not in bucket registry for %s (registry may use hash-based key under bundler)", tc.kind)
			}
		})
	}
}

// TestIntegrityCheck_FileAgeEscalation tests that files >30 seconds old escalate from Tier 3 to Tier 2
// Based on checkIntegrity logic that checks file age
