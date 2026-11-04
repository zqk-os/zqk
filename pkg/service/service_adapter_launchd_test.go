// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func mockCommandRunner(t testing.TB, handler func(name string, args ...string) (string, error)) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		out, err := handler(name, args...)
		if err != nil {
			cmd := testkit.ManagedCommand(t, ctx, "/bin/sh", "-c", "cat >&2; exit 1")
			cmd.Stdin = strings.NewReader(out)
			return cmd
		}
		cmd := testkit.ManagedCommand(t, ctx, "/bin/sh", "-c", "cat")
		cmd.Stdin = strings.NewReader(out)
		return cmd
	}
}

func TestCycle208_AdapterHelpers_DirectoryOperations(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Nonexistent directory returns nil, nil
	entries, err := readServiceDirEntries(filepath.Join(tempDir, "does-not-exist"))
	require.NoError(t, err)
	assert.Nil(t, entries)

	// 2. File as directory returns error
	regularFile := filepath.Join(tempDir, "file.txt")
	err = fileutil.WriteFile(regularFile, []byte("data"), paths.FilePerm644)
	require.NoError(t, err)

	entries, err = readServiceDirEntries(regularFile)
	assert.Error(t, err)
	assert.Nil(t, entries)

	// 3. Valid directory with file
	validDir := filepath.Join(tempDir, "valid-dir")
	err = fileutil.MkdirAll(validDir, paths.DirPerm755)
	require.NoError(t, err)

	testItem := filepath.Join(validDir, "item.plist")
	err = fileutil.WriteFile(testItem, []byte("test"), paths.FilePerm644)
	require.NoError(t, err)

	entries, err = readServiceDirEntries(validDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	// 4. readValidServiceDir with empty directory
	emptyDir := filepath.Join(tempDir, "empty-dir")
	err = fileutil.MkdirAll(emptyDir, paths.DirPerm755)
	require.NoError(t, err)

	validEntries, err := readValidServiceDir(emptyDir)
	require.NoError(t, err)
	assert.Nil(t, validEntries)
}

func TestCycle208_AdapterHelpers_LegacyAndUnitFile(t *testing.T) {
	tempDir := t.TempDir()

	// 1. initLegacyCleanup on empty directory
	emptyDir := filepath.Join(tempDir, "empty-clean-dir")
	err := fileutil.MkdirAll(emptyDir, paths.DirPerm755)
	require.NoError(t, err)

	targetSet, entries, err := initLegacyCleanup(emptyDir, []string{"s1"})
	require.NoError(t, err)
	assert.Nil(t, targetSet)
	assert.Nil(t, entries)

	// 2. initLegacyCleanup on dir with files
	workDir := filepath.Join(tempDir, "work-clean-dir")
	err = fileutil.MkdirAll(workDir, paths.DirPerm755)
	require.NoError(t, err)

	f1 := filepath.Join(workDir, "legacy.plist")
	err = fileutil.WriteFile(f1, []byte("data"), paths.FilePerm644)
	require.NoError(t, err)

	targetSet, entries, err = initLegacyCleanup(workDir, []string{"legacy", "other"})
	require.NoError(t, err)
	assert.Len(t, targetSet, 2)
	assert.Len(t, entries, 1)

	// 3. checkUnitFilePresent branches
	st, present, err := checkUnitFilePresent(workDir, errors.New("dir error"), "legacy.plist", true, "svc")
	assert.Error(t, err)
	assert.False(t, present)
	assert.Equal(t, StateStopped, st.State)

	st, present, err = checkUnitFilePresent(workDir, nil, "nonexistent.plist", true, "svc")
	require.NoError(t, err)
	assert.False(t, present)
	assert.Equal(t, StateStopped, st.State)

	st, present, err = checkUnitFilePresent(workDir, nil, "legacy.plist", false, "svc")
	require.NoError(t, err)
	assert.False(t, present)

	st, present, err = checkUnitFilePresent(workDir, nil, "legacy.plist", true, "svc")
	require.NoError(t, err)
	assert.True(t, present)
	assert.Equal(t, StateStopped, st.State)
}

func TestCycle208_LaunchdAdapter_PlistOptions(t *testing.T) {
	la := NewLaunchdAdapter()

	// 1. RestartOnFailure policy
	spec1 := ServiceSpec{
		ID:            "com.test.onfailure",
		Executable:    "/bin/service",
		RestartPolicy: RestartOnFailure,
	}
	content1 := la.PlistContent(spec1)
	assert.Contains(t, content1, "<key>SuccessfulExit</key>\n    <false/>")
	assert.Contains(t, content1, "<key>RunAtLoad</key>\n  <false/>")

	// 2. StandardErrorPath without WorkingDir or Environment
	spec2 := ServiceSpec{
		ID:                "com.test.stderr",
		Executable:        "/bin/service",
		StandardErrorPath: "/var/log/err.log",
		RestartPolicy:     RestartNever,
	}
	content2 := la.PlistContent(spec2)
	assert.Contains(t, content2, "<key>StandardErrorPath</key>\n  <string>/var/log/err.log</string>")
	assert.NotContains(t, content2, "<key>WorkingDirectory</key>")
	assert.NotContains(t, content2, "<key>EnvironmentVariables</key>")
	assert.NotContains(t, content2, "<key>KeepAlive</key>")

	// 3. Default getDir uses home directory
	laDefault := NewLaunchdAdapter()
	laDefault.BaseDir = ""
	dir, err := laDefault.getDir()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(dir, filepath.Join("Library", "LaunchAgents")))

	// 4. Custom BaseDir
	laDefault.BaseDir = "/custom/path"
	dir, err = laDefault.getDir()
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", dir)
}

