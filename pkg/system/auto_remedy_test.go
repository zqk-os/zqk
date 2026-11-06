package system_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/system"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAutoRemedy_DiagnoseStaleLocksAndTemps(t *testing.T) {
	tmpDir := t.TempDir()

	zqkDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(zqkDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create .zqk dir: %v", err)
	}

	// Create a stale lock file (older than DefaultLockStaleAge 15m)
	staleLock := filepath.Join(zqkDir, "test.lock")
	if err := fileutil.WriteFile(staleLock, []byte("lock content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create stale lock: %v", err)
	}
	oldTime := time.Now().Add(-20 * time.Minute)
	if err := fileutil.Chtimes(staleLock, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set stale lock modtime: %v", err)
	}

	// Create an orphaned temp file (older than DefaultTempOrphanAge 30m)
	orphanTmp := filepath.Join(zqkDir, "test.tmp")
	if err := fileutil.WriteFile(orphanTmp, []byte("temp content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create orphan tmp: %v", err)
	}
	veryOldTime := time.Now().Add(-45 * time.Minute)
	if err := fileutil.Chtimes(orphanTmp, veryOldTime, veryOldTime); err != nil {
		t.Fatalf("failed to set orphan tmp modtime: %v", err)
	}

	engine := system.NewDiagnosticsRemedyEngine(tmpDir)
	plans, err := engine.Diagnose(context.Background(), nil)
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}

	hasLockPlan := false
	hasTmpPlan := false
	for _, p := range plans {
		if p.ActionType == system.ActionRemoveFile {
			if filepath.Base(p.Target) == "test.lock" {
				hasLockPlan = true
			}
			if filepath.Base(p.Target) == "test.tmp" {
				hasTmpPlan = true
			}
		}
	}

	if !hasLockPlan {
		t.Errorf("expected plan for stale lock file, got: %+v", plans)
	}
	if !hasTmpPlan {
		t.Errorf("expected plan for orphaned tmp file, got: %+v", plans)
	}

	// Apply auto-remedy
	report, err := engine.Apply(context.Background(), plans)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if report.TotalApplied < 2 {
		t.Errorf("expected at least 2 applied remedies, got %d", report.TotalApplied)
	}

	// Verify files are cleaned up
	if _, err := fileutil.Stat(staleLock); !fileutil.IsNotExist(err) {
		t.Errorf("expected stale lock to be deleted, stat err: %v", err)
	}
	if _, err := fileutil.Stat(orphanTmp); !fileutil.IsNotExist(err) {
		t.Errorf("expected orphan tmp to be deleted, stat err: %v", err)
	}
}

func TestAutoRemedy_DiagnoseUnseededPreconditions(t *testing.T) {
	tmpDir := t.TempDir()

	engine := system.NewDiagnosticsRemedyEngine(tmpDir)
	policySeeded := false
	personaSeeded := false

	engine.PolicySeeder = func(root string) (int, error) {
		policySeeded = true
		return 3, nil
	}
	engine.PersonaSeeder = func(root string) (int, error) {
		personaSeeded = true
		return 5, nil
	}

	plans, err := engine.Diagnose(context.Background(), nil)
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}

	hasPolicyPlan := false
	hasPersonaPlan := false
	for _, p := range plans {
		if p.ID == "REMEDY-UNSEEDED-POLICIES" {
			hasPolicyPlan = true
		}
		if p.ID == "REMEDY-UNSEEDED-PERSONAS" {
			hasPersonaPlan = true
		}
	}

	if !hasPolicyPlan {
		t.Errorf("expected REMEDY-UNSEEDED-POLICIES in plans: %+v", plans)
	}
	if !hasPersonaPlan {
		t.Errorf("expected REMEDY-UNSEEDED-PERSONAS in plans: %+v", plans)
	}

	// Apply seed remedies
	report, err := engine.Apply(context.Background(), plans)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !policySeeded {
		t.Errorf("expected PolicySeeder to be called")
	}
	if !personaSeeded {
		t.Errorf("expected PersonaSeeder to be called")
	}
	if report.TotalApplied != 2 {
		t.Errorf("expected 2 applied remedies, got %d", report.TotalApplied)
	}
}

