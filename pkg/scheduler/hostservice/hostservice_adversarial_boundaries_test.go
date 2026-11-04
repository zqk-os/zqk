package hostservice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupAdversarialHome(t *testing.T) string {
	t.Helper()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("ZQK_DISABLE_HOSTSERVICE_MUTATION", "1")
	return tempHome
}

func createAdversarialExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	binPath := filepath.Join(dir, name)
	err := fileutil.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755)
	require.NoError(t, err)
	return binPath
}

func createAdversarialPIDFile(t *testing.T, filePath, content string) {
	t.Helper()
	err := fileutil.MkdirAll(filepath.Dir(filePath), paths.DirPerm755)
	require.NoError(t, err)
	err = fileutil.WriteFile(filePath, []byte(content), paths.FilePerm644)
	require.NoError(t, err)
}

func TestHostServiceCycle32_Adversarial_InstallRoot_NonExistentAndFile(t *testing.T) {
	setupAdversarialHome(t)
	nonExistent := filepath.Join(t.TempDir(), "does_not_exist_xyz")
	_, err := InstallRoot(nonExistent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root must be an existing directory")

	tempDir := t.TempDir()
	regularFile := filepath.Join(tempDir, "regular_file.txt")
	writeErr := fileutil.WriteFile(regularFile, []byte("hello"), paths.FilePerm644)
	require.NoError(t, writeErr)

	_, errFile := InstallRoot(regularFile)
	require.Error(t, errFile)
	assert.Contains(t, errFile.Error(), "project root must be an existing directory")
}

func TestHostServiceCycle32_Adversarial_UninstallRoot_MissingEntry(t *testing.T) {
	setupAdversarialHome(t)
	err := UninstallRoot("non-existent-root-id-999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no host service registry entry for")

	errPath := UninstallRoot("/non/existent/abs/path")
	require.Error(t, errPath)
	assert.Contains(t, errPath.Error(), "no host service registry entry for")
}

func TestHostServiceCycle32_Adversarial_ResolveEntry_FailClosed(t *testing.T) {
	setupAdversarialHome(t)
	_, errID := ResolveEntry("unknown-id-888")
	require.Error(t, errID)
	assert.Contains(t, errID.Error(), "no host service registry entry for")

	_, errPath := ResolveEntry("/some/unregistered/abs/path")
	require.Error(t, errPath)
	assert.Contains(t, errPath.Error(), "no host service registry entry for")
}

func TestHostServiceCycle32_Adversarial_Rebind_MissingRoot(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()
	_, err := Rebind("missing-root-id-777", tempDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in host service registry")
}

func TestHostServiceCycle32_Adversarial_DesiredState_AdversarialInputs(t *testing.T) {
	setupAdversarialHome(t)
	adversarialInvalid := []string{"", "   ", "RUNNING", "active", "UNKNOWN", "invalid-state", "foo-bar"}
	for _, val := range adversarialInvalid {
		normalized := NormalizeDesiredState(val)
		assert.Empty(t, normalized)
		_, err := SetEntryDesiredState("some-id", val)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid desired_state")
	}

	validInputs := map[string]string{
		"enabled":     DesiredStateEnabled,
		"  ENABLED  ": DesiredStateEnabled,
		"disabled":    DesiredStateDisabled,
		" DiSaBlEd ":  DesiredStateDisabled,
		"absent":      DesiredStateAbsent,
		"  AbSeNt  ":  DesiredStateAbsent,
	}
	for input, expected := range validInputs {
		assert.Equal(t, expected, NormalizeDesiredState(input))
	}

	_, missingErr := SetEntryDesiredState("missing-id-123", DesiredStateEnabled)
	require.Error(t, missingErr)
	assert.Contains(t, missingErr.Error(), "not in host service registry")
}

func TestHostServiceCycle32_Adversarial_SetEntryDesiredState_Success(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()
	rootID := RootID(tempDir)
	entry := Entry{
		RootID:       rootID,
		AbsRoot:      tempDir,
		UnitLabel:    UnitLabel(rootID),
		InstalledAt:  time.Now().UTC(),
		DesiredState: DesiredStateEnabled,
	}
	reg := &Registry{Entries: []Entry{entry}}
	saveErr := SaveRegistry(reg)
	require.NoError(t, saveErr)

	updated, err := SetEntryDesiredState(rootID, DesiredStateDisabled)
	require.NoError(t, err)
	assert.Equal(t, DesiredStateDisabled, updated.DesiredState)

	loaded, loadErr := LoadRegistry()
	require.NoError(t, loadErr)
	persisted, found := loaded.FindByRootID(rootID)
	assert.True(t, found)
	assert.Equal(t, DesiredStateDisabled, persisted.DesiredState)
}

func TestHostServiceCycle32_Adversarial_Registry_UpsertCollisionAndRemoval(t *testing.T) {
	reg := &Registry{}
	_, ok1 := reg.FindByRootID("none")
	assert.False(t, ok1)
	_, ok2 := reg.FindByAbsRoot("/none")
	assert.False(t, ok2)

	entryA := Entry{RootID: "dup-id", AbsRoot: "/root/a", DesiredState: DesiredStateEnabled}
	entryB := Entry{RootID: "dup-id", AbsRoot: "/root/b", DesiredState: DesiredStateDisabled}

	reg.UpsertEntry(entryA)
	assert.Len(t, reg.Entries, 1)

	reg.UpsertEntry(entryB)
	assert.Len(t, reg.Entries, 1)
	found, exists := reg.FindByRootID("dup-id")
	assert.True(t, exists)
	assert.Equal(t, "/root/b", found.AbsRoot)
	assert.Equal(t, DesiredStateDisabled, found.DesiredState)

	reg.RemoveEntry("non-existent-id")
	assert.Len(t, reg.Entries, 1)

	reg.RemoveEntry("dup-id")
	assert.Empty(t, reg.Entries)
}

func TestHostServiceCycle32_Adversarial_RoleSymlink_FailClosedInputs(t *testing.T) {
	tempDir := t.TempDir()
	_, errEmptyRoot := EnsureServiceRoleSymlink("", "sched", "/path/to/bin")
	require.Error(t, errEmptyRoot)
	assert.Contains(t, errEmptyRoot.Error(), "project root required")

	_, errUnknownRole := EnsureServiceRoleSymlink(tempDir, "", "/path/to/bin")
	require.Error(t, errUnknownRole)
	assert.Contains(t, errUnknownRole.Error(), "unknown role")

	_, errNonExistentBin := EnsureServiceRoleSymlink(tempDir, "sched", "/non/existent/bin/path")
	require.Error(t, errNonExistentBin)
	assert.Contains(t, errNonExistentBin.Error(), "not a regular file")

	dirAsBin := filepath.Join(tempDir, "dirAsBin")
	mkdirErr := fileutil.MkdirAll(dirAsBin, paths.DirPerm755)
	require.NoError(t, mkdirErr)

	_, errDirTarget := EnsureServiceRoleSymlink(tempDir, "sched", dirAsBin)
	require.Error(t, errDirTarget)
	assert.Contains(t, errDirTarget.Error(), "not a regular file")

	errAllEmpty := EnsureAllServiceRoleSymlinks("", "/path/to/bin")
	require.Error(t, errAllEmpty)
	assert.Contains(t, errAllEmpty.Error(), "project root required")

	errAllDir := EnsureAllServiceRoleSymlinks(tempDir, dirAsBin)
	require.Error(t, errAllDir)
	assert.Contains(t, errAllDir.Error(), "not a regular file")
}

func TestHostServiceCycle32_Adversarial_PlatformAdapters_UnsupportedAndWindows(t *testing.T) {
	unsupported := UnsupportedAdapter{GOOS: "solaris"}
	entry := Entry{UnitLabel: "com.test.unit"}

	assert.Error(t, unsupported.Install(entry))
	assert.Error(t, unsupported.Uninstall(entry))
	assert.Error(t, unsupported.Start(entry))
	assert.Error(t, unsupported.Stop(entry))
	st, stErr := unsupported.Status(entry)
	assert.Empty(t, st)
	assert.Error(t, stErr)

	win := WindowsStubAdapter{}
	assert.Error(t, win.Install(entry))
	assert.Error(t, win.Uninstall(entry))
	assert.Error(t, win.Start(entry))
	assert.Error(t, win.Stop(entry))
	winSt, winStErr := win.Status(entry)
	assert.Equal(t, "unsupported", winSt)
	assert.Error(t, winStErr)
}

func TestHostServiceCycle32_Adversarial_ReadPidFile_CorruptedAndStale(t *testing.T) {
	tempDir := t.TempDir()
	pid, ok := readPidFile(filepath.Join(tempDir, "missing.pid"))
	assert.Equal(t, 0, pid)
	assert.False(t, ok)

	corruptCases := map[string]string{
		"empty.pid":      "",
		"whitespace.pid": "   \n\t",
		"alpha.pid":      "not-a-pid\n",
		"negative.pid":   "-42\n",
		"zero.pid":       "0\n",
	}
	for name, content := range corruptCases {
		p := filepath.Join(tempDir, name)
		createAdversarialPIDFile(t, p, content)
		resPID, resOK := readPidFile(p)
		assert.Equal(t, 0, resPID)
		assert.False(t, resOK)
	}

	deadPidFile := filepath.Join(tempDir, "dead.pid")
	createAdversarialPIDFile(t, deadPidFile, "999999999\n")
	deadPID, deadOK := readPidFile(deadPidFile)
	assert.Equal(t, 999999999, deadPID)
	assert.False(t, deadOK)

	livePidFile := filepath.Join(tempDir, "live.pid")
	currentPID := os.Getpid()
	createAdversarialPIDFile(t, livePidFile, strconv.Itoa(currentPID)+"\n")
	livePID, liveOK := readPidFile(livePidFile)
	assert.Equal(t, currentPID, livePID)
	assert.True(t, liveOK)
}

func TestHostServiceCycle32_Adversarial_SupervisorSelf_PIDVariations(t *testing.T) {
	assert.Equal(t, 0, liveSupervisorPID(""))
	assert.Equal(t, 0, liveSupervisorPID("   "))
	assert.Equal(t, 0, liveSupervisorPID("/non/existent/path"))

	tempDir := t.TempDir()
	pidPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerPIDFile)
	createAdversarialPIDFile(t, pidPath, "corrupted-pid\n")
	assert.Equal(t, 0, liveSupervisorPID(tempDir))

	createAdversarialPIDFile(t, pidPath, "-123\n")
	assert.Equal(t, 0, liveSupervisorPID(tempDir))

	createAdversarialPIDFile(t, pidPath, "54321\n")
	assert.Equal(t, 54321, liveSupervisorPID(tempDir))

	missingRoot := t.TempDir()
	require.NoError(t, refuseIfCallerDescendsFromUnit(Entry{AbsRoot: missingRoot}))

	createAdversarialPIDFile(t, pidPath, strconv.Itoa(os.Getpid())+"\n")
	refuseErr := refuseIfCallerDescendsFromUnit(Entry{AbsRoot: tempDir, UnitLabel: "test.label"})
	require.Error(t, refuseErr)
	assert.Contains(t, refuseErr.Error(), "refusing host-service mutation")
}

func TestHostServiceCycle32_Adversarial_Registry_CorruptedJSON(t *testing.T) {
	setupAdversarialHome(t)
	regPath, err := RegistryPath()
	require.NoError(t, err)

	createAdversarialPIDFile(t, regPath, "{ malformed json: not valid [[")
	_, loadErr := LoadRegistry()
	require.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "parse host service registry")

	cleanReg := &Registry{Entries: []Entry{{RootID: "recovered-id"}}}
	saveErr := SaveRegistry(cleanReg)
	require.NoError(t, saveErr)

	stat, statErr := fileutil.Stat(regPath + ".tmp")
	assert.Nil(t, stat)
	assert.True(t, fileutil.IsNotExist(statErr))

	reloaded, reloadErr := LoadRegistry()
	require.NoError(t, reloadErr)
	assert.Len(t, reloaded.Entries, 1)
	assert.Equal(t, "recovered-id", reloaded.Entries[0].RootID)
}

