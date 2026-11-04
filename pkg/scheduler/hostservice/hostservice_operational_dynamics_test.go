package hostservice

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestHostServiceCycle32_Functional_InstallRebindUninstallLifecycle(t *testing.T) {
	setupAdversarialHome(t)
	tempRoot := t.TempDir()

	// 1. InstallRoot
	entry, err := InstallRoot(tempRoot)
	require.NoError(t, err)
	assert.Equal(t, tempRoot, entry.AbsRoot)
	assert.Equal(t, RootID(tempRoot), entry.RootID)
	assert.Equal(t, DesiredStateEnabled, entry.DesiredState)

	// Verify it shows in registry
	resolved, resolveErr := ResolveEntry(tempRoot)
	require.NoError(t, resolveErr)
	assert.Equal(t, entry.RootID, resolved.RootID)

	// 2. Rebind to new root
	newRoot := t.TempDir()
	rebound, rebindErr := Rebind(entry.RootID, newRoot)
	require.NoError(t, rebindErr)
	assert.Equal(t, newRoot, rebound.AbsRoot)

	// 3. UninstallRoot
	uninstallErr := UninstallRoot(entry.RootID)
	require.NoError(t, uninstallErr)

	// Verify uninstalled
	_, notFoundErr := ResolveEntry(newRoot)
	require.Error(t, notFoundErr)

	// Error path: InstallRoot fails when adapter install fails
	launchctlRunner = func(args ...string) ([]byte, error) {
		return nil, assert.AnError
	}
	systemctlRunner = func(args ...string) ([]byte, error) {
		return nil, assert.AnError
	}
	defer func() {
		launchctlRunner = nil
		systemctlRunner = nil
	}()
	_, installFailErr := InstallRoot(tempRoot)
	require.Error(t, installFailErr)
	launchctlRunner = nil
	systemctlRunner = nil
}

func TestHostServiceCycle32_Functional_WranglerStartAndStatus(t *testing.T) {
	setupAdversarialHome(t)
	tempRoot := t.TempDir()

	// Install root first
	entry, err := InstallRoot(tempRoot)
	require.NoError(t, err)

	wrangler := NewRuntimeWrangler(tempRoot)
	require.NotNil(t, wrangler)

	// Start wrangler
	startErr := wrangler.Start()
	require.NoError(t, startErr)

	// Status check
	report, err := wrangler.Status()
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, entry.RootID, report.RootID)

	// Stop wrangler
	stopErr := wrangler.Stop()
	require.NoError(t, stopErr)
}

func TestHostServiceCycle32_Functional_DarwinAdapterDirectMethods(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()
	entry := Entry{
		RootID:       "darwin-test-root",
		AbsRoot:      tempDir,
		UnitLabel:    "com.zqk.darwin.functional.test",
		DesiredState: DesiredStateEnabled,
		BinaryRef:    "/path/to/zqk",
	}

	adapter := DarwinAdapter{}

	// Install
	installErr := adapter.Install(entry)
	require.NoError(t, installErr)

	// Status
	status, stErr := adapter.Status(entry)
	require.NoError(t, stErr)
	assert.Equal(t, "stopped", status)

	// Start
	startErr := adapter.Start(entry)
	require.NoError(t, startErr)

	// Stop
	stopErr := adapter.Stop(entry)
	require.NoError(t, stopErr)

	// Uninstall
	uninstallErr := adapter.Uninstall(entry)
	require.NoError(t, uninstallErr)
}

func TestHostServiceCycle32_Functional_LinuxAdapterLifecycle(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()
	entry := Entry{
		RootID:       "linux-test-root",
		AbsRoot:      tempDir,
		UnitLabel:    "com.zqk.linux.functional.test",
		DesiredState: DesiredStateEnabled,
		BinaryRef:    "/path/to/zqk",
	}

	adapter := LinuxAdapter{}

	unitPath := adapter.unitPath(entry)
	assert.True(t, strings.HasSuffix(unitPath, "com.zqk.linux.functional.test.service"))

	// In test environment, install writes the systemd service file cleanly
	installErr := adapter.Install(entry)
	require.NoError(t, installErr)
	assert.True(t, fileutil.IsRegularFile(unitPath))

	status, stErr := adapter.Status(entry)
	require.NoError(t, stErr)
	assert.NotEmpty(t, status)

	startErr := adapter.Start(entry)
	require.NoError(t, startErr)

	stopErr := adapter.Stop(entry)
	require.NoError(t, stopErr)

	uninstallErr := adapter.Uninstall(entry)
	require.NoError(t, uninstallErr)
}