func TestAutoRemedy_DiagnoseCheckIssues(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	mockResults := []systemcheck.CheckResult{
		{
			ObjectID: "BLI-123",
			Issues: []systemcheck.Issue{
				{
					Message:    "Precondition not met: missing estimated_effort",
					FixCommand: "zqk object update BLI-123 --field estimated_effort=1d",
				},
				{
					Message:     "Hash mismatch in object registry",
					AutoFixable: true,
				},
			},
		},
	}

	plans, err := engine.Diagnose(context.Background(), mockResults)
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}

	if len(plans) < 2 {
		t.Fatalf("expected at least 2 plans from check issues, got %d", len(plans))
	}
}

func TestAutoRemedy_ApplyKillProcess(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	plans := []system.RemedyPlan{
		{
			ID:         "REMEDY-ORPHAN-PROC-PID-9999",
			Title:      "Terminate Orphaned Process",
			ActionType: system.ActionKillProcess,
			Target:     "PID 9999: zqk",
			Confidence: 1.0,
			AutoApply:  true,
		},
	}

	report, err := engine.Apply(context.Background(), plans)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if report.TotalApplied != 1 {
		t.Errorf("expected 1 applied remedy, got %d", report.TotalApplied)
	}
}

func TestAutoRemedy_OrphanedLockFilesCleaning(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup directories: .zqk/state, .zqk/lock, .zqk/scheduler/events
	stateDir := filepath.Join(tmpDir, paths.ProjectDataDir, "state")
	lockDir := filepath.Join(tmpDir, paths.ProjectDataDir, "lock")
	eventsDir := filepath.Join(tmpDir, paths.ProjectDataDir, "scheduler", "events")

	for _, d := range []string{stateDir, lockDir, eventsDir} {
		if err := fileutil.MkdirAll(d, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	sessionLock := filepath.Join(stateDir, "session.lock")
	autofixLock := filepath.Join(lockDir, "autofix-processing.lock")
	coordLock := filepath.Join(eventsDir, "coordination-bus.jsonl.lock")

	// 1. session.lock with dead PID (created recently)
	deadPIDContent := []byte(`{"pid": 9999999, "created_at": "` + time.Now().Add(-10*time.Second).Format(time.RFC3339) + `"}` + "\n")
	if err := fileutil.WriteFile(sessionLock, deadPIDContent, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write session.lock: %v", err)
	}

	// 2. autofix-processing.lock with dead PID
	if err := fileutil.WriteFile(autofixLock, deadPIDContent, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write autofix-processing.lock: %v", err)
	}

	// 3. coordination-bus.jsonl.lock (aged > 5m)
	if err := fileutil.WriteFile(coordLock, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write coordination-bus.jsonl.lock: %v", err)
	}
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := fileutil.Chtimes(coordLock, oldTime, oldTime); err != nil {
		t.Fatalf("failed to chtimes coordLock: %v", err)
	}

	engine := system.NewDiagnosticsRemedyEngine(tmpDir)
	plans, err := engine.Diagnose(context.Background(), nil)
	if err != nil {
		t.Fatalf("diagnose failed: %v", err)
	}

	remedyTargets := make(map[string]bool)
	for _, p := range plans {
		if p.ActionType == system.ActionRemoveFile {
			remedyTargets[filepath.Base(p.Target)] = true
		}
	}

	if !remedyTargets["session.lock"] {
		t.Errorf("expected session.lock to be diagnosed for removal")
	}
	if !remedyTargets["autofix-processing.lock"] {
		t.Errorf("expected autofix-processing.lock to be diagnosed for removal")
	}
	if !remedyTargets["coordination-bus.jsonl.lock"] {
		t.Errorf("expected coordination-bus.jsonl.lock to be diagnosed for removal")
	}

	// Apply auto-remedy
	report, err := engine.Apply(context.Background(), plans)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if report.TotalApplied < 3 {
		t.Errorf("expected at least 3 applied lock remedies, got %d", report.TotalApplied)
	}

	// Verify all 3 files are removed
	for _, f := range []string{sessionLock, autofixLock, coordLock} {
		if fileutil.Exists(f) {
			t.Errorf("expected %s to be removed by auto-remedy, but file still exists", filepath.Base(f))
		}
	}
}

// TestSanitizeRecipeCommand_RemovesIDsAcrossAllVerbsAndPreservesPositionalIDs verifies
// that sanitizeRecipeCommand comprehensively handles recipe commands across all verbs,
// eliminating invalid '--ids ' and '--ids=' flag usage while preserving positional arguments.
func TestSanitizeRecipeCommand_RemovesIDsAcrossAllVerbsAndPreservesPositionalIDs(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "system check with auto-fix and --ids",
			input:    "zqk system check --auto-fix --ids OBJ-123",
			expected: "zqk system check --auto-fix OBJ-123",
		},
		{
			name:     "system check with direct --ids",
			input:    "zqk system check --ids OBJ-123",
			expected: "zqk system check OBJ-123",
		},
		{
			name:     "object delete with --ids and subsequent flag",
			input:    "zqk object delete --ids OBJ-456 --unlink-references",
			expected: "zqk object delete OBJ-456 --unlink-references",
		},
		{
			name:     "object update with --ids= syntax",
			input:    "zqk object update --ids=OBJ-789 --field title=New",
			expected: "zqk object update OBJ-789 --field title=New",
		},
		{
			name:     "object promote with --ids",
			input:    "zqk object promote --ids OBJ-101",
			expected: "zqk object promote OBJ-101",
		},
		{
			name:     "object demote with --ids=",
			input:    "zqk object demote --ids=OBJ-202",
			expected: "zqk object demote OBJ-202",
		},
		{
			name:     "object park with --ids",
			input:    "zqk object park --ids OBJ-303",
			expected: "zqk object park OBJ-303",
		},
		{
			name:     "object get with --ids",
			input:    "zqk object get --ids OBJ-404",
			expected: "zqk object get OBJ-404",
		},
		{
			name:     "object inspect with --ids",
			input:    "zqk object inspect --ids OBJ-505",
			expected: "zqk object inspect OBJ-505",
		},
		{
			name:     "workflow gen-trace-pipeline with --ids",
			input:    "zqk workflow gen-trace-pipeline --ids OBJ-606",
			expected: "zqk workflow gen-trace-pipeline OBJ-606",
		},
		{
			name:     "multiple consecutive spaces around --ids",
			input:    "zqk system check   --ids   OBJ-707",
			expected: "zqk system check OBJ-707",
		},
		{
			name:     "empty --ids quotes at end",
			input:    "zqk system check --auto-fix --ids=\"\"",
			expected: "zqk system check --auto-fix",
		},
		{
			name:     "trailing --ids without argument",
			input:    "zqk object delete OBJ-123 --ids",
			expected: "zqk object delete OBJ-123",
		},
		{
			name:     "multiple --ids in single command",
			input:    "zqk object delete --ids OBJ-1 --ids OBJ-2",
			expected: "zqk object delete OBJ-1 OBJ-2",
		},
		{
			name:     "preserving quoted values with whitespace",
			input:    "zqk object update OBJ-123 --field \"title=Refined Title with Spaces\"",
			expected: "zqk object update OBJ-123 --field \"title=Refined Title with Spaces\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := system.SanitizeRecipeCommand(tc.input)
			assert.Equal(t, tc.expected, got)
			// Ensure sanitized output passes validation when non-empty
			if got != "" {
				err := system.ValidateRecipeCommand(got)
				assert.NoError(t, err, "sanitized command %q should pass ValidateRecipeCommand", got)
			}
		})
	}
}

