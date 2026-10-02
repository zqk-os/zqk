package hostservice

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// HostServiceUnit represents an individual workshop host service.
type HostServiceUnit struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	PID          int    `json:"pid,omitempty"`
	Running      bool   `json:"running"`
	DesiredState string `json:"desired_state"`
	Details      string `json:"details,omitempty"`
}

// RuntimeWranglerReport is the consolidated status report of all workshop host units.
type RuntimeWranglerReport struct {
	ProjectRoot string            `json:"project_root"`
	RootID      string            `json:"root_id"`
	Timestamp   time.Time         `json:"timestamp"`
	Units       []HostServiceUnit `json:"units"`
}

// RuntimeWrangler provides unified start/stop/status control across all host services for a root.
type RuntimeWrangler struct {
	AbsRoot string
}

// NewRuntimeWrangler creates a new RuntimeWrangler for the given root.
func NewRuntimeWrangler(absRoot string) *RuntimeWrangler {
	clean, _ := filepath.Abs(absRoot)
	if clean == "" {
		clean = absRoot
	}
	return &RuntimeWrangler{AbsRoot: clean}
}

// Status inspects scheduler host unit, privileged-writer, MCP daemon, and mesh LaunchAgents.
func (w *RuntimeWrangler) Status() (*RuntimeWranglerReport, error) {
	rootID := RootID(w.AbsRoot)
	report := &RuntimeWranglerReport{
		ProjectRoot: w.AbsRoot,
		RootID:      rootID,
		Timestamp:   time.Now().UTC(),
		Units:       make([]HostServiceUnit, 0),
	}

	// 1. Scheduler host service
	schedUnit := HostServiceUnit{
		Name:         "scheduler",
		Label:        UnitLabel(rootID),
		Kind:         "supervisor_unit",
		DesiredState: DesiredStateAbsent,
	}
	if entry, err := ResolveEntry(w.AbsRoot); err == nil {
		schedUnit.DesiredState = entry.DesiredState
		st, stErr := NewAdapter().Status(entry)
		if stErr == nil {
			schedUnit.Details = st
			if strings.Contains(strings.ToLower(st), "running") {
				schedUnit.Running = true
			}
		}
	} else {
		// Check process fallback via daemon lock / PID
		pidFile := filepath.Join(w.AbsRoot, paths.ProjectDataDir, paths.StateDir, "scheduler.pid")
		if pid, alive := readPidFile(pidFile); alive {
			schedUnit.Running = true
			schedUnit.PID = pid
			schedUnit.Details = "active via pidfile"
		}
	}
	report.Units = append(report.Units, schedUnit)

	// 2. Privileged Writer
	pwUnit := HostServiceUnit{
		Name:         "privileged-writer",
		Label:        "com.zqk.privileged-writer",
		Kind:         "launch_agent",
		DesiredState: DesiredStateEnabled,
	}
	pwPID, pwRunning, pwDetails := inspectLaunchAgent(pwUnit.Label)
	pwUnit.PID = pwPID
	pwUnit.Running = pwRunning
	pwUnit.Details = pwDetails
	report.Units = append(report.Units, pwUnit)

	// 3. MCP Daemon
	mcpUnit := HostServiceUnit{
		Name:         "mcp-daemon",
		Label:        "com.zqk.mcp",
		Kind:         "tcp_daemon",
		DesiredState: DesiredStateEnabled,
	}
	mcpPidFile := filepath.Join(w.AbsRoot, paths.ProjectDataDir, "mcp", "daemon-8443.pid")
	if pid, alive := readPidFile(mcpPidFile); alive {
		mcpUnit.Running = true
		mcpUnit.PID = pid
		mcpUnit.Details = "port 8443 open"
	} else {
		// Port probe
		conn, err := net.DialTimeout("tcp", "127.0.0.1:8443", 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			mcpUnit.Running = true
			mcpUnit.Details = "port 8443 open (external PID)"
		} else {
			mcpUnit.Running = false
			mcpUnit.Details = "stopped"
		}
	}
	report.Units = append(report.Units, mcpUnit)

	// 4. Mesh Night-Duty LaunchAgent
	ndUnit := HostServiceUnit{
		Name:         "mesh-night-duty",
		Label:        "com.zqk.mesh.night-duty",
		Kind:         "launch_agent",
		DesiredState: DesiredStateEnabled,
	}
	ndPID, ndRunning, ndDetails := inspectLaunchAgent(ndUnit.Label)
	ndUnit.PID = ndPID
	ndUnit.Running = ndRunning
	ndUnit.Details = ndDetails
	report.Units = append(report.Units, ndUnit)

	// 5. Mesh TPM Watchdog LaunchAgent
	wdUnit := HostServiceUnit{
		Name:         "mesh-tpm-agy-watchdog",
		Label:        "com.zqk.mesh.tpm-agy-watchdog",
		Kind:         "launch_agent",
		DesiredState: DesiredStateEnabled,
	}
	wdPID, wdRunning, wdDetails := inspectLaunchAgent(wdUnit.Label)
	wdUnit.PID = wdPID
	wdUnit.Running = wdRunning
	wdUnit.Details = wdDetails
	report.Units = append(report.Units, wdUnit)

	return report, nil
}

