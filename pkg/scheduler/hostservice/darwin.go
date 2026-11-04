package hostservice

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// isHostServiceLiveDisabled returns true if live host launchctl operations
// should be suppressed (e.g. during automated test execution or explicit env disable)
// to prevent tearing down the operator's active desktop/GUI session.
func isHostServiceLiveDisabled() bool {
	if testing.Testing() {
		return true
	}
	if os.Getenv("ZQK_DISABLE_HOSTSERVICE_MUTATION") != "" {
		return true
	}
	return false
}

var launchctlRunner func(args ...string) ([]byte, error)

var launchctlCmd = func(args ...string) error {
	if launchctlRunner != nil {
		_, err := launchctlRunner(args...)
		return err
	}
	if isHostServiceLiveDisabled() {
		return nil
	}
	return execwrap.Command("launchctl", args...).Run()
}

var launchctlOutput = func(args ...string) ([]byte, error) {
	if launchctlRunner != nil {
		return launchctlRunner(args...)
	}
	if isHostServiceLiveDisabled() {
		return []byte{}, nil
	}
	return execwrap.Command("launchctl", args...).CombinedOutput()
}

// DarwinAdapter manages LaunchAgents for per-root scheduler daemons.
type DarwinAdapter struct{}

// keepAlivePlistXML is crash-only KeepAlive (SuccessfulExit=false).
// Unconditional <true/> overrides intentional `scheduler stop` via launchd.
func keepAlivePlistXML() string {
	return `  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key>
    <false/>
  </dict>`
}

func schedulerEnvironmentPlistXML(e Entry) string {
	home, _ := fileutil.UserHomeDir()
	path := zqkenv.OSPath().Get()
	return fmt.Sprintf(`  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
    <key>PATH</key>
    <string>%s</string>
    <key>%s</key>
    <string>%s</string>
    <key>%s</key>
    <string>1</string>
  </dict>`, home, path, zqkenv.ProjectRoot().Name(), e.AbsRoot, zqkenv.SchedulerDaemonMode().Name())
}

func (DarwinAdapter) plistPath(e Entry) string {
	home, _ := fileutil.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", e.UnitLabel+".plist")
}

func (a DarwinAdapter) Install(e Entry) error {
	plist := a.plistPath(e)
	if err := fileutil.MkdirAll(filepath.Dir(plist), paths.DirPerm755); err != nil {
		return errfmt.Newf("mkdir LaunchAgents").Wrap(err)
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>scheduler</string>
    <string>start</string>
    <string>--foreground</string>
  </array>
  <key>WorkingDirectory</key>
  <string>%s</string>
%s
  <key>RunAtLoad</key>
  <true/>
%s
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, e.UnitLabel, e.BinaryRef, e.AbsRoot, schedulerEnvironmentPlistXML(e),
		keepAlivePlistXML(),
		filepath.Join(e.AbsRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler-service.stdout.log"),
		filepath.Join(e.AbsRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler-service.stderr.log"),
	)
	if err := fileutil.MkdirAll(filepath.Join(e.AbsRoot, paths.ProjectDataDir, paths.LogsDir), paths.DirPerm755); err != nil {
		return errfmt.Newf("mkdir scheduler service logs").Wrap(err)
	}
	if err := fileutil.WriteFile(plist, []byte(body), paths.FilePerm644); err != nil {
		return errfmt.Newf("write LaunchAgent plist").Wrap(err)
	}
	if isHostServiceLiveDisabled() && launchctlRunner == nil {
		return nil
	}
	_ = launchctlCmd("bootout", "gui/"+uid(), plist)
	if err := launchctlCmd("bootstrap", "gui/"+uid(), plist); err != nil {
		// Older macOS: load
		if err2 := launchctlCmd("load", "-w", plist); err2 != nil {
			return errfmt.Newf("launchctl bootstrap/load").Wrap(err)
		}
	}
	return nil
}

func (a DarwinAdapter) Uninstall(e Entry) error {
	if strings.TrimSpace(e.UnitLabel) == "" {
		return nil
	}
	plist := a.plistPath(e)
	if isHostServiceLiveDisabled() && launchctlRunner == nil {
		_ = fileutil.Remove(plist)
		return nil
	}
	if fi, err := os.Stat(plist); err == nil && !fi.IsDir() {
		_ = launchctlCmd("bootout", "gui/"+uid(), plist)
		_ = launchctlCmd("unload", "-w", plist)
	}
	_ = fileutil.Remove(plist)
	return nil
}

func (a DarwinAdapter) prepareUnitControl(e Entry) (plist, domain string, err error) {
	if strings.TrimSpace(e.UnitLabel) == "" {
		return "", "", errfmt.Errorf("empty unit label")
	}
	if err := refuseIfCallerDescendsFromUnit(e); err != nil {
		return "", "", err
	}
	return a.plistPath(e), "gui/" + uid(), nil
}

func (a DarwinAdapter) Start(e Entry) error {
	plist, domain, err := a.prepareUnitControl(e)
	if err != nil {
		return err
	}
	if isHostServiceLiveDisabled() && launchctlRunner == nil {
		return nil
	}
	// Stop boots the job out of the domain; Start must bootstrap again.
	if err := launchctlCmd("bootstrap", domain, plist); err != nil {
		// Already loaded, or older macOS load path.
		_ = launchctlCmd("load", "-w", plist)
	}
	if err := launchctlCmd("kickstart", "-k", domain+"/"+e.UnitLabel); err != nil {
		if err2 := launchctlCmd("start", e.UnitLabel); err2 != nil {
			return errfmt.Newf("launchctl kickstart/start").Wrap(err)
		}
	}
	return nil
}

func (a DarwinAdapter) Stop(e Entry) error {
	plist, domain, err := a.prepareUnitControl(e)
	if err != nil {
		return err
	}
	if isHostServiceLiveDisabled() && launchctlRunner == nil {
		return nil
	}
	target := domain + "/" + e.UnitLabel
	// Graceful signal first when the job is still in the domain.
	_ = launchctlCmd("kill", "SIGTERM", target)
	// bootout (not launchctl stop): KeepAlive SuccessfulExit=false respawns after
	// non-zero signal exits, so plain stop looks successful then immediately runs again.
	// intentional disable vs crash-only KeepAlive.
	if fi, err := os.Stat(plist); err == nil && !fi.IsDir() {
		if err := launchctlCmd("bootout", domain, plist); err != nil {
			_ = launchctlCmd("bootout", target)
			_ = launchctlCmd("unload", "-w", plist)
		}
	} else {
		_ = launchctlCmd("bootout", target)
	}
	st, _ := a.Status(e)
	if st == "running" {
		return errfmt.Errorf("scheduler host unit still running after stop: %s", e.UnitLabel)
	}
	return nil
}

func (a DarwinAdapter) Status(e Entry) (string, error) {
	if strings.TrimSpace(e.UnitLabel) == "" || (isHostServiceLiveDisabled() && launchctlRunner == nil) {
		return "stopped", nil
	}
	out, err := launchctlOutput("print", "gui/"+uid()+"/"+e.UnitLabel)
	if err != nil {
		return "stopped", nil
	}
	s := string(out)
	if strings.Contains(s, "state = running") || strings.Contains(s, "pid =") {
		return "running", nil
	}
	return "installed", nil
}

func uid() string {
	return fmt.Sprintf("%d", os.Getuid())
}
