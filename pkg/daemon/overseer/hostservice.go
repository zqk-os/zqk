package overseer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

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