func TestCycle208_LaunchdAdapter_InstallErrorsAndFallbacks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	la := NewLaunchdAdapter()

	// 1. Invalid spec fails
	err := la.Install(ctx, ServiceSpec{})
	assert.Error(t, err)

	// 2. Unsupported platform when not available and BaseDir empty
	la.AvailableFunc = func() bool { return false }
	la.BaseDir = ""
	err = la.Install(ctx, ServiceSpec{ID: "test.id", Executable: "/bin/true"})
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)

	// 3. Fallback from bootstrap to load succeeding
	tempDir := t.TempDir()
	la.BaseDir = tempDir
	la.AvailableFunc = func() bool { return true }

	var executedCmds []string
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		full := strings.Join(append([]string{name}, args...), " ")
		executedCmds = append(executedCmds, full)
		if strings.Contains(full, "launchctl bootstrap") {
			return "bootstrap failed", errors.New("bootstrap error")
		}
		return "ok", nil
	})

	err = la.Install(ctx, ServiceSpec{ID: "com.test.fallback", Executable: "/bin/echo"})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(tempDir, "com.test.fallback.plist"))

	// 4. Bootstrap and load both failing
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "failure", errors.New("command failed")
	})
	err = la.Install(ctx, ServiceSpec{ID: "com.test.bothfail", Executable: "/bin/echo"})
	assert.Error(t, err)
}

func TestCycle208_LaunchdAdapter_UninstallEdgeCases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tempDir := t.TempDir()
	la := NewLaunchdAdapter()
	la.BaseDir = tempDir
	la.AvailableFunc = func() bool { return true }

	// 1. Uninstall nonexistent returns ErrServiceNotFound
	err := la.Uninstall(ctx, "nonexistent.service")
	assert.ErrorIs(t, err, ErrServiceNotFound)

	// 2. Uninstall existing file runs launchctl unload/bootout
	spec := ServiceSpec{ID: "com.test.uninstall", Executable: "/bin/true"}
	content := la.PlistContent(spec)
	plistFile := filepath.Join(tempDir, spec.ID+".plist")
	err = fileutil.WriteFile(plistFile, []byte(content), paths.FilePerm644)
	require.NoError(t, err)

	unloaded := false
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		unloaded = true
		return "", nil
	})

	err = la.Uninstall(ctx, spec.ID)
	require.NoError(t, err)
	assert.True(t, unloaded)
	assert.NoFileExists(t, plistFile)
}