// TestValidateRecipeCommand_NegativeAdversarialBoundary satisfies CRIT-1793961678251838000-c44a82a1:
// Negative Adversarial Boundary: Invalid flag arguments passed to auto-remedy recipes fail with explicit descriptive validation errors.
func TestValidateRecipeCommand_NegativeAdversarialBoundary(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		expectedErr string
	}{
		{
			name:        "empty command string",
			command:     "",
			expectedErr: "recipe command cannot be empty",
		},
		{
			name:        "whitespace only command string",
			command:     "    \t\n  ",
			expectedErr: "recipe command cannot be empty",
		},
		{
			name:        "bare double dash flag",
			command:     "zqk system check -- OBJ-123",
			expectedErr: "invalid bare flag '--' in recipe command",
		},
		{
			name:        "triple dash flag prefix",
			command:     "zqk system check ---auto-fix OBJ-123",
			expectedErr: "malformed flag prefix \"---auto-fix\"",
		},
		{
			name:        "malformed flag equals argument",
			command:     "zqk system check --=auto-fix OBJ-123",
			expectedErr: "malformed flag argument \"--=auto-fix\"",
		},
		{
			name:        "unsanitized --ids flag",
			command:     "zqk system check --ids OBJ-123",
			expectedErr: "recipe command contains invalid flag '--ids': positional ID arguments must be used",
		},
		{
			name:        "unsanitized --ids= flag",
			command:     "zqk object update --ids=OBJ-123",
			expectedErr: "recipe command contains invalid flag '--ids': positional ID arguments must be used",
		},
		{
			name:        "flag with empty argument assignment",
			command:     "zqk object update OBJ-123 --field=",
			expectedErr: "flag \"--field\" requires a non-empty argument",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := system.ValidateRecipeCommand(tc.command)
			require.Error(t, err, "expected validation failure for command: %q", tc.command)
			assert.Contains(t, err.Error(), tc.expectedErr)
		})
	}

	// In addition, verify that applyPlan returns the validation error when given a plan with an invalid command
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	invalidPlan := system.RemedyPlan{
		ID:         "REMEDY-INVALID-CMD",
		Title:      "Test Invalid Command Boundary",
		ActionType: system.ActionRunCommand,
		Target:     "OBJ-999",
		Command:    "zqk system check --ids OBJ-999",
		Confidence: 1.0,
		AutoApply:  true,
	}

	report, err := engine.Apply(context.Background(), []system.RemedyPlan{invalidPlan})
	require.NoError(t, err)
	require.Equal(t, 1, len(report.Outcomes))
	assert.False(t, report.Outcomes[0].Applied)
	assert.Contains(t, report.Outcomes[0].Error, "invalid remedy command: recipe command contains invalid flag '--ids'")
}

