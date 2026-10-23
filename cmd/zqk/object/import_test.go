package object

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestInProcess_ObjectImport_PositiveYAML(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	yamlContent := `- id: BLI-IMP-001
  kind: backlog_item
  title: Imported Backlog Item
  description: Valid description of sufficient length for import test.
  problem_statement: Valid problem statement for import test.
  acceptance_considerations: Valid acceptance considerations for import test.
  priority: medium
  priority_tier: P2
  priority_plan_ref: PRI-IMP-001
  milestone_refs: [MIL-IMP-001]
  status: planned
`
	importFile := filepath.Join(tempDir, "items.yaml")
	err = fileutil.WriteFile(importFile, []byte(yamlContent), paths.FilePerm600)
	require.NoError(t, err)

	out, err := executeObjectCommand(t, tempDir, provider, "import", "--file", importFile)
	assert.NoError(t, err, "import output: %s", out)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	readObj, err := provider.Read(ctx, secCtx, "BLI-IMP-001")
	require.NoError(t, err)
	assert.Equal(t, "Imported Backlog Item", readObj[objects.FieldKeyTitle])
}

func TestInProcess_ObjectImport_PositiveJSONUpsert(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	jsonContent := `[
  {
    "id": "BLI-IMP-002",
    "kind": "backlog_item",
    "title": "Imported Upsert Item",
    "description": "Valid description of sufficient length for upsert import test.",
    "problem_statement": "Valid problem statement for upsert test.",
    "acceptance_considerations": "Acceptance considerations for upsert test.",
    "priority": "high",
    "priority_tier": "P1",
    "priority_plan_ref": "PRI-IMP-002",
    "milestone_refs": ["MIL-IMP-002"],
    "status": "planned"
  }
]`
	importFile := filepath.Join(tempDir, "items.json")
	err = fileutil.WriteFile(importFile, []byte(jsonContent), paths.FilePerm600)
	require.NoError(t, err)

	out, err := executeObjectCommand(t, tempDir, provider, "import", "--file", importFile, "--mode", "upsert", "--input-format", "json")
	assert.NoError(t, err, "import upsert output: %s", out)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	readObj, err := provider.Read(ctx, secCtx, "BLI-IMP-002")
	require.NoError(t, err)
	assert.Equal(t, "Imported Upsert Item", readObj[objects.FieldKeyTitle])
}

func TestInProcess_ObjectImport_DryRunAndContinueOnError(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	yamlContent := `- id: BLI-IMP-DRY
  kind: backlog_item
  title: Dry Run Item
  description: Valid description of sufficient length.
  problem_statement: Valid problem statement.
  acceptance_considerations: Valid considerations.
  priority: low
  priority_tier: P3
  priority_plan_ref: PRI-IMP-003
  milestone_refs: [MIL-IMP-003]
  status: planned
`
	importFile := filepath.Join(tempDir, "dry_run.yaml")
	err = fileutil.WriteFile(importFile, []byte(yamlContent), paths.FilePerm600)
	require.NoError(t, err)

	out, err := executeObjectCommand(t, tempDir, provider, "import", "--file", importFile, "--dry-run")
	assert.NoError(t, err, "dry-run output: %s", out)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	exists, err := provider.Exists(ctx, secCtx, "BLI-IMP-DRY")
	require.NoError(t, err)
	assert.False(t, exists, "dry-run should not create object in storage")

	out, err = executeObjectCommand(t, tempDir, provider, "import", "--file", importFile, "--continue-on-error")
	assert.NoError(t, err, "continue-on-error output: %s", out)
}

func TestInProcess_ObjectImport_Negatives(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Missing file flag
	_, err = executeObjectCommand(t, tempDir, provider, "import")
	assert.Error(t, err)

	// Non-existent file
	_, err = executeObjectCommand(t, tempDir, provider, "import", "--file", filepath.Join(tempDir, "notfound.yaml"))
	assert.Error(t, err)

	// Corrupted file
	corruptFile := filepath.Join(tempDir, "corrupt.yaml")
	err = fileutil.WriteFile(corruptFile, []byte(":::invalid-yaml:::"), paths.FilePerm600)
	require.NoError(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "import", "--file", corruptFile)
	assert.Error(t, err)
}
