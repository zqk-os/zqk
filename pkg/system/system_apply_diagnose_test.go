// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package system

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSystem_NewDiagnosticsRemedyEngine(t *testing.T) {
	root := t.TempDir()
	engine := NewDiagnosticsRemedyEngine(root)
	require.NotNil(t, engine)
	assert.Equal(t, root, engine.ProjectRoot)
	assert.Nil(t, engine.PolicySeeder)
	assert.Nil(t, engine.PersonaSeeder)
}

func TestSystem_Diagnose_EmptyProjectRoot(t *testing.T) {
	engine := NewDiagnosticsRemedyEngine("")
	plans, err := engine.Diagnose(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, plans)
}

func TestSystem_Diagnose_RelativeIOResources(t *testing.T) {
	cwd, err := fileutil.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	relDir, err := filepath.Rel(cwd, tmpDir)
	require.NoError(t, err)

	zqkDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	err = fileutil.MkdirAll(zqkDir, paths.DirPerm755)
	require.NoError(t, err)

	staleLock := filepath.Join(zqkDir, "relative_test.lock")
	err = fileutil.WriteFile(staleLock, []byte("lock"), paths.FilePerm644)
	require.NoError(t, err)
	oldTime := time.Now().Add(-20 * time.Minute)
	err = fileutil.Chtimes(staleLock, oldTime, oldTime)
	require.NoError(t, err)

	orphanTmp := filepath.Join(zqkDir, "relative_test.tmp")
	err = fileutil.WriteFile(orphanTmp, []byte("tmp"), paths.FilePerm644)
	require.NoError(t, err)
	veryOldTime := time.Now().Add(-45 * time.Minute)
	err = fileutil.Chtimes(orphanTmp, veryOldTime, veryOldTime)
	require.NoError(t, err)

	engine := NewDiagnosticsRemedyEngine(relDir)
	plans, err := engine.Diagnose(context.Background(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, plans)
}

func TestSystem_Diagnose_CheckIssues_EdgeCases(t *testing.T) {
	root := t.TempDir()
	engine := NewDiagnosticsRemedyEngine(root)

	issues := []systemcheck.CheckResult{
		{
			ObjectID: "OBJ-FIX-CMD",
			Issues: []systemcheck.Issue{
				{Message: "cmd issue", FixCommand: "fix-it"},
			},
		},
		{
			ObjectID: "OBJ-AUTO-FIX",
			Issues: []systemcheck.Issue{
				{Message: "autofix issue", AutoFixable: true},
			},
		},
		{
			ObjectID: "OBJ-IGNORED",
			Issues: []systemcheck.Issue{
				{Message: "info only", AutoFixable: false, FixCommand: ""},
			},
		},
	}

	plans, err := engine.Diagnose(context.Background(), issues)
	require.NoError(t, err)
	require.Len(t, plans, 4) // 2 from check issues + 2 unseeded preconditions

	var fixCmdPlan, autoFixPlan *RemedyPlan
	for i := range plans {
		p := &plans[i]
		if p.ID == "REMEDY-ISSUE-OBJ-FIX-CMD" {
			fixCmdPlan = p
		}
		if p.ID == "REMEDY-AUTOFIX-OBJ-AUTO-FIX" {
			autoFixPlan = p
		}
	}

	require.NotNil(t, fixCmdPlan)
	assert.Equal(t, "fix-it", fixCmdPlan.Command)
	assert.False(t, fixCmdPlan.AutoApply)

	require.NotNil(t, autoFixPlan)
	assert.Contains(t, autoFixPlan.Command, "system check --auto-fix --ids OBJ-AUTO-FIX")
	assert.False(t, autoFixPlan.AutoApply)
}

func TestSystem_DirHasYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent directory
	assert.False(t, dirHasYAML(filepath.Join(tmpDir, "non_existent")))

	// Empty directory
	emptyDir := filepath.Join(tmpDir, "empty")
	err := fileutil.MkdirAll(emptyDir, paths.DirPerm755)
	require.NoError(t, err)
	assert.False(t, dirHasYAML(emptyDir))

	// Subdirectory ending in .yaml should be ignored
	subDirYaml := filepath.Join(emptyDir, "nested.yaml")
	err = fileutil.MkdirAll(subDirYaml, paths.DirPerm755)
	require.NoError(t, err)
	assert.False(t, dirHasYAML(emptyDir))

	// Directory with only txt file
	txtFile := filepath.Join(emptyDir, "notes.txt")
	err = fileutil.WriteFile(txtFile, []byte("plain text"), paths.FilePerm644)
	require.NoError(t, err)
	assert.False(t, dirHasYAML(emptyDir))

	// Directory with .yaml file
	yamlDir := filepath.Join(tmpDir, "yamldir")
	err = fileutil.MkdirAll(yamlDir, paths.DirPerm755)
	require.NoError(t, err)
	yamlFile := filepath.Join(yamlDir, "config.yaml")
	err = fileutil.WriteFile(yamlFile, []byte("key: value"), paths.FilePerm644)
	require.NoError(t, err)
	assert.True(t, dirHasYAML(yamlDir))

	// Directory with .yml file
	ymlDir := filepath.Join(tmpDir, "ymldir")
	err = fileutil.MkdirAll(ymlDir, paths.DirPerm755)
	require.NoError(t, err)
	ymlFile := filepath.Join(ymlDir, "config.yml")
	err = fileutil.WriteFile(ymlFile, []byte("key: value"), paths.FilePerm644)
	require.NoError(t, err)
	assert.True(t, dirHasYAML(ymlDir))
}