func TestHostServiceCycle32_Adversarial_Registry_MissingFileReturnsEmpty(t *testing.T) {
	setupAdversarialHome(t)
	reg, err := LoadRegistry()
	require.NoError(t, err)
	assert.NotNil(t, reg)
	assert.Empty(t, reg.Entries)
}

func TestHostServiceCycle32_Adversarial_Registry_DirectoryConflict(t *testing.T) {
	setupAdversarialHome(t)
	regPath, err := RegistryPath()
	require.NoError(t, err)

	parentDir := filepath.Dir(regPath)
	fileAsParent := parentDir
	cleanErr := fileutil.RemoveAll(parentDir)
	require.NoError(t, cleanErr)

	mkdirParentErr := fileutil.MkdirAll(filepath.Dir(parentDir), paths.DirPerm755)
	require.NoError(t, mkdirParentErr)

	writeErr := fileutil.WriteFile(fileAsParent, []byte("blocking-file"), paths.FilePerm644)
	require.NoError(t, writeErr)

	reg := &Registry{Entries: []Entry{{RootID: "conflict-id"}}}
	saveErr := SaveRegistry(reg)
	require.Error(t, saveErr)
	assert.Contains(t, saveErr.Error(), "mkdir host service registry")

	removeErr := fileutil.Remove(fileAsParent)
	require.NoError(t, removeErr)

	dirAsFile := regPath
	mkdirErr := fileutil.MkdirAll(dirAsFile, paths.DirPerm755)
	require.NoError(t, mkdirErr)

	_, loadErr := LoadRegistry()
	require.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "read host service registry")
}

