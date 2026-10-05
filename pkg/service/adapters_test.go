package service_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/service"
)

func TestLaunchdAdapter_PlistGeneration(t *testing.T) {
	adapter := service.NewLaunchdAdapter()
	spec := service.ServiceSpec{
		ID:                "com.example.testservice",
		DisplayName:       "Test Service",
		Executable:        "/usr/local/bin/test",
		Arguments:         []string{"--foo", "bar"},
		WorkingDir:        "/tmp/testdir",
		Environment:       map[string]string{"ENV_VAR": "val1"},
		StandardOutPath:   "/tmp/test.out",
		StandardErrorPath: "/tmp/test.err",
		RunAtLoad:         true,
		KeepAlive:         true,
	}

	content := adapter.PlistContent(spec)
	assert.Contains(t, content, "<key>Label</key>\n  <string>com.example.testservice</string>")
	assert.Contains(t, content, "<string>/usr/local/bin/test</string>")
	assert.Contains(t, content, "<string>--foo</string>")
	assert.Contains(t, content, "<string>bar</string>")
	assert.Contains(t, content, "<key>WorkingDirectory</key>\n  <string>/tmp/testdir</string>")
	assert.Contains(t, content, "<key>ENV_VAR</key>\n    <string>val1</string>")
	assert.Contains(t, content, "<key>RunAtLoad</key>\n  <true/>")
	assert.Contains(t, content, "<key>KeepAlive</key>\n  <true/>")
	assert.Contains(t, content, "<key>StandardOutPath</key>\n  <string>/tmp/test.out</string>")
}

func TestLaunchdAdapter_FileOperations(t *testing.T) {
	tempDir := t.TempDir()

	adapter := service.NewLaunchdAdapter()
	adapter.BaseDir = tempDir

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	spec := service.ServiceSpec{
		ID:         "com.example.filetest",
		Executable: "/usr/bin/true",
	}

	// Install creates plist file
	err := adapter.Install(ctx, spec)
	require.NoError(t, err)

	plistFile := filepath.Join(tempDir, spec.ID+".plist")
	assert.FileExists(t, plistFile)

	// Status sees stopped (since not loaded in mock dir)
	st, err := adapter.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, service.StateStopped, st.State)

	// Uninstall removes file
	err = adapter.Uninstall(ctx, spec.ID)
	require.NoError(t, err)
	assert.NoFileExists(t, plistFile)
}

func TestSystemdAdapter_UnitGeneration(t *testing.T) {
	adapter := service.NewSystemdAdapter(true)
	spec := service.ServiceSpec{
		ID:                "test-service",
		DisplayName:       "Test Daemon",
		Executable:        "/usr/local/bin/test",
		Arguments:         []string{"daemon", "--flag"},
		WorkingDir:        "/tmp/work",
		Environment:       map[string]string{"KEY": "VAL"},
		StandardOutPath:   "/tmp/test.out",
		StandardErrorPath: "/tmp/test.err",
		RestartPolicy:     service.RestartAlways,
	}

	content := adapter.UnitContent(spec)
	assert.Contains(t, content, "Description=Test Daemon")
	assert.Contains(t, content, "ExecStart=/usr/local/bin/test daemon --flag")
	assert.Contains(t, content, "WorkingDirectory=/tmp/work")
	assert.Contains(t, content, `Environment="KEY=VAL"`)
	assert.Contains(t, content, "StandardOutput=append:/tmp/test.out")
	assert.Contains(t, content, "Restart=always")
	assert.Contains(t, content, "WantedBy=default.target")
}

func TestSystemdAdapter_FileOperations(t *testing.T) {
	tempDir := t.TempDir()

	adapter := service.NewSystemdAdapter(true)
	adapter.UnitDir = tempDir

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	spec := service.ServiceSpec{
		ID:         "example-systemd-test",
		Executable: "/bin/true",
	}

	err := adapter.Install(ctx, spec)
	require.NoError(t, err)

	unitFile := filepath.Join(tempDir, "example-systemd-test.service")
	assert.FileExists(t, unitFile)

	cleaned, err := adapter.CleanupLegacy(ctx, []string{"example-systemd-test"})
	require.NoError(t, err)
	assert.Equal(t, []string{"example-systemd-test"}, cleaned)
	assert.NoFileExists(t, unitFile)
}

func TestSupervisorAdapter_Lifecycle(t *testing.T) {
	sup := service.NewSupervisorAdapter()
	assert.Equal(t, "supervisor", sup.Name())
	assert.True(t, sup.IsAvailable())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	spec := service.ServiceSpec{
		ID:         "local.supervisor.sleep",
		Executable: "sleep",
		Arguments:  []string{"10"},
	}

	// 1. Install
	err := sup.Install(ctx, spec)
	require.NoError(t, err)

	// 2. Start
	err = sup.Start(ctx, spec.ID)
	require.NoError(t, err)

	// Wait briefly for process to register
	time.Sleep(50 * time.Millisecond)

	st, err := sup.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, service.StateRunning, st.State)
	assert.Greater(t, st.PID, 0)

	// 3. Stop
	err = sup.Stop(ctx, spec.ID)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	st, err = sup.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, service.StateStopped, st.State)

	// 4. Uninstall
	err = sup.Uninstall(ctx, spec.ID)
	require.NoError(t, err)
}

func TestAdapter_OSAvailabilityGuardrails(t *testing.T) {
	ld := service.NewLaunchdAdapter()
	sd := service.NewSystemdAdapter(true)

	if runtime.GOOS == "darwin" {
		assert.True(t, ld.IsAvailable())
		assert.False(t, sd.IsAvailable())
	} else if runtime.GOOS == "linux" {
		assert.False(t, ld.IsAvailable())
	}
}
