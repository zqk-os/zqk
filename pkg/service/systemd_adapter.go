package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SystemdAdapter implements ServiceAdapter using Linux systemd (user or system mode).
type SystemdAdapter struct {
	// UnitDir overrides the default systemd unit file directory (useful for tests).
	UnitDir string

	// UserMode specifies if systemctl --user is used instead of system-wide units.
	UserMode bool

	// CommandRunner executes commands (defaults to exec.CommandContext).
	CommandRunner func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewSystemdAdapter creates a systemd adapter defaulting to user unit mode.
func NewSystemdAdapter(userMode bool) *SystemdAdapter {
	return &SystemdAdapter{
		UserMode:      userMode,
		CommandRunner: exec.CommandContext,
	}
}

func (a *SystemdAdapter) Name() string {
	return "systemd"
}

func (a *SystemdAdapter) IsAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func (a *SystemdAdapter) getDir() (string, error) {
	if a.UnitDir != "" {
		return a.UnitDir, nil
	}
	if a.UserMode {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home directory: %w", err)
		}
		return filepath.Join(home, ".config", "systemd", "user"), nil
	}
	return "/etc/systemd/system", nil
}

func (a *SystemdAdapter) runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	runner := a.CommandRunner
	if runner == nil {
		runner = exec.CommandContext
	}
	cmd := runner(ctx, name, args...)
	return cmd.CombinedOutput()
}

func (a *SystemdAdapter) systemctlArgs(extra ...string) []string {
	var args []string
	if a.UserMode {
		args = append(args, "--user")
	}
	return append(args, extra...)
}

// UnitContent generates the standard systemd unit file content.
func (a *SystemdAdapter) UnitContent(spec ServiceSpec) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	if spec.DisplayName != "" {
		b.WriteString(fmt.Sprintf("Description=%s\n", spec.DisplayName))
	} else if spec.Description != "" {
		b.WriteString(fmt.Sprintf("Description=%s\n", spec.Description))
	} else {
		b.WriteString(fmt.Sprintf("Description=%s\n", spec.ID))
	}
	b.WriteString("After=network.target\n\n")

	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")

	execLine := spec.Executable
	if len(spec.Arguments) > 0 {
		execLine += " " + strings.Join(spec.Arguments, " ")
	}
	b.WriteString(fmt.Sprintf("ExecStart=%s\n", execLine))

	if spec.WorkingDir != "" {
		b.WriteString(fmt.Sprintf("WorkingDirectory=%s\n", spec.WorkingDir))
	}

	for k, v := range spec.Environment {
		b.WriteString(fmt.Sprintf("Environment=\"%s=%s\"\n", k, v))
	}

	if spec.StandardOutPath != "" {
		b.WriteString(fmt.Sprintf("StandardOutput=append:%s\n", spec.StandardOutPath))
	}
	if spec.StandardErrorPath != "" {
		b.WriteString(fmt.Sprintf("StandardError=append:%s\n", spec.StandardErrorPath))
	}

	restart := "on-failure"
	if spec.RestartPolicy == RestartAlways || spec.KeepAlive {
		restart = "always"
	} else if spec.RestartPolicy == RestartNever {
		restart = "no"
	}
	b.WriteString(fmt.Sprintf("Restart=%s\n", restart))
	b.WriteString("RestartSec=5s\n\n")

	b.WriteString("[Install]\n")
	if a.UserMode {
		b.WriteString("WantedBy=default.target\n")
	} else {
		b.WriteString("WantedBy=multi-user.target\n")
	}

	return b.String()
}

func (a *SystemdAdapter) unitName(id string) string {
	if !strings.HasSuffix(id, ".service") {
		return id + ".service"
	}
	return id
}

func (a *SystemdAdapter) Install(ctx context.Context, spec ServiceSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if !a.IsAvailable() && a.UnitDir == "" {
		return ErrUnsupportedPlatform
	}

	dir, err := a.getDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return fmt.Errorf("create unit dir %s: %w", dir, err)
	}

	unitFile := filepath.Join(dir, a.unitName(spec.ID))
	content := a.UnitContent(spec)
	if err := fileutil.WriteFile(unitFile, []byte(content), paths.FilePerm644); err != nil {
		return fmt.Errorf("write systemd unit %s: %w", unitFile, err)
	}

	if a.IsAvailable() {
		_, _ = a.runCmd(ctx, "systemctl", a.systemctlArgs("daemon-reload")...)
		if spec.RunAtLoad {
			_, _ = a.runCmd(ctx, "systemctl", a.systemctlArgs("enable", "--now", a.unitName(spec.ID))...)
		} else {
			_, _ = a.runCmd(ctx, "systemctl", a.systemctlArgs("enable", a.unitName(spec.ID))...)
		}
	}

	return nil
}

