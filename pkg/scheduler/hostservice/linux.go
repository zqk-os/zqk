package hostservice

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// LinuxAdapter manages systemd --user units for per-root scheduler daemons.
type LinuxAdapter struct{}

var systemctlRunner func(args ...string) ([]byte, error)

var systemctlCmd = func(args ...string) error {
	if systemctlRunner != nil {
		_, err := systemctlRunner(args...)
		return err
	}
	if isHostServiceLiveDisabled() || runtime.GOOS != "linux" {
		return nil
	}
	return execwrap.Command("systemctl", args...).Run()
}

var systemctlOutput = func(args ...string) ([]byte, error) {
	if systemctlRunner != nil {
		return systemctlRunner(args...)
	}
	if isHostServiceLiveDisabled() || runtime.GOOS != "linux" {
		return []byte("inactive"), nil
	}
	return execwrap.Command("systemctl", args...).CombinedOutput()
}

func (LinuxAdapter) unitPath(e Entry) string {
	home, _ := fileutil.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", e.UnitLabel+".service")
}

func (a LinuxAdapter) Install(e Entry) error {
	path := a.unitPath(e)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return errfmt.Newf("mkdir systemd user units").Wrap(err)
	}
	body := fmt.Sprintf(`[Unit]
Description=ZQK scheduler for %s
After=default.target

[Service]
Type=simple
WorkingDirectory=%s
Environment=%s=%s
ExecStart=%s scheduler start --foreground
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, e.AbsRoot, e.AbsRoot, zqkenv.ProjectRoot().Name(), e.AbsRoot, e.BinaryRef)
	if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
		return errfmt.Newf("write systemd unit").Wrap(err)
	}
	if (isHostServiceLiveDisabled() || runtime.GOOS != "linux") && systemctlRunner == nil {
		return nil
	}
	_ = systemctlCmd("--user", "daemon-reload")
	if err := systemctlCmd("--user", "enable", "--now", e.UnitLabel+".service"); err != nil {
		return errfmt.Newf("systemctl enable --now").Wrap(err)
	}
	return nil
}

func (a LinuxAdapter) Uninstall(e Entry) error {
	if (isHostServiceLiveDisabled() || runtime.GOOS != "linux") && systemctlRunner == nil {
		_ = fileutil.Remove(a.unitPath(e))
		return nil
	}
	_ = systemctlCmd("--user", "disable", "--now", e.UnitLabel+".service")
	_ = fileutil.Remove(a.unitPath(e))
	_ = systemctlCmd("--user", "daemon-reload")
	return nil
}

func (a LinuxAdapter) Start(e Entry) error {
	if (isHostServiceLiveDisabled() || runtime.GOOS != "linux") && systemctlRunner == nil {
		return nil
	}
	// Match desired_state=enabled: re-enable unit then start.
	if err := systemctlCmd("--user", "enable", "--now", e.UnitLabel+".service"); err != nil {
		if err2 := systemctlCmd("--user", "start", e.UnitLabel+".service"); err2 != nil {
			return errfmt.Newf("systemctl enable --now / start").Wrap(err)
		}
	}
	return nil
}

func (a LinuxAdapter) Stop(e Entry) error {
	if (isHostServiceLiveDisabled() || runtime.GOOS != "linux") && systemctlRunner == nil {
		return nil
	}
	// stop + disable so Restart=on-failure cannot race and desired_state=disabled sticks.
	_ = systemctlCmd("--user", "stop", e.UnitLabel+".service")
	if err := systemctlCmd("--user", "disable", e.UnitLabel+".service"); err != nil {
		return errfmt.Newf("systemctl disable").Wrap(err)
	}
	return nil
}

func (a LinuxAdapter) Status(e Entry) (string, error) {
	if (isHostServiceLiveDisabled() || runtime.GOOS != "linux") && systemctlRunner == nil {
		return "inactive", nil
	}
	out, err := systemctlOutput("--user", "is-active", e.UnitLabel+".service")
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			s = "inactive"
		}
		return s, nil
	}
	return s, nil
}