func TestHostServiceCycle32_Adversarial_Registry_PermissionDenied(t *testing.T) {
	setupAdversarialHome(t)
	regPath, err := RegistryPath()
	require.NoError(t, err)

	reg := &Registry{Entries: []Entry{{RootID: "perm-id"}}}
	saveErr := SaveRegistry(reg)
	require.NoError(t, saveErr)

	chmodErr := fileutil.Chmod(regPath, 0000)
	require.NoError(t, chmodErr)
	t.Cleanup(func() {
		_ = fileutil.Chmod(regPath, paths.FilePerm644)
	})

	_, loadErr := LoadRegistry()
	require.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "read host service registry")
}

func TestHostServiceCycle32_Adversarial_CrashRecovery_WranglerStatusStalePIDs(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()

	schedPID := filepath.Join(tempDir, paths.ProjectDataDir, paths.StateDir, "scheduler.pid")
	createAdversarialPIDFile(t, schedPID, "999999999\n")

	mcpPID := filepath.Join(tempDir, paths.ProjectDataDir, "mcp", "daemon-8443.pid")
	createAdversarialPIDFile(t, mcpPID, "999999998\n")

	wrangler := NewRuntimeWrangler(tempDir)
	report, err := wrangler.Status()
	require.NoError(t, err)
	require.NotNil(t, report)

	for _, unit := range report.Units {
		if unit.Name == "scheduler" {
			assert.False(t, unit.Running)
			assert.Equal(t, 0, unit.PID)
		}
	}
}