// TestAutoRemedy_GeneratedRecipes_OperationalDynamicProof satisfies CRIT-1793961675960953000-217f5c81:
// Operational Dynamic Proof: Verify system check auto-remedy recipes output valid CLI flags and commands that execute cleanly.
func TestAutoRemedy_GeneratedRecipes_OperationalDynamicProof(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	mockResults := []systemcheck.CheckResult{
		{
			ObjectID: "BLI-101",
			Issues: []systemcheck.Issue{
				{
					Message:    "Precondition missing estimated_effort",
					FixCommand: "zqk object update --ids BLI-101 --field estimated_effort=1d",
				},
				{
					Message:     "Integrity hash mismatch",
					AutoFixable: true,
				},
			},
		},
		{
			ObjectID: "REQ-202",
			Issues: []systemcheck.Issue{
				{
					Message:    "Orphaned requirement needing deletion",
					FixCommand: "zqk object delete --ids=REQ-202 --unlink-references",
				},
				{
					Message:    "Status transition needed",
					FixCommand: "zqk object promote --ids REQ-202",
				},
			},
		},
	}

	plans, err := engine.Diagnose(context.Background(), mockResults)
	require.NoError(t, err)
	require.True(t, len(plans) >= 4, "expected at least 4 recipe plans, got %d", len(plans))

	for _, p := range plans {
		if p.ActionType == system.ActionRunCommand {
			assert.NotEmpty(t, p.Command, "recipe command should not be empty")

			// 1. Must pass ValidateRecipeCommand
			err := system.ValidateRecipeCommand(p.Command)
			assert.NoError(t, err, "plan %s command %q must pass ValidateRecipeCommand", p.ID, p.Command)

			// 2. Must not contain --ids or --ids=
			assert.False(t, strings.Contains(p.Command, "--ids "), "plan %s command %q should not contain '--ids '", p.ID, p.Command)
			assert.False(t, strings.Contains(p.Command, "--ids="), "plan %s command %q should not contain '--ids='", p.ID, p.Command)

			// 3. Must retain the target object ID as a positional parameter
			assert.Contains(t, p.Command, p.Target, "plan %s command %q must contain target ID %s", p.ID, p.Command, p.Target)
		}
	}
}

