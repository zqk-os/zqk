package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"github.com/zqk-os/zqk/pkg/paths"
)

// LaunchdAdapter implements ServiceAdapter using Apple's launchd daemon/agent system.
type LaunchdAdapter struct {
	// BaseDir overrides the default ~/Library/LaunchAgents directory if set (useful for tests).
	BaseDir string

	// CommandRunner executes commands (defaults to exec.CommandContext).
	CommandRunner func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewLaunchdAdapter constructs a new LaunchdAdapter.
func NewLaunchdAdapter() *LaunchdAdapter {
	return &LaunchdAdapter{
		CommandRunner: exec.CommandContext,
	}
}

func (a *LaunchdAdapter) Name() string {
	return "launchd"
}

func (a *LaunchdAdapter) IsAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("launchctl")
	return err == nil
}

func (a *LaunchdAdapter) getDir() (string, error) {
	if a.BaseDir != "" {
		return a.BaseDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func (a *LaunchdAdapter) runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	runner := a.CommandRunner
	if runner == nil {
		runner = exec.CommandContext
	}
	cmd := runner(ctx, name, args...)
	return cmd.CombinedOutput()
}

// PlistContent generates the XML plist definition for a ServiceSpec.
func (a *LaunchdAdapter) PlistContent(spec ServiceSpec) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")

	// Label
	b.WriteString(fmt.Sprintf("  <key>Label</key>\n  <string>%s</string>\n", spec.ID))

	// ProgramArguments
	b.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	b.WriteString(fmt.Sprintf("    <string>%s</string>\n", spec.Executable))
	for _, arg := range spec.Arguments {
		b.WriteString(fmt.Sprintf("    <string>%s</string>\n", arg))
	}
	b.WriteString("  </array>\n")

	// WorkingDirectory
	if spec.WorkingDir != "" {
		b.WriteString(fmt.Sprintf("  <key>WorkingDirectory</key>\n  <string>%s</string>\n", spec.WorkingDir))
	}

	// EnvironmentVariables
	if len(spec.Environment) > 0 {
		b.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
		for k, v := range spec.Environment {
			b.WriteString(fmt.Sprintf("    <key>%s</key>\n    <string>%s</string>\n", k, v))
		}
		b.WriteString("  </dict>\n")
	}

	// RunAtLoad
	if spec.RunAtLoad {
		b.WriteString("  <key>RunAtLoad</key>\n  <true/>\n")
	} else {
		b.WriteString("  <key>RunAtLoad</key>\n  <false/>\n")
	}

	// KeepAlive
	if spec.KeepAlive || spec.RestartPolicy == RestartAlways {
		b.WriteString("  <key>KeepAlive</key>\n  <true/>\n")
	} else if spec.RestartPolicy == RestartOnFailure {
		b.WriteString("  <key>KeepAlive</key>\n  <dict>\n    <key>SuccessfulExit</key>\n    <false/>\n  </dict>\n")
	}

	// StandardOutPath & StandardErrorPath
	if spec.StandardOutPath != "" {
		b.WriteString(fmt.Sprintf("  <key>StandardOutPath</key>\n  <string>%s</string>\n", spec.StandardOutPath))
	}
	if spec.StandardErrorPath != "" {
		b.WriteString(fmt.Sprintf("  <key>StandardErrorPath</key>\n  <string>%s</string>\n", spec.StandardErrorPath))
	}

	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func (a *LaunchdAdapter) Install(ctx context.Context, spec ServiceSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if !a.IsAvailable() && a.BaseDir == "" {
		return ErrUnsupportedPlatform
	}

	dir, err := a.getDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return fmt.Errorf("create launchd dir %s: %w", dir, err)
	}

	plistPath := filepath.Join(dir, spec.ID+".plist")
	content := a.PlistContent(spec)
	if err := os.WriteFile(plistPath, []byte(content), paths.FilePerm644); err != nil {
		return fmt.Errorf("write plist %s: %w", plistPath, err)
	}

	if a.IsAvailable() {
		uid := strconv.Itoa(os.Getuid())
		_, _ = a.runCmd(ctx, "launchctl", "bootout", "gui/"+uid+"/"+spec.ID)
		_, _ = a.runCmd(ctx, "launchctl", "bootout", "gui/"+uid, plistPath)
		_, _ = a.runCmd(ctx, "launchctl", "unload", "-w", plistPath)

		if _, err := a.runCmd(ctx, "launchctl", "bootstrap", "gui/"+uid, plistPath); err != nil {
			if _, err2 := a.runCmd(ctx, "launchctl", "load", "-w", plistPath); err2 != nil {
				return fmt.Errorf("bootstrap/load plist %s: %w", plistPath, err)
			}
		}
	}

	return nil
}