func TestHostServiceCycle32_Functional_NewAdapterPlatformDispatch(t *testing.T) {
	adapter := NewAdapter()
	require.NotNil(t, adapter)
	if runtime.GOOS == "darwin" {
		assert.IsType(t, DarwinAdapter{}, adapter)
	} else if runtime.GOOS == "linux" {
		assert.IsType(t, LinuxAdapter{}, adapter)
	}
}

func TestHostServiceCycle32_Functional_EnsureAllServiceRoleSymlinks_Full(t *testing.T) {
	tempRoot := t.TempDir()
	binDir := filepath.Join(tempRoot, "bin")
	require.NoError(t, fileutil.MkdirAll(binDir, paths.DirPerm755))

	fakeBin := filepath.Join(binDir, "zqk")
	require.NoError(t, fileutil.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755))

	err := EnsureAllServiceRoleSymlinks(tempRoot, fakeBin)
	require.NoError(t, err)

	// Verify symlinks were created
	assert.True(t, fileutil.Exists(filepath.Join(binDir, "zqk-sched")))
	assert.True(t, fileutil.Exists(filepath.Join(binDir, "zqk-amb")))

	// Calling with no targetBin when bin/zqk exists should resolve it
	err2 := EnsureAllServiceRoleSymlinks(tempRoot)
	require.NoError(t, err2)

	// Empty projectRoot returns error
	require.Error(t, EnsureAllServiceRoleSymlinks(""))

	// External binary target requiring symlink rewrite in baseInBin
	otherDir := t.TempDir()
	extBin1 := filepath.Join(otherDir, "zqk1")
	require.NoError(t, fileutil.WriteFile(extBin1, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755))
	extBin2 := filepath.Join(otherDir, "zqk2")
	require.NoError(t, fileutil.WriteFile(extBin2, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755))

	// Initial symlink to extBin1
	require.NoError(t, EnsureAllServiceRoleSymlinks(tempRoot, extBin1))

	// Rewrite symlink to extBin2 (exercises evalSymlinks mismatch branch)
	require.NoError(t, EnsureAllServiceRoleSymlinks(tempRoot, extBin2))
}

func TestHostServiceCycle32_Functional_ResolveDaemonBinary_Variants(t *testing.T) {
	// Empty project root
	assert.NotEmpty(t, ResolveDaemonBinary("", ""))
	assert.NotEmpty(t, ResolveDaemonBinary("", "sched"))
	assert.NotEmpty(t, ResolveDaemonBinary("", "amb"))
	assert.NotEmpty(t, ResolveDaemonBinary("", "unknown-custom-role"))

	// Non-empty root with fake binary
	tempRoot := t.TempDir()
	binDir := filepath.Join(tempRoot, "bin")
	require.NoError(t, fileutil.MkdirAll(binDir, paths.DirPerm755))
	fakeBin := filepath.Join(binDir, "zqk")
	require.NoError(t, fileutil.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755))

	res := ResolveDaemonBinary(tempRoot, "sched")
	assert.NotEmpty(t, res)

	resEmpty := ResolveDaemonBinary(tempRoot, "")
	assert.NotEmpty(t, resEmpty)
}

func TestHostServiceCycle32_Functional_ResolveEntry_Variants(t *testing.T) {
	setupAdversarialHome(t)
	tempRoot := t.TempDir()

	entry, err := InstallRoot(tempRoot)
	require.NoError(t, err)

	// Match by root ID
	e1, err1 := ResolveEntry(entry.RootID)
	require.NoError(t, err1)
	assert.Equal(t, entry.RootID, e1.RootID)

	// Match by AbsRoot
	e2, err2 := ResolveEntry(tempRoot)
	require.NoError(t, err2)
	assert.Equal(t, entry.RootID, e2.RootID)

	// Match not found
	_, errNotFound := ResolveEntry("non-existent-root-id-xyz")
	require.Error(t, errNotFound)
}