func TestSystem_Diagnose_SeedingPreconditions_Variants(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := filepath.Join(tmpDir, paths.ProcessDir)

	// Create singular "policy" directory with .yml file
	policyDir := filepath.Join(processDir, "policy")
	err := fileutil.MkdirAll(policyDir, paths.DirPerm755)
	require.NoError(t, err)
	err = fileutil.WriteFile(filepath.Join(policyDir, "rule.yml"), []byte("policy: true"), paths.FilePerm644)
	require.NoError(t, err)

	// Create singular "persona" directory with .yaml file
	personaDir := filepath.Join(processDir, "persona")
	err = fileutil.MkdirAll(personaDir, paths.DirPerm755)
	require.NoError(t, err)
	err = fileutil.WriteFile(filepath.Join(personaDir, "lead.yaml"), []byte("persona: true"), paths.FilePerm644)
	require.NoError(t, err)

	engine := NewDiagnosticsRemedyEngine(tmpDir)
	plans := engine.diagnoseSeedingPreconditions()
	assert.Empty(t, plans)
}

func TestSystem_Apply_AutoApplyFalse(t *testing.T) {
	engine := NewDiagnosticsRemedyEngine(t.TempDir())
	plans := []RemedyPlan{
		{
			ID:         "MANUAL-PLAN-1",
			Title:      "Manual Intervention",
			ActionType: ActionRunCommand,
			AutoApply:  false,
		},
	}

	report, err := engine.Apply(context.Background(), plans)
	require.NoError(t, err)
	assert.Equal(t, 1, report.TotalDiagnosed)
	assert.Equal(t, 0, report.TotalApplied)
	require.Len(t, report.Outcomes, 1)
	assert.False(t, report.Outcomes[0].Applied)
	assert.Equal(t, msgManualRequired, report.Outcomes[0].Detail)
}

func TestSystem_ApplyPlan_ActionRunCommandAndUnknown(t *testing.T) {
	root := t.TempDir()
	engine := NewDiagnosticsRemedyEngine(root)
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	err := fileutil.MkdirAll(zqkDir, paths.DirPerm755)
	require.NoError(t, err)

	cmdPlan := RemedyPlan{
		ID:         "PLAN-CMD",
		ActionType: ActionRunCommand,
		AutoApply:  true,
	}
	resCmd := engine.applyPlan(cmdPlan)
	assert.False(t, resCmd.Applied)
	assert.Equal(t, msgCommandRequires, resCmd.Error)

	touchPlan := RemedyPlan{
		ID:         "PLAN-TOUCH",
		Target:     filepath.Join(zqkDir, "applied.touch"),
		ActionType: ActionTouchFile,
		AutoApply:  true,
	}
	resTouch := engine.applyPlan(touchPlan)
	assert.True(t, resTouch.Applied)
	assert.Equal(t, "touched applied.touch", resTouch.Detail)

	unknownPlan := RemedyPlan{
		ID:         "PLAN-UNKNOWN",
		ActionType: RemedyActionType("invalid_type"),
		AutoApply:  true,
	}
	resUnknown := engine.applyPlan(unknownPlan)
	assert.False(t, resUnknown.Applied)
	assert.Equal(t, "unrecognized action type", resUnknown.Error)
}

