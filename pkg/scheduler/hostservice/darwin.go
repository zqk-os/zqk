package hostservice

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

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
	_ = execwrap.Command("launchctl", "bootout", "gui/"+uid(), plist).Run()
	if err := execwrap.Command("launchctl", "bootstrap", "gui/"+uid(), plist).Run(); err != nil {
		// Older macOS: load
		if err2 := execwrap.Command("launchctl", "load", "-w", plist).Run(); err2 != nil {
			return errfmt.Newf("launchctl bootstrap/load").Wrap(err)
		}
	}
	return nil
}

func (a DarwinAdapter) Uninstall(e Entry) error {
	plist := a.plistPath(e)
	_ = execwrap.Command("launchctl", "bootout", "gui/"+uid(), plist).Run()
	_ = execwrap.Command("launchctl", "unload", "-w", plist).Run()
	_ = fileutil.Remove(plist)
	return nil
}

func (a DarwinAdapter) Start(e Entry) error {
	if err := refuseIfCallerDescendsFromUnit(e); err != nil {
		return err
	}
	plist := a.plistPath(e)
	domain := "gui/" + uid()
	// Stop boots the job out of the domain; Start must bootstrap again.
	if err := execwrap.Command("launchctl", "bootstrap", domain, plist).Run(); err != nil {
		// Already loaded, or older macOS load path.
		_ = execwrap.Command("launchctl", "load", "-w", plist).Run()
	}
	if err := execwrap.Command("launchctl", "kickstart", "-k", domain+"/"+e.UnitLabel).Run(); err != nil {
		if err2 := execwrap.Command("launchctl", "start", e.UnitLabel).Run(); err2 != nil {
			return errfmt.Newf("launchctl kickstart/start").Wrap(err)
		}
	}
	return nil
}

func (a DarwinAdapter) Stop(e Entry) error {
	if err := refuseIfCallerDescendsFromUnit(e); err != nil {
		return err
	}
	plist := a.plistPath(e)
	domain := "gui/" + uid()
	target := domain + "/" + e.UnitLabel
	// Graceful signal first when the job is still in the domain.
	_ = execwrap.Command("launchctl", "kill", "SIGTERM", target).Run()
	// bootout (not launchctl stop): KeepAlive SuccessfulExit=false respawns after
	// non-zero signal exits, so plain stop looks successful then immediately runs again.
	// intentional disable vs crash-only KeepAlive.
	if err := execwrap.Command("launchctl", "bootout", domain, plist).Run(); err != nil {
		_ = execwrap.Command("launchctl", "bootout", target).Run()
		_ = execwrap.Command("launchctl", "unload", "-w", plist).Run()
	}
	st, _ := a.Status(e)
	if st == "running" {
		return errfmt.Errorf("scheduler host unit still running after stop: %s", e.UnitLabel)
	}
	return nil
}

func (a DarwinAdapter) Status(e Entry) (string, error) {
	out, err := execwrap.Command("launchctl", "print", "gui/"+uid()+"/"+e.UnitLabel).CombinedOutput()
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