func TestHostServiceCycle32_Adversarial_CrashRecovery_WranglerStopStalePIDCleanup(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()

	schedPID := filepath.Join(tempDir, paths.ProjectDataDir, paths.StateDir, "scheduler.pid")
	createAdversarialPIDFile(t, schedPID, "999999999\n")

	mcpPID := filepath.Join(tempDir, paths.ProjectDataDir, "mcp", "daemon-8443.pid")
	createAdversarialPIDFile(t, mcpPID, "999999998\n")

	wrangler := NewRuntimeWrangler(tempDir)
	stopErr := wrangler.Stop()
	require.NoError(t, stopErr)

	_, statSched := fileutil.Stat(schedPID)
	assert.True(t, fileutil.IsNotExist(statSched))

	_, statMCP := fileutil.Stat(mcpPID)
	assert.True(t, fileutil.IsNotExist(statMCP))
}

func TestHostServiceCycle32_Adversarial_Wrangler_UninstalledServiceOperations(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()

	wrangler := NewRuntimeWrangler(tempDir)
	report, err := wrangler.Status()
	require.NoError(t, err)
	assert.Equal(t, tempDir, report.ProjectRoot)

	startErr := wrangler.Start()
	require.NoError(t, startErr)

	stopErr := wrangler.Stop()
	require.NoError(t, stopErr)
}