func TestCycle208_LaunchdAdapter_StartStopRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tempDir := t.TempDir()
	la := NewLaunchdAdapter()
	la.BaseDir = tempDir
	la.AvailableFunc = func() bool { return true }

	// Missing plist errors
	err := la.Start(ctx, "missing")
	assert.ErrorIs(t, err, ErrServiceNotFound)
	err = la.Stop(ctx, "missing")
	assert.ErrorIs(t, err, ErrServiceNotFound)

	// Create plist
	id := "com.test.lifecycle"
	plistFile := filepath.Join(tempDir, id+".plist")
	err = fileutil.WriteFile(plistFile, []byte("<plist></plist>"), paths.FilePerm644)
	require.NoError(t, err)

	// Start: kickstart succeeds
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "", nil
	})
	err = la.Start(ctx, id)
	require.NoError(t, err)

	// Start: kickstart fails, fallback start succeeds
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "kickstart") {
			return "fail", errors.New("kickstart err")
		}
		return "", nil
	})
	err = la.Start(ctx, id)
	require.NoError(t, err)

	// Start: both fail
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "fail", errors.New("err")
	})
	err = la.Start(ctx, id)
	assert.Error(t, err)

	// Stop: kill succeeds
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "", nil
	})
	err = la.Stop(ctx, id)
	require.NoError(t, err)

	// Stop: kill fails, stop succeeds
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "kill") {
			return "fail", errors.New("kill err")
		}
		return "", nil
	})
	err = la.Stop(ctx, id)
	require.NoError(t, err)

	// Stop: both kill and stop fail
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "fail", errors.New("stop err")
	})
	err = la.Stop(ctx, id)
	assert.Error(t, err)

	// Start and Stop when not available return nil
	la.AvailableFunc = func() bool { return false }
	err = la.Start(ctx, id)
	require.NoError(t, err)
	err = la.Stop(ctx, id)
	require.NoError(t, err)
	la.AvailableFunc = func() bool { return true }

	// Default runCmd when runner is nil
	laDefaultRunner := NewLaunchdAdapter()
	laDefaultRunner.CommandRunner = nil
	out, err := laDefaultRunner.runCmd(ctx, "echo", "cycle208")
	require.NoError(t, err)
	assert.Contains(t, string(out), "cycle208")

	// Restart
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "", nil
	})
	err = la.Restart(ctx, id)
	require.NoError(t, err)
}

func TestCycle208_LaunchdAdapter_StatusAndCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tempDir := t.TempDir()
	la := NewLaunchdAdapter()
	la.BaseDir = tempDir
	la.AvailableFunc = func() bool { return true }

	id := "com.test.status"
	plistFile := filepath.Join(tempDir, id+".plist")
	err := fileutil.WriteFile(plistFile, []byte("<plist></plist>"), paths.FilePerm644)
	require.NoError(t, err)

	// Status query returns running with PID
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "PID\tStatus\tLabel\n45678\t0\tcom.test.status\n", nil
	})
	st, err := la.Status(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, StateRunning, st.State)
	assert.Equal(t, 45678, st.PID)

	// Status query returns stopped if PID unparseable
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "-\t0\tcom.test.status\n", nil
	})
	st, err = la.Status(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, StateStopped, st.State)

	// Status query returns error if command fails
	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "error", errors.New("list failed")
	})
	_, err = la.Status(ctx, id)
	assert.Error(t, err)

	// CleanupLegacy with files and subdirectories
	subDir := filepath.Join(tempDir, "ignored-dir")
	err = fileutil.MkdirAll(subDir, paths.DirPerm755)
	require.NoError(t, err)

	nonPlist := filepath.Join(tempDir, "file.txt")
	err = fileutil.WriteFile(nonPlist, []byte("data"), paths.FilePerm644)
	require.NoError(t, err)

	legacyPlist := filepath.Join(tempDir, "legacy-svc.plist")
	err = fileutil.WriteFile(legacyPlist, []byte("<plist></plist>"), paths.FilePerm644)
	require.NoError(t, err)

	la.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "", nil
	})
	cleaned, err := la.CleanupLegacy(ctx, []string{"legacy-svc"})
	require.NoError(t, err)
	assert.Equal(t, []string{"legacy-svc"}, cleaned)
	assert.NoFileExists(t, legacyPlist)
}

