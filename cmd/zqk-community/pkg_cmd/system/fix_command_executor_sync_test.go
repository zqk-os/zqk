package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/internal/cli"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestExecuteFixCommand(t *testing.T) {
	// Get project root BEFORE setting ZQK_TEST_ROOT
	// (ResolveProjectRoot might return test root if ZQK_TEST_ROOT is set)
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		t.Fatal("Could not find project root - cannot copy spec files")
	}

	realSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "system.fix_command_executor",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "copy_specs_and_kind_dirs",
				Fn: func() error {
					testSpecsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
					if err := os.MkdirAll(testSpecsDir, paths.DirPerm755); err != nil {
						return err
					}
					specEntries, err := os.ReadDir(realSpecsDir)
					if err != nil {
						return err
					}
					for _, entry := range specEntries {
						if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
							continue
						}
						srcPath := filepath.Join(realSpecsDir, entry.Name())
						dstPath := filepath.Join(testSpecsDir, entry.Name())
						srcData, err := os.ReadFile(srcPath)
						if err != nil {
							return err
						}
						if err := os.WriteFile(dstPath, srcData, paths.FilePerm644); err != nil {
							return err
						}
					}
					milestoneDir := datacell.CellCASPrimaryDir(root, "milestones")
					if err := os.MkdirAll(milestoneDir, paths.DirPerm755); err != nil {
						return err
					}
					backlogDir := filepath.Join(root, paths.ProcessBacklogDir)
					return os.MkdirAll(backlogDir, paths.DirPerm755)
				},
			}}
		},
	})
	testRoot := proj.Root
	storageProvider := proj.FileStorage

	stdctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	milestoneObj := map[string]any{
		objects.FieldKeyID:            "MIL-999",
		objects.FieldKeyKind:          "milestone",
		objects.FieldKeyTitle:         "White-Label Branding System",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyCategory:      "feature",
		objects.FieldKeyTags:          []string{"branding", "white-label"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2025-01-07T00:00:00Z",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     "2025-01-07T00:00:00Z",
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
	}

	if err := storageProvider.Create(stdctx, secCtx, milestoneObj); err != nil {
		t.Fatalf("Failed to create milestone: %v", err)
	}

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)

	backlogObj := map[string]any{
		objects.FieldKeyID:            "ITEM-998",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "White-Label Branding System",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyCategory:      "feature",
		objects.FieldKeyTags:          []string{"branding", "white-label"},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2025-01-07T00:00:00Z",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     "2025-01-07T00:00:00Z",
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
	}

	if err := storageProvider.Create(stdctx, secCtx, backlogObj); err != nil {
		t.Fatalf("Failed to create backlog item: %v", err)
	}

	// Get file path using findObjectByID
	backlogFilePath, _ := findObjectByID(testRoot, "ITEM-998")
	if backlogFilePath == emptyValue {
		// Fallback to expected path
		backlogFilePath = filepath.Join(backlogDir, "ITEM-998.yaml")
	}

	yamlParser := parser.NewYAMLParser()
	parsedObj, err := yamlParser.ParseFile(backlogFilePath)
	if err != nil {
		// If parsing fails, create a minimal ParsedObject from the object we created
		readObj, readErr := storageProvider.Read(stdctx, secCtx, "ITEM-998")
		if readErr != nil {
			t.Fatalf("Failed to read backlog item: %v", readErr)
		}
		parsedObj = &parser.ParsedObject{
			ID:         "ITEM-998",
			Kind:       "backlog_item",
			Properties: readObj,
		}
	}

	// Create spec loader
	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)

	// Create fix context
	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	fixCtx := &AutoFixContext{
		Ctx:      ctx,
		Obj:      parsedObj,
		FilePath: backlogFilePath,
		Kind:     "backlog_item",
		Logger:   logging.GetLoggerFromProfile("test"),
	}

	// Create issue with fix command
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	issue := Issue{
		Tier:        1,
		Category:    "instance_validation",
		Message:     "status: Precondition not met for status 'planned': At least one milestone_ref linked",
		AutoFixable: true,
		FixCommand:  fmt.Sprintf("%s object update ITEM-998 --field milestone_refs+=<MILESTONE_ID:category=feature&tags=branding,white-label&title_pattern=White-Label Branding System>", cliCmd),
	}

	// Verify milestone exists and can be queried
	t.Logf("Verifying milestone exists in storage...")
	verifyObj, verifyErr := storageProvider.Read(stdctx, secCtx, "MIL-999")
	if verifyErr != nil {
		t.Fatalf("Failed to verify milestone exists: %v", verifyErr)
	}
	t.Logf("Milestone found: ID=%v, title=%v, category=%v, tags=%v",
		verifyObj[objects.FieldKeyID], verifyObj[objects.FieldKeyTitle], verifyObj[objects.FieldKeyCategory], verifyObj[objects.FieldKeyTags])

	// Execute fix command
	t.Logf("Executing fix command: %s", issue.FixCommand)
	success, msg := executeFixCommand(ctx, fixCtx, issue, storageProvider, specLoader)
	if !success {
		t.Logf("Fix command failed. Message: %s", msg)
		t.Logf("This might be due to:")
		t.Logf("  1. Placeholder resolution finding 0 candidates")
		t.Logf("  2. Field resolver errors")
		t.Logf("  3. Storage provider update errors")
		// For now, let's check if we can at least parse the command
		fixCmd, parseErr := parseFixCommand(issue.FixCommand)
		if parseErr != nil {
			t.Fatalf("Failed to parse fix command: %v", parseErr)
		}
		t.Logf("Parsed command: ObjectID=%s, Field=%s, Operator=%s, Placeholder=%s",
			fixCmd.ObjectID, fixCmd.Field, fixCmd.Operator, fixCmd.Placeholder)
		t.Fatalf("Expected fix command to succeed, but it failed. Message: %s", msg)
	}

	t.Logf("Fix command executed: %s", msg)

	// Verify the fix was applied
	readObj, readErr := storageProvider.Read(stdctx, secCtx, "ITEM-998")
	if readErr != nil {
		t.Fatalf("Failed to read object after fix: %v", readErr)
	}

	milestoneRefs, ok := readObj[objects.FieldKeyMilestoneRefs].([]any)
	if !ok {
		milestoneRefsStr, ok := readObj[objects.FieldKeyMilestoneRefs].([]string)
		if !ok {
			t.Fatalf("milestone_refs is not a list (type: %T)", readObj[objects.FieldKeyMilestoneRefs])
		}
		// Convert []string to []any
		milestoneRefs = make([]any, len(milestoneRefsStr))
		for i, v := range milestoneRefsStr {
			milestoneRefs[i] = v
		}
	}

	if len(milestoneRefs) == 0 {
		t.Fatal("Expected milestone_refs to be set after fix, but it's empty")
	}

	if milestoneRefs[0] != "MIL-999" {
		t.Errorf("Expected milestone_refs[0] to be MIL-999, got %v", milestoneRefs[0])
	}
}