func TestHostServiceCycle32_Functional_RuntimeWranglerStop_WithPids(t *testing.T) {
	setupAdversarialHome(t)
	tempRoot := t.TempDir()

	entry, err := InstallRoot(tempRoot)
	require.NoError(t, err)

	// Create scheduler PID file with stale/harmless PID 999999
	stateDir := filepath.Join(tempRoot, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	schedPIDFile := filepath.Join(stateDir, "scheduler.pid")
	require.NoError(t, fileutil.WriteFile(schedPIDFile, []byte("999999\n"), paths.FilePerm644))

	// Create MCP daemon PID file with stale PID 999998
	mcpDir := filepath.Join(tempRoot, paths.ProjectDataDir, "mcp")
	require.NoError(t, fileutil.MkdirAll(mcpDir, paths.DirPerm755))
	mcpPIDFile := filepath.Join(mcpDir, "daemon-8443.pid")
	require.NoError(t, fileutil.WriteFile(mcpPIDFile, []byte("999998\n"), paths.FilePerm644))

	// Create mock LaunchAgent plists in fake home
	home, _ := fileutil.UserHomeDir()
	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	require.NoError(t, fileutil.MkdirAll(launchAgentsDir, paths.DirPerm755))
	_ = fileutil.WriteFile(filepath.Join(launchAgentsDir, UnitLabel(entry.RootID)+".plist"), []byte("<plist/>"), paths.FilePerm644)
	_ = fileutil.WriteFile(filepath.Join(launchAgentsDir, "com.zqk.mesh.night-duty.plist"), []byte("<plist/>"), paths.FilePerm644)

	var stoppedLabels []string
	launchctlRunner = func(args ...string) ([]byte, error) {
		stoppedLabels = append(stoppedLabels, strings.Join(args, " "))
		return []byte("ok"), nil
	}
	defer func() { launchctlRunner = nil }()

	wrangler := NewRuntimeWrangler(tempRoot)
	require.NotNil(t, wrangler)

	// Stop cleans up both PID files and sets desired state to disabled
	stopErr := wrangler.Stop()
	require.NoError(t, stopErr)

	assert.False(t, fileutil.Exists(schedPIDFile))
	assert.False(t, fileutil.Exists(mcpPIDFile))
	assert.NotEmpty(t, stoppedLabels)

	// Verify entry status is disabled
	resolved, resErr := ResolveEntry(entry.RootID)
	require.NoError(t, resErr)
	assert.Equal(t, DesiredStateDisabled, resolved.DesiredState)
}

func TestHostServiceCycle32_Functional_GCAndOrphans(t *testing.T) {
	setupAdversarialHome(t)
	liveRoot := t.TempDir()
	deadRoot := filepath.Join(t.TempDir(), "deleted-dir")
	require.NoError(t, fileutil.MkdirAll(deadRoot, paths.DirPerm755))

	eLive, err := InstallRoot(liveRoot)
	require.NoError(t, err)
	eDead, err := InstallRoot(deadRoot)
	require.NoError(t, err)

	// Delete deadRoot from disk
	require.NoError(t, fileutil.RemoveAll(deadRoot))

	// Orphans should identify deadRoot
	orphans, oErr := Orphans()
	require.NoError(t, oErr)
	assert.Len(t, orphans, 1)
	assert.Equal(t, eDead.RootID, orphans[0].RootID)

	// GC should prune deadRoot
	cleaned, gcErr := GC()
	require.NoError(t, gcErr)
	require.Len(t, cleaned, 1)
	assert.Equal(t, eDead.RootID, cleaned[0].RootID)

	// Live root remains
	_, liveErr := ResolveEntry(eLive.RootID)
	require.NoError(t, liveErr)
}

func TestHostServiceCycle32_Functional_DarwinAdapter_MockLaunchctl(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()
	entry := Entry{
		RootID:       "darwin-mock-root",
		AbsRoot:      tempDir,
		UnitLabel:    "com.zqk.darwin.mock.test",
		DesiredState: DesiredStateEnabled,
		BinaryRef:    "/path/to/zqk",
	}

	adapter := DarwinAdapter{}

	// 1. Success paths with mock runner
	var recordedCalls [][]string
	launchctlRunner = func(args ...string) ([]byte, error) {
		recordedCalls = append(recordedCalls, args)
		if len(args) > 0 && args[0] == "print" {
			return []byte("state = running\npid = 42\n"), nil
		}
		return []byte("ok"), nil
	}
	defer func() { launchctlRunner = nil }()

	require.NoError(t, adapter.Install(entry))
	require.NoError(t, adapter.Start(entry))
	st, err := adapter.Status(entry)
	require.NoError(t, err)
	assert.Equal(t, "running", st)

	// 2. Error branches
	// Bootstrap fails, fallback to load
	launchctlRunner = func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "bootstrap" {
			return nil, assert.AnError
		}
		if len(args) > 0 && args[0] == "kickstart" {
			return nil, assert.AnError
		}
		return []byte("ok"), nil
	}
	require.NoError(t, adapter.Install(entry))
	require.NoError(t, adapter.Start(entry))

	// Both bootstrap and load fail on Install
	launchctlRunner = func(args ...string) ([]byte, error) {
		if len(args) > 0 && (args[0] == "bootstrap" || args[0] == "load") {
			return nil, assert.AnError
		}
		return []byte("ok"), nil
	}
	require.Error(t, adapter.Install(entry))

	// Both kickstart and start fail on Start
	launchctlRunner = func(args ...string) ([]byte, error) {
		if len(args) > 0 && (args[0] == "kickstart" || args[0] == "start") {
			return nil, assert.AnError
		}
		return []byte("ok"), nil
	}
	require.Error(t, adapter.Start(entry))

	// Stop with running status returning error
	launchctlRunner = func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "print" {
			return []byte("state = running\n"), nil
		}
		return []byte("ok"), nil
	}
	require.Error(t, adapter.Stop(entry))

	// Stop and Uninstall success
	launchctlRunner = func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "print" {
			return nil, assert.AnError
		}
		return []byte("ok"), nil
	}
	require.NoError(t, adapter.Stop(entry))
	require.NoError(t, adapter.Uninstall(entry))
}