func TestCycle208_SystemdAdapter_TemplatingAndPaths(t *testing.T) {
	sa := NewSystemdAdapter(false)
	assert.Equal(t, "systemd", sa.Name())

	// 1. IsAvailable fallback vs AvailableFunc
	sa.AvailableFunc = func() bool { return false }
	assert.False(t, sa.IsAvailable())
	sa.AvailableFunc = func() bool { return true }
	assert.True(t, sa.IsAvailable())

	// 2. getDir variations
	sa.UnitDir = ""
	sa.UserMode = false
	dir, err := sa.getDir()
	require.NoError(t, err)
	assert.Equal(t, "/etc/systemd/system", dir)

	sa.UserMode = true
	dir, err = sa.getDir()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(dir, filepath.Join(".config", "systemd", "user")))

	sa.UnitDir = "/custom/systemd/dir"
	dir, err = sa.getDir()
	require.NoError(t, err)
	assert.Equal(t, "/custom/systemd/dir", dir)

	// 3. systemctlArgs with and without UserMode
	sa.UserMode = true
	args := sa.systemctlArgs("status", "svc")
	assert.Equal(t, []string{"--user", "status", "svc"}, args)

	sa.UserMode = false
	args = sa.systemctlArgs("status", "svc")
	assert.Equal(t, []string{"status", "svc"}, args)

	// 4. unitName with and without .service suffix
	assert.Equal(t, "my-service.service", sa.unitName("my-service"))
	assert.Equal(t, "my-service.service", sa.unitName("my-service.service"))
}

func TestCycle208_SystemdAdapter_UnitContentVariations(t *testing.T) {
	sa := NewSystemdAdapter(false)

	// 1. Description fallback: Description when DisplayName is empty
	spec1 := ServiceSpec{
		ID:          "svc-desc",
		Description: "Fallback Description",
		Executable:  "/usr/bin/daemon",
	}
	content1 := sa.UnitContent(spec1)
	assert.Contains(t, content1, "Description=Fallback Description")
	assert.Contains(t, content1, "WantedBy=multi-user.target")
	assert.Contains(t, content1, "Restart=on-failure")

	// 2. Description fallback: ID when DisplayName and Description are empty
	spec2 := ServiceSpec{
		ID:            "svc-id-only",
		Executable:    "/usr/bin/daemon",
		RestartPolicy: RestartNever,
	}
	content2 := sa.UnitContent(spec2)
	assert.Contains(t, content2, "Description=svc-id-only")
	assert.Contains(t, content2, "Restart=no")

	// 3. KeepAlive = true sets Restart=always
	spec3 := ServiceSpec{
		ID:         "svc-keepalive",
		Executable: "/usr/bin/daemon",
		KeepAlive:  true,
	}
	content3 := sa.UnitContent(spec3)
	assert.Contains(t, content3, "Restart=always")
}

func TestCycle208_SystemdAdapter_InstallAndUninstall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sa := NewSystemdAdapter(true)

	// 1. Invalid spec
	err := sa.Install(ctx, ServiceSpec{})
	assert.Error(t, err)

	// 2. Unsupported platform when UnitDir empty and not available
	sa.AvailableFunc = func() bool { return false }
	sa.UnitDir = ""
	err = sa.Install(ctx, ServiceSpec{ID: "test", Executable: "/bin/true"})
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)

	// 3. Install with RunAtLoad=true and AvailableFunc=true
	tempDir := t.TempDir()
	sa.UnitDir = tempDir
	sa.AvailableFunc = func() bool { return true }

	var commands []string
	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return "", nil
	})

	spec := ServiceSpec{ID: "run-at-load-svc", Executable: "/bin/true", RunAtLoad: true}
	err = sa.Install(ctx, spec)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(tempDir, "run-at-load-svc.service"))
	assert.True(t, len(commands) >= 2)

	// Install with RunAtLoad=false
	specNoLoad := ServiceSpec{ID: "no-load-svc", Executable: "/bin/true", RunAtLoad: false}
	err = sa.Install(ctx, specNoLoad)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(tempDir, "no-load-svc.service"))

	// Default runCmd when runner is nil
	saDefaultRunner := NewSystemdAdapter(true)
	saDefaultRunner.CommandRunner = nil
	out, err := saDefaultRunner.runCmd(ctx, "echo", "cycle208-systemd")
	require.NoError(t, err)
	assert.Contains(t, string(out), "cycle208-systemd")

	// 4. Uninstall missing returns ErrServiceNotFound
	err = sa.Uninstall(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrServiceNotFound)

	// 5. Uninstall existing
	err = sa.Uninstall(ctx, "run-at-load-svc")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(tempDir, "run-at-load-svc.service"))
}

