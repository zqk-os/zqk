//go:build darwin
// +build darwin

package overseer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func currentUID() string {
	return strconv.Itoa(os.Getuid())
}

// LaunchAgentsDir returns the path to ~/Library/LaunchAgents.
func LaunchAgentsDir() (string, error) {
	home, err := fileutil.UserHomeDir()
	if err != nil {
		return "", errfmt.Newf("resolve user home dir").Wrap(err)
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

// CleanLegacyLaunchAgents scans ~/Library/LaunchAgents, unloads, and removes all legacy
// per-daemon LaunchAgents (com.zqk.scheduler.*, com.zqk.privileged-writer.*, com.zqk.mesh.*).
func CleanLegacyLaunchAgents() ([]string, error) {
	dir, err := LaunchAgentsDir()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errfmt.Newf("stat LaunchAgents dir %s", dir).Wrap(err)
	}

	var cleaned []string
	uid := currentUID()

	cleanInDir := func(d string) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".plist") || !strings.HasPrefix(name, "com.zqk.") {
				continue
			}
			label := strings.TrimSuffix(name, ".plist")
			// Do not clean the unified overseer agent
			if label == OverseerLaunchAgentLabel {
				continue
			}

			// Clean legacy zqk daemon agents: scheduler, privileged-writer, mesh, mcp, etc.
			isLegacy := strings.HasPrefix(label, "com.zqk.scheduler.") ||
				strings.HasPrefix(label, "com.zqk.privileged-writer") ||
				strings.HasPrefix(label, "com.zqk.mesh.") ||
				strings.HasPrefix(label, "com.zqk.mcp")

			if !isLegacy {
				continue
			}

			plistPath := filepath.Join(d, name)
			_ = execwrap.Command("launchctl", "bootout", "gui/"+uid+"/"+label).Run()
			_ = execwrap.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run()
			_ = execwrap.Command("launchctl", "unload", "-w", plistPath).Run()
			_ = fileutil.Remove(plistPath)
			cleaned = append(cleaned, label)
		}
	}

	cleanInDir(dir)
	cleanInDir(filepath.Join(dir, "disabled"))

	return cleaned, nil
}

// InstallOverseerLaunchAgent cleans legacy units, generates the com.zqk.overseer.plist,
// and bootstraps it under macOS launchd.
func InstallOverseerLaunchAgent(projectRoot, binaryPath string) (*LaunchAgentStatus, error) {
	cleaned, err := CleanLegacyLaunchAgents()
	if err != nil {
		return nil, errfmt.Newf("clean legacy launchagents").Wrap(err)
	}

	dir, err := LaunchAgentsDir()
	if err != nil {
		return nil, err
	}
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("mkdir %s", dir).Wrap(err)
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = projectRoot
	}

	bin := binaryPath
	if bin == "" {
		candidate := filepath.Join(absRoot, "bin", "zqk")
		if fileutil.Exists(candidate) {
			bin = candidate
		} else {
			bin = "zqk"
		}
	}

	home, _ := fileutil.UserHomeDir()
	envPath := zqkenv.OSPath().Get()
	if envPath == "" {
		envPath = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}

	logsDir := filepath.Join(absRoot, paths.ProjectDataDir, paths.LogsDir)
	_ = fileutil.MkdirAll(logsDir, paths.DirPerm755)

	stdoutLog := filepath.Join(logsDir, "overseer.launchd.out")
	stderrLog := filepath.Join(logsDir, "overseer.launchd.err")

	plistBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>daemon</string>
    <string>run</string>
  </array>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
    <key>PATH</key>
    <string>%s</string>
    <key>ZQK_PROJECT_ROOT</key>
    <string>%s</string>
    <key>ZQK_IS_DAEMON</key>
    <string>1</string>
    <key>ZQK_API_KEY</key>
    <string>ACC-SYSTEM</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key>
    <false/>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, OverseerLaunchAgentLabel, bin, absRoot, home, envPath, absRoot, stdoutLog, stderrLog)

	plistPath := filepath.Join(dir, OverseerLaunchAgentLabel+".plist")
	if err := fileutil.WriteFile(plistPath, []byte(plistBody), paths.FilePerm644); err != nil {
		return nil, errfmt.Newf("write %s", plistPath).Wrap(err)
	}

	uid := currentUID()
	_ = execwrap.Command("launchctl", "bootout", "gui/"+uid+"/"+OverseerLaunchAgentLabel).Run()
	_ = execwrap.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run()
	_ = execwrap.Command("launchctl", "unload", "-w", plistPath).Run()

	if err := execwrap.Command("launchctl", "bootstrap", "gui/"+uid, plistPath).Run(); err != nil {
		if err2 := execwrap.Command("launchctl", "load", "-w", plistPath).Run(); err2 != nil {
			return nil, errfmt.Newf("bootstrap/load launchagent %s", plistPath).Wrap(err)
		}
	}

	st, _ := StatusOverseerLaunchAgent()
	if st != nil {
		st.CleanedUnits = cleaned
		return st, nil
	}

	return &LaunchAgentStatus{
		Label:        OverseerLaunchAgentLabel,
		PlistPath:    plistPath,
		Installed:    true,
		Loaded:       true,
		CleanedUnits: cleaned,
	}, nil
}

// UninstallOverseerLaunchAgent unloads and removes the overseer LaunchAgent.
func UninstallOverseerLaunchAgent() error {
	dir, err := LaunchAgentsDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(dir, OverseerLaunchAgentLabel+".plist")
	uid := currentUID()

	_ = execwrap.Command("launchctl", "bootout", "gui/"+uid+"/"+OverseerLaunchAgentLabel).Run()
	_ = execwrap.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run()
	_ = execwrap.Command("launchctl", "unload", "-w", plistPath).Run()
	_ = fileutil.Remove(plistPath)
	return nil
}

// StatusOverseerLaunchAgent returns the status of the solitary overseer LaunchAgent.
func StatusOverseerLaunchAgent() (*LaunchAgentStatus, error) {
	dir, err := LaunchAgentsDir()
	if err != nil {
		return nil, err
	}
	plistPath := filepath.Join(dir, OverseerLaunchAgentLabel+".plist")
	installed := fileutil.Exists(plistPath)

	st := &LaunchAgentStatus{
		Label:     OverseerLaunchAgentLabel,
		PlistPath: plistPath,
		Installed: installed,
	}

	out, err := execwrap.Command("launchctl", "list").Output()
	if err != nil {
		return st, nil
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == OverseerLaunchAgentLabel {
			st.Loaded = true
			if pid, err := strconv.Atoi(fields[0]); err == nil && pid > 0 {
				st.Running = true
				st.PID = pid
			}
			break
		}
	}

	return st, nil
}
