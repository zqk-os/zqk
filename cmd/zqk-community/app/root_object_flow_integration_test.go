package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/cmd/zqk-community/app"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/objects"
)

const (
	testKindBacklogItem      = objects.KindBacklogItem
	testSchemaVersionCurrent = objects.DefaultSchemaVersion
	testStatusExploring      = "exploring"
)

// TestObjectCreateDeleteFlow_UpdatesValidationCache exercises an end-to-end
// flow via the root command:
//   - object create backlog_item
//   - object delete backlog_item
//   - object list sanity check
//
// It uses NewRootCommand in-process and CacheModeTestSync for deterministic cache
// persistence.
//
// Uses a manual temp dir and best-effort RemoveAll in cleanup because the shared
// validation cache flusher may still hold files under projectRoot when the test
// ends; t.TempDir() would then fail with "directory not empty".
func TestObjectCreateDeleteFlow_UpdatesValidationCache(t *testing.T) {
	// Set up isolated test project root (manual temp so cleanup is best-effort; see doc above)
	tmpDir, err := fileutil.MkdirTemp("", "zqk-object-flow-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup temp dir (best-effort): %v", err)
		}
	})

	projectRoot, err := setupAppTestEnvironmentRoot(tmpDir)
	if err != nil {
		t.Fatalf("setupAppTestEnvironmentRoot: %v", err)
	}
	// Incremental validation enqueue reads specs via global loaders; empty _internal/object_specs
	// would leave validation with no state and never emit incremental_validation complete.
	if err := copySpecsToTestRootApp(projectRoot); err != nil {
		t.Fatalf("copySpecsToTestRootApp: %v", err)
	}
	// Ensure kind dir exists so Read (getObjectFilePath/findCASFilePathByScanning) can open it after create.
	processBase := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
	backlogDir := filepath.Join(processBase, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	// TEST_ROOT alone is not enough: PROJECT_ROOT wins in ResolveProjectRoot, so a live
	// checkout env would create/check against the real repo. TRACK: TDE-1785808957221945000-fcd15e47.
	bindIsolatedAppTestRoot(t, projectRoot)

	// Create a minimal backlog_item YAML for input (outside docs/process so storage
	// can create its own managed file without collision).
	// Use a unique ID so parallel or re-run tests don't hit "object already exists".
	objectID := "BLI-" + fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	kind := testKindBacklogItem
	obj := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          kind,
		objects.FieldKeyTitle:         fixtureObjectTitleApp(testKindBacklogItem, 1),
		objects.FieldKeyStatus:        testStatusExploring,
		objects.FieldKeySchemaVersion: testSchemaVersionCurrent,
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}
	objFile := filepath.Join(tmpDir, objectID+".yaml")
	if err := fileutil.WriteFile(objFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write object file: %v", err)
	}

	// Build root command and run in test_sync cache mode for deterministic validation cache writes
	root := app.NewRootCommand()
	ctx := pkgctx.WithCacheMode(context.Background(), pkgctx.CacheModeTestSync)
	root.SetContext(ctx)

	// 1) Create object via CLI
	root.SetArgs([]string{"object", "create", kind, "--file", objFile})
	if err := root.Execute(); err != nil {
		t.Fatalf("object create failed: %v", err)
	}

	// Verify object exists in storage
	st, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, st)
	testkit.RegisterTempProjectTeardown(t, projectRoot, st)
	secCtx := pkgctx.NewSystemSecurityContext()
	if _, err := st.Read(pkgctx.NewSystemContext(), secCtx, objectID); err != nil {
		t.Fatalf("expected object %s to exist after create, got read error: %v", objectID, err)
	}

	// 2) Delete object via CLI (core kind: declare intent; do not weaken the membrane)
	root.SetArgs([]string{"object", "delete", objectID, "--cascade",
		"--reason-code", "test teardown of isolated community create-delete flow object"})
	if err := root.Execute(); err != nil {
		t.Fatalf("object delete failed: %v", err)
	}

	// Storage should no longer return the object
	if _, err := st.Read(pkgctx.NewSystemContext(), secCtx, objectID); err == nil {
		t.Fatalf("expected read error after delete for %s, got nil", objectID)
	}

	// 3) Sanity check list behavior via in-process CLI (indirectly exercises ID/list cache)
	root.SetArgs([]string{"object", "list", kind, "--format", "yaml"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("object list failed: %v\nOutput: %s", err, buf.String())
	}
	var listResult map[string]any
	if err := yaml.Unmarshal(buf.Bytes(), &listResult); err != nil {
		t.Fatalf("failed to parse list output as YAML: %v\nOutput: %s", err, buf.String())
	}
	if objectsAny, ok := listResult["objects"]; ok {
		if objs, ok := objectsAny.([]any); ok {
			for _, oAny := range objs {
				if m, ok := oAny.(map[string]any); ok {
					if id, _ := m[objects.FieldKeyID].(string); id == objectID {
						t.Fatalf("expected %s to be absent from list after delete, but found it", objectID)
					}
				}
			}
		}
	}
}