func TestCycle208_SystemdAdapter_LifecycleAndStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tempDir := t.TempDir()
	sa := NewSystemdAdapter(true)
	sa.UnitDir = tempDir
	sa.AvailableFunc = func() bool { return true }

	// Missing unit file returns error
	err := sa.Start(ctx, "missing")
	assert.ErrorIs(t, err, ErrServiceNotFound)
	err = sa.Stop(ctx, "missing")
	assert.ErrorIs(t, err, ErrServiceNotFound)
	err = sa.Restart(ctx, "missing")
	assert.ErrorIs(t, err, ErrServiceNotFound)

	// Write unit file
	id := "active-svc"
	unitFile := filepath.Join(tempDir, id+".service")
	err = fileutil.WriteFile(unitFile, []byte("[Unit]\n"), paths.FilePerm644)
	require.NoError(t, err)

	// Start success and failure
	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "", nil
	})
	err = sa.Start(ctx, id)
	require.NoError(t, err)

	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "fail", errors.New("start failed")
	})
	err = sa.Start(ctx, id)
	assert.Error(t, err)

	// Stop and Restart
	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "", nil
	})
	err = sa.Stop(ctx, id)
	require.NoError(t, err)
	err = sa.Restart(ctx, id)
	require.NoError(t, err)

	// Actions when not available return nil
	sa.AvailableFunc = func() bool { return false }
	err = sa.Start(ctx, id)
	require.NoError(t, err)
	err = sa.Stop(ctx, id)
	require.NoError(t, err)
	err = sa.Restart(ctx, id)
	require.NoError(t, err)
	stUnavailable, err := sa.Status(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, StateStopped, stUnavailable.State)
	sa.AvailableFunc = func() bool { return true }

	// Status: ActiveState=active, MainPID=8877
	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "ActiveState=active\nMainPID=8877\n", nil
	})
	st, err := sa.Status(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, StateRunning, st.State)
	assert.Equal(t, 8877, st.PID)

	// Status: ActiveState=inactive, MainPID=0, malformed lines
	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "ActiveState=inactive\nMainPID=0\nInvalidPropLine\n", nil
	})
	st, err = sa.Status(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, StateStopped, st.State)
	assert.Equal(t, 0, st.PID)

	// Status command failure
	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "error", errors.New("show failed")
	})
	_, err = sa.Status(ctx, id)
	assert.Error(t, err)
}

func TestCycle208_SystemdAdapter_CleanupLegacy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tempDir := t.TempDir()
	sa := NewSystemdAdapter(true)
	sa.UnitDir = tempDir
	sa.AvailableFunc = func() bool { return true }

	// Write units: one to clean, one to keep
	toClean := filepath.Join(tempDir, "legacy-daemon.service")
	err := fileutil.WriteFile(toClean, []byte("[Unit]\n"), paths.FilePerm644)
	require.NoError(t, err)

	toKeep := filepath.Join(tempDir, "modern-daemon.service")
	err = fileutil.WriteFile(toKeep, []byte("[Unit]\n"), paths.FilePerm644)
	require.NoError(t, err)

	sa.CommandRunner = mockCommandRunner(t, func(name string, args ...string) (string, error) {
		return "", nil
	})

	cleaned, err := sa.CleanupLegacy(ctx, []string{"legacy-daemon"})
	require.NoError(t, err)
	assert.Equal(t, []string{"legacy-daemon"}, cleaned)
	assert.NoFileExists(t, toClean)
	assert.FileExists(t, toKeep)
}