// TestAutoRemedy_StaticFloor_SafeCommandRecognitionAndFlagParity satisfies CRIT-1793982022958714000-6de30dc2:
// Acceptance: DiagnosticsRemedyEngine recognizes safe command remedy actions and retains recipe command structure without invalid flags.
func TestAutoRemedy_StaticFloor_SafeCommandRecognitionAndFlagParity(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	safeCommands := []string{
		"zqk system check --auto-fix OBJ-100",
		"zqk system check OBJ-200 --auto-fix",
		"zqk system init --force",
		"zqk system agent-onboard",
		"zqk system cleanup-duplicates",
		"zqk system cleanup-duplicates --hash-duplicates",
		"zqk object update BLI-300 --field estimated_effort=2h",
		"zqk object update REQ-400 --field title=\"Updated Feature Spec\"",
	}

	for _, cmd := range safeCommands {
		t.Run("safe_cmd_"+cmd, func(t *testing.T) {
			assert.True(t, system.IsSafeRemedyCommand(cmd, tmpDir), "command %q should be recognized as safe", cmd)
			err := engine.ValidateSafeCommand(cmd)
			assert.NoError(t, err, "ValidateSafeCommand should accept %q", cmd)
		})
	}

	// Verify additional recipe sanitization edge cases (GH-937, GH-940)
	sanitizationCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bare --ids at beginning",
			input:    "--ids OBJ-123",
			expected: "OBJ-123",
		},
		{
			name:     "bare --ids= at beginning",
			input:    "--ids=OBJ-123",
			expected: "OBJ-123",
		},
		{
			name:     "only --ids",
			input:    "--ids",
			expected: "",
		},
		{
			name:     "only --ids=",
			input:    "--ids=",
			expected: "",
		},
		{
			name:     "empty quotes in middle",
			input:    "zqk system check --auto-fix --ids=\"\" OBJ-123",
			expected: "zqk system check --auto-fix OBJ-123",
		},
		{
			name:     "empty single quotes in middle",
			input:    "zqk system check --auto-fix --ids='' OBJ-123",
			expected: "zqk system check --auto-fix OBJ-123",
		},
		{
			name:     "bare --ids followed by flag",
			input:    "zqk system check --ids --auto-fix OBJ-123",
			expected: "zqk system check --auto-fix OBJ-123",
		},
		{
			name:     "bare --ids= followed by flag",
			input:    "zqk system check --ids= --auto-fix OBJ-123",
			expected: "zqk system check --auto-fix OBJ-123",
		},
		{
			name:     "trailing --ids=",
			input:    "zqk object delete OBJ-123 --ids=",
			expected: "zqk object delete OBJ-123",
		},
	}

	for _, tc := range sanitizationCases {
		t.Run(tc.name, func(t *testing.T) {
			got := system.SanitizeRecipeCommand(tc.input)
			assert.Equal(t, tc.expected, got)
			if got != "" {
				err := system.ValidateRecipeCommand(got)
				assert.NoError(t, err, "sanitized command %q must pass ValidateRecipeCommand", got)
			}
		})
	}
}

// TestAutoRemedy_OperationalProof_AutoFixCommandExecution satisfies CRIT-1793982039960170000-70b46b5b:
// Functional: Verify auto-remedy safely applies internal auto-fix commands and successfully clears recoverable issues.
func TestAutoRemedy_OperationalProof_AutoFixCommandExecution(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	var executedCommands []string
	engine.CommandRunner = func(ctx context.Context, cmd string) (string, error) {
		executedCommands = append(executedCommands, cmd)
		return "applied auto-fix successfully for " + cmd, nil
	}

	mockResults := []systemcheck.CheckResult{
		{
			ObjectID: "BLI-777",
			Issues: []systemcheck.Issue{
				{
					Message:     "Hash mismatch on BLI-777",
					AutoFixable: true,
				},
			},
		},
		{
			ObjectID: "REQ-888",
			Issues: []systemcheck.Issue{
				{
					Message:     "Integrity drift on REQ-888",
					AutoFixable: true,
				},
			},
		},
	}

	plans, err := engine.Diagnose(context.Background(), mockResults)
	require.NoError(t, err)
	require.NotEmpty(t, plans)

	// Filter down to the auto-fix plans
	var autoFixPlans []system.RemedyPlan
	for _, p := range plans {
		if strings.HasPrefix(p.ID, "REMEDY-AUTOFIX-") {
			autoFixPlans = append(autoFixPlans, p)
		}
	}
	require.Equal(t, 2, len(autoFixPlans), "expected 2 auto-fix plans")

	report, err := engine.Apply(context.Background(), autoFixPlans)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, 2, report.TotalDiagnosed)
	assert.Equal(t, 2, report.TotalApplied, "both auto-fix plans should be safely executed and applied")

	for _, outcome := range report.Outcomes {
		assert.True(t, outcome.Applied, "outcome for %s should be applied", outcome.Plan.ID)
		assert.Empty(t, outcome.Error, "outcome for %s should have no error", outcome.Plan.ID)
		assert.Contains(t, outcome.Detail, "applied auto-fix successfully")
	}

	require.Equal(t, 2, len(executedCommands))
	assert.Contains(t, executedCommands[0], "system check --auto-fix BLI-777")
	assert.Contains(t, executedCommands[1], "system check --auto-fix REQ-888")

	// Also verify runner error handling: if CommandRunner returns an error, outcome records failure
	failEngine := system.NewDiagnosticsRemedyEngine(tmpDir)
	failEngine.CommandRunner = func(ctx context.Context, cmd string) (string, error) {
		return "", fmt.Errorf("lock timeout during auto-fix execution")
	}

	failReport, err := failEngine.Apply(context.Background(), autoFixPlans[:1])
	require.NoError(t, err)
	assert.Equal(t, 0, failReport.TotalApplied)
	require.Len(t, failReport.Outcomes, 1)
	assert.False(t, failReport.Outcomes[0].Applied)
	assert.Contains(t, failReport.Outcomes[0].Error, "command execution failed: lock timeout during auto-fix execution")
}