func (a *LaunchdAdapter) Uninstall(ctx context.Context, id string) error {
	dir, err := a.getDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(dir, id+".plist")

	if a.IsAvailable() {
		uid := strconv.Itoa(os.Getuid())
		_, _ = a.runCmd(ctx, "launchctl", "bootout", "gui/"+uid+"/"+id)
		_, _ = a.runCmd(ctx, "launchctl", "bootout", "gui/"+uid, plistPath)
		_, _ = a.runCmd(ctx, "launchctl", "unload", "-w", plistPath)
	}

	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist %s: %w", plistPath, err)
	}
	return nil
}

func (a *LaunchdAdapter) Start(ctx context.Context, id string) error {
	dir, err := a.getDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(dir, id+".plist")
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	if !a.IsAvailable() {
		return nil
	}

	uid := strconv.Itoa(os.Getuid())
	if _, err := a.runCmd(ctx, "launchctl", "kickstart", "-k", "gui/"+uid+"/"+id); err != nil {
		if _, err2 := a.runCmd(ctx, "launchctl", "start", id); err2 != nil {
			return fmt.Errorf("start service %s: %w", id, err)
		}
	}
	return nil
}

func (a *LaunchdAdapter) Stop(ctx context.Context, id string) error {
	dir, err := a.getDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(dir, id+".plist")
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}

	if !a.IsAvailable() {
		return nil
	}

	uid := strconv.Itoa(os.Getuid())
	if _, err := a.runCmd(ctx, "launchctl", "kill", "SIGTERM", "gui/"+uid+"/"+id); err != nil {
		if _, err2 := a.runCmd(ctx, "launchctl", "stop", id); err2 != nil {
			return fmt.Errorf("stop service %s: %w", id, err)
		}
	}
	return nil
}

func (a *LaunchdAdapter) Restart(ctx context.Context, id string) error {
	if err := a.Stop(ctx, id); err != nil && !errors.Is(err, ErrServiceNotRunning) {
		// Ignore not running on restart
	}
	return a.Start(ctx, id)
}

func (a *LaunchdAdapter) Status(ctx context.Context, id string) (ServiceStatus, error) {
	st := ServiceStatus{
		ID:    id,
		State: StateStopped,
	}

	dir, err := a.getDir()
	if err != nil {
		return st, err
	}
	plistPath := filepath.Join(dir, id+".plist")
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return st, nil
	}

	if !a.IsAvailable() {
		return st, nil
	}

	out, err := a.runCmd(ctx, "launchctl", "list")
	if err != nil {
		return st, fmt.Errorf("query launchctl list: %w", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == id {
			if pid, err := strconv.Atoi(fields[0]); err == nil && pid > 0 {
				st.State = StateRunning
				st.PID = pid
			}
			break
		}
	}

	return st, nil
}

func (a *LaunchdAdapter) CleanupLegacy(ctx context.Context, legacyIDs []string) ([]string, error) {
	dir, err := a.getDir()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	targetSet := make(map[string]struct{}, len(legacyIDs))
	for _, id := range legacyIDs {
		targetSet[id] = struct{}{}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	var cleaned []string
	uid := strconv.Itoa(os.Getuid())

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".plist") {
			continue
		}
		label := strings.TrimSuffix(entry.Name(), ".plist")
		if _, matches := targetSet[label]; !matches {
			continue
		}

		plistPath := filepath.Join(dir, entry.Name())
		if a.IsAvailable() {
			_, _ = a.runCmd(ctx, "launchctl", "bootout", "gui/"+uid+"/"+label)
			_, _ = a.runCmd(ctx, "launchctl", "bootout", "gui/"+uid, plistPath)
			_, _ = a.runCmd(ctx, "launchctl", "unload", "-w", plistPath)
		}
		_ = os.Remove(plistPath)
		cleaned = append(cleaned, label)
	}

	return cleaned, nil
}
