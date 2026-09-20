package hostservice

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// LinuxAdapter manages systemd --user units for per-root scheduler daemons.
type LinuxAdapter struct{}

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
	_ = execwrap.Command("systemctl", "--user", "daemon-reload").Run()
	if err := execwrap.Command("systemctl", "--user", "enable", "--now", e.UnitLabel+".service").Run(); err != nil {
		return errfmt.Newf("systemctl enable --now").Wrap(err)
	}
	return nil
}

func (a LinuxAdapter) Uninstall(e Entry) error {
	_ = execwrap.Command("systemctl", "--user", "disable", "--now", e.UnitLabel+".service").Run()
	_ = fileutil.Remove(a.unitPath(e))
	_ = execwrap.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

func (a LinuxAdapter) Start(e Entry) error {
	// Match desired_state=enabled: re-enable unit then start.
	if err := execwrap.Command("systemctl", "--user", "enable", "--now", e.UnitLabel+".service").Run(); err != nil {
		if err2 := execwrap.Command("systemctl", "--user", "start", e.UnitLabel+".service").Run(); err2 != nil {
			return errfmt.Newf("systemctl enable --now / start").Wrap(err)
		}
	}
	return nil
}

func (a LinuxAdapter) Stop(e Entry) error {
	// stop + disable so Restart=on-failure cannot race and desired_state=disabled sticks.
	// TRACK: BLI-SCHED-HOST-SERVICE-STOP-STICK-001
	_ = execwrap.Command("systemctl", "--user", "stop", e.UnitLabel+".service").Run()
	if err := execwrap.Command("systemctl", "--user", "disable", e.UnitLabel+".service").Run(); err != nil {
		return errfmt.Newf("systemctl disable").Wrap(err)
	}
	return nil
}

func (a LinuxAdapter) Status(e Entry) (string, error) {
	out, err := execwrap.Command("systemctl", "--user", "is-active", e.UnitLabel+".service").CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			s = "inactive"
		}
		return s, nil
	}
	return s, nil
}