// TestAutoRemedy_NegativeBoundary_UnsafeRecipeRejection satisfies CRIT-1793982042724177000-c4858ffb:
// Security: Reject auto-applying non-whitelisted, unsafe, or destructive commands outside project boundary.
func TestAutoRemedy_NegativeBoundary_UnsafeRecipeRejection(t *testing.T) {
	tmpDir := t.TempDir()
	engine := system.NewDiagnosticsRemedyEngine(tmpDir)

	runnerInvoked := false
	engine.CommandRunner = func(ctx context.Context, cmd string) (string, error) {
		runnerInvoked = true
		return "should not be called", nil
	}

	unsafePlans := []struct {
		name        string
		command     string
		errContains string
	}{
		{
			name:        "non-whitelisted executable rm",
			command:     "rm -rf /",
			errContains: "executable \"rm\" is not whitelisted",
		},
		{
			name:        "non-whitelisted executable curl",
			command:     "curl https://malicious.example.com",
			errContains: "executable \"curl\" is not whitelisted",
		},
		{
			name:        "non-whitelisted executable bash",
			command:     "bash -c 'echo pwned'",
			errContains: "executable \"bash\" is not whitelisted",
		},
		{
			name:        "destructive object delete command",
			command:     "zqk object delete OBJ-123 --unlink-references",
			errContains: "destructive command 'object delete' cannot be auto-applied",
		},
		{
			name:        "destructive object purge command",
			command:     "zqk object purge OBJ-456",
			errContains: "destructive command 'object purge' cannot be auto-applied",
		},
		{
			name:        "non-whitelisted system subcommand",
			command:     "zqk system purge-database",
			errContains: "system subcommand \"purge-database\" is not whitelisted",
		},
		{
			name:        "shell injection semicolon chaining",
			command:     "zqk system check --auto-fix OBJ-1; rm -rf /",
			errContains: "unsafe shell metacharacter \";\"",
		},
		{
			name:        "shell injection background operator",
			command:     "zqk system check --auto-fix OBJ-1 &",
			errContains: "unsafe shell metacharacter \"&\"",
		},
		{
			name:        "shell injection pipe",
			command:     "zqk system check --auto-fix OBJ-1 | sh",
			errContains: "unsafe shell metacharacter \"|\"",
		},
		{
			name:        "shell injection redirect",
			command:     "zqk system check --auto-fix OBJ-1 > /tmp/out",
			errContains: "unsafe shell metacharacter \">\"",
		},
		{
			name:        "shell injection command substitution",
			command:     "zqk system check --auto-fix OBJ-1 $(whoami)",
			errContains: "unsafe shell metacharacter \"$\"",
		},
		{
			name:        "path traversal outside project boundary",
			command:     "zqk system check --auto-fix ../../../secret",
			errContains: "path traversal '..' outside project boundary",
		},
		{
			name:        "absolute path outside project boundary",
			command:     "zqk system check --auto-fix /etc/shadow",
			errContains: "references path outside project boundary",
		},
	}

	for _, tc := range unsafePlans {
		t.Run(tc.name, func(t *testing.T) {
			runnerInvoked = false

			plan := system.RemedyPlan{
				ID:         "REMEDY-UNSAFE-TEST",
				Title:      "Unsafe Test Plan",
				ActionType: system.ActionRunCommand,
				Target:     "OBJ-UNSAFE",
				Command:    tc.command,
				Confidence: 1.0,
				AutoApply:  true, // Caller attempted to force auto-apply
			}

			report, err := engine.Apply(context.Background(), []system.RemedyPlan{plan})
			require.NoError(t, err)
			assert.Equal(t, 0, report.TotalApplied, "unsafe plan must never be applied")
			require.Len(t, report.Outcomes, 1)
			assert.False(t, report.Outcomes[0].Applied, "outcome must be applied=false")
			assert.Contains(t, report.Outcomes[0].Error, tc.errContains, "outcome error should describe rejection reason")
			assert.False(t, runnerInvoked, "CommandRunner must never be invoked for unsafe command")
		})
	}
}