func TestHostServiceCycle32_Adversarial_GC_And_Orphans_MissingRoots(t *testing.T) {
	setupAdversarialHome(t)
	liveDir := t.TempDir()
	deletedDir := filepath.Join(t.TempDir(), "deleted_dir")

	entryLive := Entry{RootID: "live", AbsRoot: liveDir, DesiredState: DesiredStateEnabled}
	entryDeleted := Entry{RootID: "orphan", AbsRoot: deletedDir, DesiredState: DesiredStateEnabled}
	entryAbsent := Entry{RootID: "absent", AbsRoot: liveDir, DesiredState: DesiredStateAbsent}

	reg := &Registry{Entries: []Entry{entryLive, entryDeleted, entryAbsent}}
	require.NoError(t, SaveRegistry(reg))

	orphans, orphanErr := Orphans()
	require.NoError(t, orphanErr)
	assert.Len(t, orphans, 1)
	assert.Equal(t, "orphan", orphans[0].RootID)

	removed, gcErr := GC()
	require.NoError(t, gcErr)
	assert.Len(t, removed, 2)

	reloaded, reloadErr := LoadRegistry()
	require.NoError(t, reloadErr)
	assert.Len(t, reloaded.Entries, 1)
	assert.Equal(t, "live", reloaded.Entries[0].RootID)
}

func TestHostServiceCycle32_Adversarial_InspectLaunchAgent_UnknownService(t *testing.T) {
	pid, running, details := inspectLaunchAgent("com.zqk.adversarial.nonexistent.service.9999")
	assert.Equal(t, 0, pid)
	assert.False(t, running)
	if runtime.GOOS == "darwin" {
		assert.Equal(t, "not loaded", details)
	} else {
		assert.Equal(t, "unsupported OS", details)
	}
}

func TestHostServiceCycle32_Adversarial_FormatReportJSON_AdversarialEdgeCases(t *testing.T) {
	emptyReport := &RuntimeWranglerReport{
		ProjectRoot: "/tmp/test",
		RootID:      "root-123",
		Timestamp:   time.Now().UTC(),
		Units:       nil,
	}
	b, err := FormatReportJSON(emptyReport)
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	var decoded RuntimeWranglerReport
	unmarshalErr := json.Unmarshal(b, &decoded)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, emptyReport.RootID, decoded.RootID)

	complexReport := &RuntimeWranglerReport{
		ProjectRoot: "/tmp/unicode-\u2602-\u6771\u4eac",
		RootID:      "root-unicode",
		Timestamp:   time.Now().UTC(),
		Units: []HostServiceUnit{
			{Name: "scheduler", Details: "line1\nline2\t\"quoted\"", PID: -1},
		},
	}
	bComplex, errComplex := FormatReportJSON(complexReport)
	require.NoError(t, errComplex)
	assert.NotEmpty(t, bComplex)
}

