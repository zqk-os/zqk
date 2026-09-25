package overseer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/service"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// OverseerServiceSpec constructs the canonical ServiceSpec for the unified overseer process.
func OverseerServiceSpec(absRoot, binaryPath string) service.ServiceSpec {
	bin := binaryPath
	if bin == "" {
		candidate := filepath.Join(absRoot, "bin", "zqk")
		if fileutil.Exists(candidate) {
			bin = candidate
		} else {
			bin = "zqk"
		}
	}

	envPath := zqkenv.OSPath().Get()
	if envPath == "" {
		envPath = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}
	home, _ := fileutil.UserHomeDir()

	logsDir := filepath.Join(absRoot, paths.ProjectDataDir, paths.LogsDir)
	stdoutLog := filepath.Join(logsDir, "overseer.launchd.out")
	stderrLog := filepath.Join(logsDir, "overseer.launchd.err")

	return service.ServiceSpec{
		ID:          OverseerLaunchAgentLabel,
		DisplayName: "ZQK Process Group Overseer",
		Description: "Solitary host-level supervisor managing all ZQK background daemons under a unified process group.",
		Executable:  bin,
		Arguments:   []string{"daemon", "run", "--timeout", "0"},
		WorkingDir:  absRoot,
		Environment: map[string]string{
			"HOME":             home,
			"PATH":             envPath,
			"ZQK_PROJECT_ROOT": absRoot,
			"ZQK_IS_DAEMON":    "1",
			"ZQK_API_KEY":      "ACC-SYSTEM",
		},
		StandardOutPath: stdoutLog,
		StandardErrorPath: stderrLog,
		RunAtLoad:       true,
		KeepAlive:       true,
		RestartPolicy:   service.RestartOnFailure,
	}
}

const (
	// OverseerLaunchAgentLabel is the canonical macOS launchd label for the solitary overseer daemon.
	OverseerLaunchAgentLabel = "com.zqk.overseer"
)

// LaunchAgentStatus captures the host launchd state for the overseer.
type LaunchAgentStatus struct {
	Label        string   `json:"label"`
	PlistPath    string   `json:"plist_path"`
	Installed    bool     `json:"installed"`
	Loaded       bool     `json:"loaded"`
	Running      bool     `json:"running"`
	PID          int      `json:"pid,omitempty"`
	CleanedUnits []string `json:"cleaned_legacy_units,omitempty"`
}

// RootID returns a stable identifier for a project root directory.
func RootID(absRoot string) string {
	canon := filepath.Clean(absRoot)
	sum := sha256.Sum256([]byte(brand.ExecutableName() + "\x00" + canon))
	return hex.EncodeToString(sum[:8])
}

// OverseerUnitLabel returns the OS service unit label for the root directory.
func OverseerUnitLabel(absRoot string) string {
	return "com." + brand.ExecutableName() + ".overseer." + RootID(absRoot)
}

// GenerateLaunchdPlist returns the launchd plist definition for the overseer.
func GenerateLaunchdPlist(absRoot, exePath string) string {
	label := OverseerUnitLabel(absRoot)
	stdout := filepath.Join(absRoot, paths.ProjectDataDir, paths.LogsDir, "overseer.stdout.log")
	stderr := filepath.Join(absRoot, paths.ProjectDataDir, paths.LogsDir, "overseer.stderr.log")
	home, _ := fileutil.UserHomeDir()

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
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
    <key>ZQK_PROJECT_ROOT</key>
    <string>%s</string>
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
`, label, exePath, absRoot, home, absRoot, stdout, stderr)
}

// GenerateSystemdService returns the systemd unit definition for the overseer.
func GenerateSystemdService(absRoot, exePath string) string {
	stdout := filepath.Join(absRoot, paths.ProjectDataDir, paths.LogsDir, "overseer.stdout.log")
	stderr := filepath.Join(absRoot, paths.ProjectDataDir, paths.LogsDir, "overseer.stderr.log")

	return fmt.Sprintf(`[Unit]
Description=ZQK Process Group Overseer (%s)
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s daemon run
Restart=on-failure
RestartSec=5s
KillMode=process
StandardOutput=append:%s
StandardError=append:%s
Environment=ZQK_PROJECT_ROOT=%s

[Install]
WantedBy=default.target
`, absRoot, absRoot, exePath, stdout, stderr, absRoot)
}