func TestCycle208_SupervisorAdapter_ExecutionAndErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sup := NewSupervisorAdapter()

	// 1. Invalid spec
	err := sup.Install(ctx, ServiceSpec{})
	assert.Error(t, err)

	// 2. Duplicate install
	spec := ServiceSpec{ID: "dup-test", Executable: "/bin/echo"}
	err = sup.Install(ctx, spec)
	require.NoError(t, err)
	err = sup.Install(ctx, spec)
	assert.ErrorIs(t, err, ErrServiceAlreadyExists)

	// 3. Start nonexistent
	err = sup.Start(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrServiceNotFound)

	// 4. Stop nonexistent
	err = sup.Stop(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrServiceNotFound)

	// 5. Stop non-running
	err = sup.Stop(ctx, "dup-test")
	assert.ErrorIs(t, err, ErrServiceNotRunning)

	// 6. Uninstall nonexistent
	err = sup.Uninstall(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrServiceNotFound)
}

func TestCycle208_SupervisorAdapter_ProcessLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tempDir := t.TempDir()
	stdoutFile := filepath.Join(tempDir, "proc.stdout")
	stderrFile := filepath.Join(tempDir, "proc.stderr")

	sup := NewSupervisorAdapter()
	spec := ServiceSpec{
		ID:                "lifecycle.proc",
		Executable:        "sleep",
		Arguments:         []string{"5"},
		WorkingDir:        tempDir,
		Environment:       map[string]string{"CYCLE": "208"},
		StandardOutPath:   stdoutFile,
		StandardErrorPath: stderrFile,
	}

	err := sup.Install(ctx, spec)
	require.NoError(t, err)

	// Start
	err = sup.Start(ctx, spec.ID)
	require.NoError(t, err)

	// Starting again returns ErrServiceAlreadyActive
	err = sup.Start(ctx, spec.ID)
	assert.ErrorIs(t, err, ErrServiceAlreadyActive)

	// Status running
	st, err := sup.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, StateRunning, st.State)
	assert.Greater(t, st.PID, 0)

	// Restart
	err = sup.Restart(ctx, spec.ID)
	require.NoError(t, err)

	// CleanupLegacy stops running process and removes spec
	cleaned, err := sup.CleanupLegacy(ctx, []string{spec.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{spec.ID}, cleaned)

	st, err = sup.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, StateStopped, st.State)
}

func TestCycle208_SupervisorAdapter_SpawnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sup := NewSupervisorAdapter()
	spec := ServiceSpec{
		ID:         "bad.exec",
		Executable: "/path/to/nonexistent/executable/binary",
	}

	err := sup.Install(ctx, spec)
	require.NoError(t, err)

	err = sup.Start(ctx, spec.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spawn process")
}

func TestCycle208_SupervisorAdapter_RunAtLoad(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sup := NewSupervisorAdapter()
	spec := ServiceSpec{
		ID:         "auto.run.service",
		Executable: "sleep",
		Arguments:  []string{"5"},
		RunAtLoad:  true,
	}

	err := sup.Install(ctx, spec)
	require.NoError(t, err)

	// Wait briefly for RunAtLoad goroutine to execute
	time.Sleep(100 * time.Millisecond)

	st, err := sup.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, StateRunning, st.State)

	err = sup.Uninstall(ctx, spec.ID)
	require.NoError(t, err)
}