func TestHostServiceCycle32_Adversarial_DarwinAdapter_RefusalOnAncestor(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()
	pidPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerPIDFile)
	createAdversarialPIDFile(t, pidPath, strconv.Itoa(os.Getpid())+"\n")

	entry := Entry{AbsRoot: tempDir, UnitLabel: "com.test.ancestor"}
	adapter := DarwinAdapter{}

	startErr := adapter.Start(entry)
	require.Error(t, startErr)
	assert.Contains(t, startErr.Error(), "refusing host-service mutation")

	stopErr := adapter.Stop(entry)
	require.Error(t, stopErr)
	assert.Contains(t, stopErr.Error(), "refusing host-service mutation")

	st, stErr := adapter.Status(Entry{UnitLabel: "com.zqk.unknown.label.999"})
	require.NoError(t, stErr)
	assert.Equal(t, "stopped", st)

	uninstallErr := adapter.Uninstall(Entry{UnitLabel: "com.zqk.unknown.label.999"})
	require.NoError(t, uninstallErr)
}

func TestHostServiceCycle32_Adversarial_LinuxAdapter_UnitPathAndStatus(t *testing.T) {
	setupAdversarialHome(t)
	adapter := LinuxAdapter{}
	entry := Entry{UnitLabel: "com.zqk.test.linux"}

	path := adapter.unitPath(entry)
	assert.True(t, strings.HasSuffix(path, filepath.Join(".config", "systemd", "user", "com.zqk.test.linux.service")))

	st, stErr := adapter.Status(entry)
	require.NoError(t, stErr)
	assert.NotEmpty(t, st)

	uninstallErr := adapter.Uninstall(entry)
	require.NoError(t, uninstallErr)
}

func TestHostServiceCycle32_Adversarial_ResolveDaemonBinary_EdgeCases(t *testing.T) {
	resEmpty := ResolveDaemonBinary("", "")
	assert.Equal(t, brand.ExecutableName(), resEmpty)

	resSched := ResolveDaemonBinary("", "sched")
	assert.Equal(t, brand.ExecutableName()+"-sched", resSched)

	resUnknown := ResolveDaemonBinary("", "unknown-role-123")
	assert.Equal(t, brand.ExecutableName()+"-unknown-role-123", resUnknown)

	resMissingRoot := ResolveDaemonBinary("/non/existent/project/root", "amb")
	assert.NotEmpty(t, resMissingRoot)

	resServiceEmpty := ResolveServiceDaemonBinary("")
	assert.Equal(t, brand.ExecutableName()+"-sched", resServiceEmpty)
}

func TestHostServiceCycle32_Adversarial_RootID_Canonicalization(t *testing.T) {
	idA := RootID("/path/to/project/../project")
	idB := RootID("/path/to/project/.")
	idC := RootID("/path/to/project/")
	idD := RootID("/path/to/project")

	assert.Equal(t, idD, idA)
	assert.Equal(t, idD, idB)
	assert.Equal(t, idD, idC)
	assert.NotEmpty(t, idD)

	unitLabel := UnitLabel(idD)
	assert.True(t, strings.HasPrefix(unitLabel, "com."+brand.ExecutableName()+".scheduler."))
}

func TestHostServiceCycle32_Adversarial_RoleSymlink_IdempotentUpdate(t *testing.T) {
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	require.NoError(t, fileutil.MkdirAll(binDir, paths.DirPerm755))

	targetBin := createAdversarialExecutable(t, binDir, brand.ExecutableName())
	link1, err1 := EnsureServiceRoleSymlink(tempDir, "sched", targetBin)
	require.NoError(t, err1)
	assert.NotEmpty(t, link1)

	link2, err2 := EnsureServiceRoleSymlink(tempDir, "sched", targetBin)
	require.NoError(t, err2)
	assert.Equal(t, link1, link2)

	errAll := EnsureAllServiceRoleSymlinks(tempDir, targetBin)
	require.NoError(t, errAll)
}