func TestSystem_ApplyRemoveFile_EdgeCases(t *testing.T) {
	root := t.TempDir()
	engine := NewDiagnosticsRemedyEngine(root)
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	err := fileutil.MkdirAll(zqkDir, paths.DirPerm755)
	require.NoError(t, err)

	// Target outside .zqk directory
	outsidePlan := RemedyPlan{
		Target:     filepath.Join(root, "outside.txt"),
		ActionType: ActionRemoveFile,
	}
	resOutside := engine.applyRemoveFile(outsidePlan)
	assert.False(t, resOutside.Applied)
	assert.Equal(t, errTargetOutsideZQK, resOutside.Error)

	// Refuse daemon lock
	daemonLockPath := filepath.Join(zqkDir, "state", "daemon_locks", "svc.lock")
	err = fileutil.MkdirAll(filepath.Dir(daemonLockPath), paths.DirPerm755)
	require.NoError(t, err)
	err = fileutil.WriteFile(daemonLockPath, []byte("pid"), paths.FilePerm644)
	require.NoError(t, err)

	lockPlan := RemedyPlan{
		Target:     daemonLockPath,
		ActionType: ActionRemoveFile,
	}
	resLock := engine.applyRemoveFile(lockPlan)
	assert.False(t, resLock.Applied)
	assert.Equal(t, msgRefuseDaemonLock, resLock.Detail)

	// Non-empty directory removal failure (fileutil.Remove fails on non-empty dir)
	nonEmptySubdir := filepath.Join(zqkDir, "non_empty_dir")
	err = fileutil.MkdirAll(nonEmptySubdir, paths.DirPerm755)
	require.NoError(t, err)
	err = fileutil.WriteFile(filepath.Join(nonEmptySubdir, "child.txt"), []byte("sub"), paths.FilePerm644)
	require.NoError(t, err)

	dirPlan := RemedyPlan{
		Target:     nonEmptySubdir,
		ActionType: ActionRemoveFile,
	}
	resDir := engine.applyRemoveFile(dirPlan)
	assert.False(t, resDir.Applied)
	assert.NotEmpty(t, resDir.Error)

	// Remove non-existent file inside .zqk (IsNotExist treated as success)
	missingPlan := RemedyPlan{
		Target:     filepath.Join(zqkDir, "already_gone.tmp"),
		ActionType: ActionRemoveFile,
	}
	resMissing := engine.applyRemoveFile(missingPlan)
	assert.True(t, resMissing.Applied)
	assert.Contains(t, resMissing.Detail, "removed already_gone.tmp")
}

func TestSystem_ApplyTouchFile_EdgeCases(t *testing.T) {
	root := t.TempDir()
	engine := NewDiagnosticsRemedyEngine(root)
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	err := fileutil.MkdirAll(zqkDir, paths.DirPerm755)
	require.NoError(t, err)

	// Target outside .zqk directory
	outsidePlan := RemedyPlan{
		Target:     filepath.Join(root, "outside.txt"),
		ActionType: ActionTouchFile,
	}
	resOutside := engine.applyTouchFile(outsidePlan)
	assert.False(t, resOutside.Applied)
	assert.Equal(t, errTargetOutsideZQK, resOutside.Error)

	// Target inside .zqk with non-existent parent directory
	missingParentPlan := RemedyPlan{
		Target:     filepath.Join(zqkDir, "nested_dir_missing", "touch.tmp"),
		ActionType: ActionTouchFile,
	}
	resMissing := engine.applyTouchFile(missingParentPlan)
	assert.False(t, resMissing.Applied)
	assert.NotEmpty(t, resMissing.Error)

	// Valid target inside .zqk
	validTarget := filepath.Join(zqkDir, "heartbeat.touch")
	validPlan := RemedyPlan{
		Target:     validTarget,
		ActionType: ActionTouchFile,
	}
	resValid := engine.applyTouchFile(validPlan)
	assert.True(t, resValid.Applied)
	assert.Equal(t, "touched heartbeat.touch", resValid.Detail)

	st, err := fileutil.Stat(validTarget)
	require.NoError(t, err)
	assert.False(t, st.IsDir())
}