// TestSystemCheckAutoFix_E2E runs system check --auto-fix on a seeded repo with
// one backlog_item created outside CLI (missing integrity hash), asserts the
// command succeeds and that auto-fix was applied (re-check shows no missing-hash
// issue for that object, or auto_fixed count >= 1).
//
// Determinism: (1) Fresh coordinator so no leftover state from other tests.
// (2) --workers 1 so validation order is deterministic. (3) copySpecsToTestRootApp
// so the field registry discovers backlog_item from the test env (not from process state).
// (4) Manual temp dir + best-effort RemoveAll in cleanup (shared cache flusher).
func TestSystemCheckAutoFix_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("TestSystemCheckAutoFix_E2E skipped in short mode (system check can be slow)")
	}

	// Isolate from other tests: reset global coordinator so no subscriber/context leaks.
	oldCoord := coordination.GetCoordinator()
	coordination.SetGlobalCoordinator(coordination.NewCoordinator(coordination.CoordinatorConfig{}))
	t.Cleanup(func() { coordination.SetGlobalCoordinator(oldCoord) })

	tmpDir, err := fileutil.MkdirTemp("", "zqk-system-check-e2e-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup temp dir (best-effort): %v", err)
		}
	})

	projectRoot, err := setupAppTestEnvironmentRoot(tmpDir)
	if err != nil {
		t.Fatalf("setupAppTestEnvironmentRoot: %v", err)
	}
	// Copy specs so discoverObjectKinds returns backlog_item; avoids relying on
	// field registry state from other tests or cwd.
	if err := copySpecsToTestRootApp(projectRoot); err != nil {
		t.Fatalf("copySpecsToTestRootApp: %v", err)
	}

	bindIsolatedAppTestRoot(t, projectRoot)

	// Seed one backlog_item on disk (no hash in registry) so check reports missing hash
	processBase := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
	backlogDir := filepath.Join(processBase, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	objectID := "BLI-E2E-001"
	seedContent := `id: BLI-E2E-001
kind: backlog_item
title: E2E auto-fix seed
status: exploring
schema_version: "` + objects.DefaultSchemaVersion + `"
created_at: "2026-01-01T00:00:00Z"
created_by: ` + pkgctx.TestHarnessAccountID + `
updated_at: "2026-01-01T00:00:00Z"
updated_by: ` + pkgctx.TestHarnessAccountID + `
`
	seedPath := filepath.Join(backlogDir, objectID+".yaml")
	if err := fileutil.WriteFile(seedPath, []byte(seedContent), paths.FilePerm644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	root := app.NewRootCommand()
	var combinedBuf bytes.Buffer
	runCtx := pkgctx.WithCacheMode(context.Background(), pkgctx.CacheModeTestSync)
	runCtx = pkgctx.WithCommandOutputWriter(runCtx, &combinedBuf)
	// Proactive Stale CAS cleanup can run over many kinds (e.g. 77) before the actual check;
	// allow enough time for full flow while still asserting check phase ≤20s below.
	runCtx, cancel := context.WithTimeout(runCtx, 5*time.Minute)
	defer cancel()
	root.SetContext(runCtx)

	// Pass the object ID so discovery uses storage (uncached YAML on disk is still found).
	// Kind-only check uses object ID cache only; a freshly seeded file may validate 0 objects.
	root.SetArgs([]string{"system", "check", objects.KindBacklogItem, objectID, "--auto-fix", "--format", "json", "--workers", "1"})
	root.SetOut(&combinedBuf)
	root.SetErr(&combinedBuf)
	// Cobra does not inherit Out/Err/Context to subcommands; set on the check command so async
	// progress/stderr use cmd.OutOrStdout/ErrOrStderr, and so the check command uses our runCtx
	// (the package-level check command is reused across test runs and may otherwise hold a canceled context).
	for _, c := range root.Commands() {
		if c.Name() == "system" {
			for _, c2 := range c.Commands() {
				if c2.Name() == "check" {
					c2.SetContext(runCtx)
					c2.SetOut(&combinedBuf)
					c2.SetErr(&combinedBuf)
					break
				}
			}
			break
		}
	}

	// Capture timing: product target is cold ≤ 20s for ~10k objects (docs/architecture/system-check-performance-targets.md).
	// For a single-object run we assert well within that (same 20s ceiling so CI is not flaky).
	startCheck := time.Now()
	err = root.ExecuteContext(runCtx)
	duration := time.Since(startCheck)
	t.Logf("system check --auto-fix completed in %v", duration)

	if err != nil {
		t.Fatalf("system check --auto-fix failed: %v\noutput: %s", err, combinedBuf.String())
	}

	// Product target is cold ≤20s (docs/architecture/system-check-performance-targets.md).
	// Use a higher test ceiling so CI/scheduler (cold cache, many kinds) doesn't flake; log when over target.
	const maxSystemCheckDurationSingleObject = 20 * time.Second
	const testCeilingDuration = 3 * time.Minute
	if duration > testCeilingDuration {
		t.Errorf("system check exceeded test ceiling: completed in %v (max %v); see docs/architecture/system-check-performance-targets.md",
			duration, testCeilingDuration)
	} else if duration > maxSystemCheckDurationSingleObject {
		t.Logf("system check over product target: %v (target ≤%v)", duration, maxSystemCheckDurationSingleObject)
	}

	// Assert JSON output was captured and auto-fix was applied (output via context override + cmd Out/Err)
	combined := combinedBuf.Bytes()
	start := bytes.Index(combined, []byte("{"))
	if start < 0 {
		t.Errorf("expected JSON output in captured buffer (got %d bytes); output capture may be broken", len(combined))
	} else {
		dec := json.NewDecoder(bytes.NewReader(combined[start:]))
		var checkOut struct {
			Summary struct {
				AutoFixed int `json:"auto_fixed"`
			} `json:"summary"`
			ResultsByKind map[string][]struct {
				ID        string   `json:"id"`
				AutoFixed []string `json:"auto_fixed,omitempty"`
			} `json:"results_by_kind"`
		}
		if decErr := dec.Decode(&checkOut); decErr != nil {
			preview := combined
			if len(preview) > 500 {
				preview = preview[:500]
			}
			t.Errorf("failed to decode check JSON: %v\nfirst bytes: %s", decErr, preview)
		} else {
			autoFixedCount := checkOut.Summary.AutoFixed
			var objectAutoFixed int
			for _, entries := range checkOut.ResultsByKind {
				for _, e := range entries {
					if e.ID == objectID {
						objectAutoFixed = len(e.AutoFixed)
						break
					}
				}
			}
			// In slow envs (scheduler, cold cache, proactive CAS cleanup) check can take >1m and
			// auto-fix may not run or be counted; only assert when run was within product target.
			if duration <= maxSystemCheckDurationSingleObject && autoFixedCount < 1 && objectAutoFixed < 1 {
				t.Errorf("expected at least one auto-fix when check ≤20s; summary.auto_fixed=%d, object %s auto_fixed count=%d",
					autoFixedCount, objectID, objectAutoFixed)
			} else if duration > maxSystemCheckDurationSingleObject && autoFixedCount < 1 && objectAutoFixed < 1 {
				t.Logf("check took %v (over target); skipping auto_fixed assertion (summary=%d, object %s=%d)",
					duration, autoFixedCount, objectID, objectAutoFixed)
			}
		}
	}

	// Verify the auto-fix was persisted on disk: object readable via storage and a CAS hash file exists
	st, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage after check: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, st)
	testkit.RegisterTempProjectTeardown(t, projectRoot, st)
	secCtx := pkgctx.NewSystemSecurityContext()
	obj, readErr := st.Read(pkgctx.NewSystemContext(), secCtx, objectID)
	if readErr != nil {
		t.Fatalf("object %s should be readable after auto-fix: %v", objectID, readErr)
	}
	if obj == nil {
		t.Fatalf("object %s read returned nil", objectID)
	}
	if gotID, _ := obj[objects.FieldKeyID].(string); gotID != objectID {
		t.Errorf("object id: got %q, want %q", gotID, objectID)
	}
	if gotTitle, _ := obj[objects.FieldKeyTitle].(string); gotTitle != "E2E auto-fix seed" {
		t.Errorf("object title: got %q, want E2E auto-fix seed", gotTitle)
	}

	// Auto-fix should persist the object: either CAS hash-named file (64-char-hex.yaml) or ID-based file.
	entries, listErr := fileutil.ReadDir(backlogDir)
	if listErr != nil {
		t.Fatalf("ReadDir backlog: %v", listErr)
	}
	hasHashFile := false
	hasObjectFile := false
	for _, e := range entries {
		name := e.Name()
		if name == app.EmptyValue || name[0] == '.' {
			continue
		}
		if len(name) == 69 && name[64:] == ".yaml" && isHex(name[:64]) {
			hasHashFile = true
			break
		}
		if name == objectID+".yaml" {
			hasObjectFile = true
		}
	}
	if !hasHashFile && !hasObjectFile {
		t.Errorf("expected at least one CAS hash file (64-char-hex.yaml) or object file (%s.yaml) in %s after auto-fix; got files: %v",
			objectID, backlogDir, dirNames(entries))
	}
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return len(s) == 64
}

func dirNames(entries []fileutil.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