func TestHostServiceCycle32_Functional_LinuxAdapter_MockSystemctl(t *testing.T) {
	setupAdversarialHome(t)
	tempDir := t.TempDir()
	entry := Entry{
		RootID:       "linux-mock-root",
		AbsRoot:      tempDir,
		UnitLabel:    "com.zqk.linux.mock.test",
		DesiredState: DesiredStateEnabled,
		BinaryRef:    "/path/to/zqk",
	}

	adapter := LinuxAdapter{}

	// 1. Success paths
	systemctlRunner = func(args ...string) ([]byte, error) {
		for _, arg := range args {
			if arg == "is-active" {
				return []byte("active\n"), nil
			}
		}
		return []byte("ok"), nil
	}
	defer func() { systemctlRunner = nil }()

	require.NoError(t, adapter.Install(entry))
	require.NoError(t, adapter.Start(entry))
	st, err := adapter.Status(entry)
	require.NoError(t, err)
	assert.Equal(t, "active", st)
	require.NoError(t, adapter.Stop(entry))
	require.NoError(t, adapter.Uninstall(entry))

	// 2. Error paths
	// Install fails on enable
	systemctlRunner = func(args ...string) ([]byte, error) {
		for _, arg := range args {
			if arg == "enable" {
				return nil, assert.AnError
			}
		}
		return []byte("ok"), nil
	}
	require.Error(t, adapter.Install(entry))

	// Start fails on enable and start
	systemctlRunner = func(args ...string) ([]byte, error) {
		for _, arg := range args {
			if arg == "enable" || arg == "start" {
				return nil, assert.AnError
			}
		}
		return []byte("ok"), nil
	}
	require.Error(t, adapter.Start(entry))

	// Stop fails on disable
	systemctlRunner = func(args ...string) ([]byte, error) {
		for _, arg := range args {
			if arg == "disable" {
				return nil, assert.AnError
			}
		}
		return []byte("ok"), nil
	}
	require.Error(t, adapter.Stop(entry))

	// Status error with empty output returns "inactive"
	systemctlRunner = func(args ...string) ([]byte, error) {
		for _, arg := range args {
			if arg == "is-active" {
				return []byte(""), assert.AnError
			}
		}
		return nil, nil
	}
	st2, err2 := adapter.Status(entry)
	require.NoError(t, err2)
	assert.Equal(t, "inactive", st2)
}

func TestHostServiceCycle32_Functional_InspectLaunchAgent_Variations(t *testing.T) {
	// 1. Running daemon with PID
	launchctlRunner = func(args ...string) ([]byte, error) {
		return []byte("1234\t0\tcom.zqk.test.running\n-\t1\tcom.zqk.test.loaded\n-\t0\tcom.zqk.test.ok\n"), nil
	}
	defer func() { launchctlRunner = nil }()

	pid, running, details := inspectLaunchAgent("com.zqk.test.running")
	assert.Equal(t, 1234, pid)
	assert.True(t, running)
	assert.Contains(t, details, "running")

	// 2. Loaded daemon with non-zero exit
	pid2, running2, details2 := inspectLaunchAgent("com.zqk.test.loaded")
	assert.Equal(t, 0, pid2)
	assert.False(t, running2)
	assert.Contains(t, details2, "loaded (last_exit=1)")

	// 3. Unknown daemon
	_, running3, details3 := inspectLaunchAgent("com.zqk.nonexistent")
	assert.False(t, running3)
	assert.Equal(t, "not loaded", details3)

	// 4. Launchctl command error
	launchctlRunner = func(args ...string) ([]byte, error) {
		return nil, assert.AnError
	}
	_, _, details4 := inspectLaunchAgent("com.zqk.test")
	assert.Equal(t, "launchctl error", details4)
}