func TestSystem_ApplySeed_AllBranches(t *testing.T) {
	root := t.TempDir()
	engine := NewDiagnosticsRemedyEngine(root)

	// Target policies with nil seeder
	polPlan := RemedyPlan{Target: targetPolicies, ActionType: ActionSeed}
	resPolNil := engine.applySeed(polPlan)
	assert.False(t, resPolNil.Applied)
	assert.Equal(t, msgNoSeeder, resPolNil.Detail)

	// Target policies with error
	engine.PolicySeeder = func(r string) (int, error) {
		return 0, errors.New("policy seed failed")
	}
	resPolErr := engine.applySeed(polPlan)
	assert.False(t, resPolErr.Applied)
	assert.Equal(t, "policy seed failed", resPolErr.Error)

	// Target policies with success
	engine.PolicySeeder = func(r string) (int, error) {
		return 7, nil
	}
	resPolOk := engine.applySeed(polPlan)
	assert.True(t, resPolOk.Applied)
	assert.Equal(t, "seeded 7 policies", resPolOk.Detail)

	// Target personas with nil seeder
	personaPlan := RemedyPlan{Target: targetPersonas, ActionType: ActionSeed}
	resPerNil := engine.applySeed(personaPlan)
	assert.False(t, resPerNil.Applied)
	assert.Equal(t, msgNoSeeder, resPerNil.Detail)

	// Target personas with error
	engine.PersonaSeeder = func(r string) (int, error) {
		return 0, errors.New("persona seed failed")
	}
	resPerErr := engine.applySeed(personaPlan)
	assert.False(t, resPerErr.Applied)
	assert.Equal(t, "persona seed failed", resPerErr.Error)

	// Target personas with success
	engine.PersonaSeeder = func(r string) (int, error) {
		return 4, nil
	}
	resPerOk := engine.applySeed(personaPlan)
	assert.True(t, resPerOk.Applied)
	assert.Equal(t, "seeded 4 personas", resPerOk.Detail)

	// Unknown seed target
	unknownPlan := RemedyPlan{Target: "unknown_target", ActionType: ActionSeed}
	resUnknown := engine.applySeed(unknownPlan)
	assert.False(t, resUnknown.Applied)
	assert.Equal(t, msgNoSeeder, resUnknown.Detail)
}

func TestSystem_ConcurrencyWithGoroutineLabels(t *testing.T) {
	t.Parallel()

	const numWorkers = 8
	const numIterations = 20

	var totalSuccess atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("test_system_concurrency", "concurrent diagnosis and remediation")
		builder.StartSimple(func() {
			defer wg.Done()

			root := t.TempDir()
			zqkDir := filepath.Join(root, paths.ProjectDataDir)
			mkdirErr := fileutil.MkdirAll(zqkDir, paths.DirPerm755)
			if mkdirErr != nil {
				t.Errorf("worker %d mkdir failed: %v", workerID, mkdirErr)
				return
			}

			eng := NewDiagnosticsRemedyEngine(root)
			eng.PolicySeeder = func(r string) (int, error) { return 2, nil }
			eng.PersonaSeeder = func(r string) (int, error) { return 3, nil }

			for j := 0; j < numIterations; j++ {
				plans, diagErr := eng.Diagnose(context.Background(), nil)
				if diagErr != nil {
					t.Errorf("worker %d iter %d diagnose failed: %v", workerID, j, diagErr)
					return
				}

				report, appErr := eng.Apply(context.Background(), plans)
				if appErr != nil {
					t.Errorf("worker %d iter %d apply failed: %v", workerID, j, appErr)
					return
				}

				if report.TotalApplied > 0 {
					totalSuccess.Add(int64(report.TotalApplied))
				}
			}
		})
	}

	wg.Wait()
	assert.Greater(t, totalSuccess.Load(), int64(0))
}