func (a *SystemdAdapter) serviceUnitFile(id string) (string, error) {
	dir, err := a.getDir()
	if err != nil {
		return "", err
	}
	unitFile := filepath.Join(dir, a.unitName(id))
	if _, err := os.Stat(unitFile); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %s", ErrServiceNotFound, id)
	}
	return unitFile, nil
}

func (a *SystemdAdapter) systemctlAction(ctx context.Context, action, id string) error {
	if _, err := a.serviceUnitFile(id); err != nil {
		return err
	}
	if !a.IsAvailable() {
		return nil
	}
	if _, err := a.runCmd(ctx, "systemctl", a.systemctlArgs(action, a.unitName(id))...); err != nil {
		return fmt.Errorf("%s unit %s: %w", action, id, err)
	}
	return nil
}

func (a *SystemdAdapter) Uninstall(ctx context.Context, id string) error {
	unitFile, err := a.serviceUnitFile(id)
	if a.IsAvailable() {
		_, _ = a.runCmd(ctx, "systemctl", a.systemctlArgs("disable", "--now", a.unitName(id))...)
		_, _ = a.runCmd(ctx, "systemctl", a.systemctlArgs("daemon-reload")...)
	}
	if err != nil {
		return err
	}
	if removeErr := os.Remove(unitFile); removeErr != nil && !os.IsNotExist(removeErr) {
		return fmt.Errorf("remove unit file %s: %w", unitFile, removeErr)
	}
	return nil
}

func (a *SystemdAdapter) Start(ctx context.Context, id string) error {
	return a.systemctlAction(ctx, "start", id)
}

func (a *SystemdAdapter) Stop(ctx context.Context, id string) error {
	return a.systemctlAction(ctx, "stop", id)
}

func (a *SystemdAdapter) Restart(ctx context.Context, id string) error {
	return a.systemctlAction(ctx, "restart", id)
}

func (a *SystemdAdapter) Status(ctx context.Context, id string) (ServiceStatus, error) {
	dir, err := a.getDir()
	st, shouldQuery, err := checkUnitFilePresent(dir, err, a.unitName(id), a.IsAvailable(), id)
	if err != nil || !shouldQuery {
		return st, err
	}

	out, err := a.runCmd(ctx, "systemctl", a.systemctlArgs("show", a.unitName(id), "--property=ActiveState,MainPID")...)
	if err != nil {
		return st, fmt.Errorf("show unit %s: %w", id, err)
	}

	props := strings.Split(string(out), "\n")
	for _, line := range props {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "ActiveState":
			if parts[1] == "active" {
				st.State = StateRunning
			} else {
				st.State = StateStopped
			}
		case "MainPID":
			if pid, err := strconv.Atoi(parts[1]); err == nil && pid > 0 {
				st.PID = pid
			}
		}
	}

	return st, nil
}

func (a *SystemdAdapter) CleanupLegacy(ctx context.Context, legacyIDs []string) ([]string, error) {
	dir, err := a.getDir()
	if err != nil {
		return nil, err
	}
	entries, err := readValidServiceDir(dir)
	if err != nil || entries == nil {
		return nil, err
	}

	targetSet := make(map[string]struct{}, len(legacyIDs)*2)
	for _, id := range legacyIDs {
		targetSet[a.unitName(id)] = struct{}{}
		targetSet[id] = struct{}{}
	}

	var cleaned []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".service") {
			continue
		}
		rawName := strings.TrimSuffix(entry.Name(), ".service")
		if _, matches := targetSet[entry.Name()]; !matches {
			if _, matchesRaw := targetSet[rawName]; !matchesRaw {
				continue
			}
		}

		unitPath := filepath.Join(dir, entry.Name())
		if a.IsAvailable() {
			_, _ = a.runCmd(ctx, "systemctl", a.systemctlArgs("disable", "--now", entry.Name())...)
		}
		_ = os.Remove(unitPath)
		cleaned = append(cleaned, rawName)
	}

	if a.IsAvailable() && len(cleaned) > 0 {
		_, _ = a.runCmd(ctx, "systemctl", a.systemctlArgs("daemon-reload")...)
	}

	return cleaned, nil
}