// Stop enforces sticky stop across all workshop services for this root.
func (w *RuntimeWrangler) Stop() error {
	rootID := RootID(w.AbsRoot)

	// 1. Scheduler service stop & disable desired state
	if entry, err := ResolveEntry(w.AbsRoot); err == nil {
		_ = NewAdapter().Stop(entry)
		_, _ = SetEntryDesiredState(entry.RootID, DesiredStateDisabled)
	}
	// Also stop local scheduler PID if running
	pidFile := filepath.Join(w.AbsRoot, paths.ProjectDataDir, paths.StateDir, "scheduler.pid")
	if pid, alive := readPidFile(pidFile); alive {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	_ = fileutil.Remove(pidFile)

	// 2. Stop MCP Daemon
	mcpPidFile := filepath.Join(w.AbsRoot, paths.ProjectDataDir, "mcp", "daemon-8443.pid")
	if pid, alive := readPidFile(mcpPidFile); alive {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	_ = fileutil.Remove(mcpPidFile)

	// 3. Stop LaunchAgents if darwin
	if runtime.GOOS == "darwin" {
		home, _ := fileutil.UserHomeDir()
		labels := []string{
			UnitLabel(rootID),
			"com.zqk.mesh.night-duty",
			"com.zqk.mesh.tpm-agy-watchdog",
		}
		for _, label := range labels {
			plist := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
			_ = execwrap.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), plist).Run()
			_ = execwrap.Command("launchctl", "unload", "-w", plist).Run()
		}
	}

	return nil
}

// Start initiates all enabled workshop services for this root.
func (w *RuntimeWrangler) Start() error {
	// 1. Scheduler service start & enable desired state
	if entry, err := ResolveEntry(w.AbsRoot); err == nil {
		if err := NewAdapter().Start(entry); err != nil {
			return errfmt.Newf("start scheduler host service").Wrap(err)
		}
		_, _ = SetEntryDesiredState(entry.RootID, DesiredStateEnabled)
	}
	return nil
}

func readPidFile(path string) (int, bool) {
	pid, ok := fileutil.ReadPIDFile(path)
	if !ok {
		return 0, false
	}
	// Check alive
	err := syscall.Kill(pid, 0)
	return pid, err == nil
}

func inspectLaunchAgent(label string) (int, bool, string) {
	if runtime.GOOS != "darwin" {
		return 0, false, "unsupported OS"
	}
	out, err := execwrap.Command("launchctl", "list").Output()
	if err != nil {
		return 0, false, "launchctl error"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, label) {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				pidStr := parts[0]
				statusStr := parts[1]
				if pidStr != "-" {
					if pid, err := strconv.Atoi(pidStr); err == nil {
						return pid, true, fmt.Sprintf("running (last_exit=%s)", statusStr)
					}
				}
				return 0, false, fmt.Sprintf("loaded (last_exit=%s)", statusStr)
			}
			return 0, false, "loaded"
		}
	}
	return 0, false, "not loaded"
}

// FormatReportJSON converts report to JSON bytes.
func FormatReportJSON(r *RuntimeWranglerReport) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