func TestCycle208_Manager_AutoDetectAndValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. AutoDetectAdapterForOS cross-platform detection
	adDarwin := AutoDetectAdapterForOS("darwin")
	require.NotNil(t, adDarwin)
	if runtime.GOOS == "darwin" {
		assert.Equal(t, "launchd", adDarwin.Name())
	}

	adLinux := AutoDetectAdapterForOS("linux")
	require.NotNil(t, adLinux)

	adOther := AutoDetectAdapterForOS("windows")
	require.NotNil(t, adOther)
	assert.Equal(t, "supervisor", adOther.Name())

	// 2. Manager validation on empty IDs
	mock := NewMockAdapter("mgr-mock", true)
	mgr := NewManager(WithAdapter(mock))

	err := mgr.Install(ctx, ServiceSpec{})
	assert.Error(t, err)

	err = mgr.Uninstall(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidSpec)

	err = mgr.Start(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidSpec)

	err = mgr.Stop(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidSpec)

	err = mgr.Restart(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidSpec)

	_, err = mgr.Status(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidSpec)

	// Empty legacy IDs returns nil, nil
	cleaned, err := mgr.CleanupLegacy(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, cleaned)
}

func TestCycle208_Manager_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := NewMockAdapter("concurrent-mgr", true)
	mgr := NewManager(WithAdapter(mock))

	var wg sync.WaitGroup
	numWorkers := 8

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("cycle208_mgr_worker", "testing concurrent manager actions").
			AsControlPlane().
			StartSimple(func() {
				defer wg.Done()
				id := fmt.Sprintf("svc-%d", workerID)
				spec := ServiceSpec{ID: id, Executable: "/bin/echo"}

				installErr := mgr.Install(ctx, spec)
				require.NoError(t, installErr)

				startErr := mgr.Start(ctx, id)
				require.NoError(t, startErr)

				st, statusErr := mgr.Status(ctx, id)
				require.NoError(t, statusErr)
				assert.Equal(t, StateRunning, st.State)

				stopErr := mgr.Stop(ctx, id)
				require.NoError(t, stopErr)

				uninstErr := mgr.Uninstall(ctx, id)
				require.NoError(t, uninstErr)
			})
	}

	wg.Wait()
}

func TestCycle208_MockAdapter_AllErrorInjectors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := NewMockAdapter("err-mock", true)
	spec := ServiceSpec{ID: "err-svc", Executable: "/bin/echo"}

	// 1. Install error injector
	injectedErr := errors.New("injected fault")
	mock.InstallErr = injectedErr
	err := mock.Install(ctx, spec)
	assert.ErrorIs(t, err, injectedErr)
	mock.InstallErr = nil

	// Install validly
	err = mock.Install(ctx, spec)
	require.NoError(t, err)

	// Install with RunAtLoad=true
	specRunLoad := ServiceSpec{ID: "run-load-mock", Executable: "/bin/echo", RunAtLoad: true}
	err = mock.Install(ctx, specRunLoad)
	require.NoError(t, err)
	stRunLoad, err := mock.Status(ctx, specRunLoad.ID)
	require.NoError(t, err)
	assert.Equal(t, StateRunning, stRunLoad.State)
	assert.Equal(t, 12345, stRunLoad.PID)

	// 2. Start error injector
	mock.StartErr = injectedErr
	err = mock.Start(ctx, spec.ID)
	assert.ErrorIs(t, err, injectedErr)
	mock.StartErr = nil

	// Start validly
	err = mock.Start(ctx, spec.ID)
	require.NoError(t, err)

	// 3. Stop error injector
	mock.StopErr = injectedErr
	err = mock.Stop(ctx, spec.ID)
	assert.ErrorIs(t, err, injectedErr)
	mock.StopErr = nil

	// 4. Restart error injector
	mock.RestartErr = injectedErr
	err = mock.Restart(ctx, spec.ID)
	assert.ErrorIs(t, err, injectedErr)
	mock.RestartErr = nil

	// 5. Status error injector
	mock.StatusErr = injectedErr
	_, err = mock.Status(ctx, spec.ID)
	assert.ErrorIs(t, err, injectedErr)
	mock.StatusErr = nil

	// 6. Cleanup error injector
	mock.CleanupErr = injectedErr
	_, err = mock.CleanupLegacy(ctx, []string{spec.ID})
	assert.ErrorIs(t, err, injectedErr)
	mock.CleanupErr = nil

	// 7. Uninstall error injector
	mock.UninstallErr = injectedErr
	err = mock.Uninstall(ctx, spec.ID)
	assert.ErrorIs(t, err, injectedErr)
	mock.UninstallErr = nil
}
