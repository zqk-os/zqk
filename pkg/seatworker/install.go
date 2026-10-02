// Package seatworker installs OS supervisor units for `agent seat-worker`.
// Production must not exec scripts/mesh/install-seat-workers.sh.
package seatworker

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	defaultPollSeconds = 30
	loadAttempts       = 3
)

// InstallConfig is the native seat-worker supervisor install request.
type InstallConfig struct {
	ProjectRoot     string
	Seats           []string
	PersonaRef      string
	ExecuteNonComms bool
	PollSeconds     int
	Binary          string
	WriteOnly       bool // skip launchctl/systemctl (tests)
}

// UnitLabel returns the OS unit label for a seat.
func UnitLabel(seatID string) string {
	return "com." + brand.ExecutableName() + ".mesh.seat-worker." + sanitizeSeat(seatID)
}

func sanitizeSeat(seat string) string {
	seat = strings.TrimSpace(seat)
	if seat == "" {
		return "seat"
	}
	var b strings.Builder
	for _, r := range seat {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Install writes and loads per-seat supervisor units for agentapi/mcp seats.
func Install(ctx context.Context, cfg InstallConfig) error {
	root := strings.TrimSpace(cfg.ProjectRoot)
	if root == "" {
		return errfmt.Errorf("project root not found")
	}
	if paths.IsAgentWorktreePath(root) {
		return errfmt.Errorf("cannot install seat workers on worktree root %s (must bind to seated kernel)", root)
	}
	poll := cfg.PollSeconds
	if poll <= 0 {
		poll = defaultPollSeconds
	}
	bin, err := resolveWorkerBinary(root, cfg.Binary)
	if err != nil {
		return err
	}
	seats, err := resolveSeats(root, cfg.Seats)
	if err != nil {
		return err
	}
	personaOverride := strings.TrimSpace(cfg.PersonaRef)
	for _, seat := range seats {
		persona := personaOverride
		if persona == "" {
			persona = personaForSeat(root, seat)
		}
		if err := persistSeatPersona(root, seat, persona); err != nil {
			return err
		}
		if err := installOne(ctx, root, bin, seat, persona, poll, cfg.ExecuteNonComms, cfg.WriteOnly); err != nil {
			return err
		}
	}
	return nil
}

func resolveSeats(root string, requested []string) ([]string, error) {
	if len(requested) > 0 {
		out := make([]string, 0, len(requested))
		for _, s := range requested {
			if id := strings.TrimSpace(s); id != "" {
				out = append(out, id)
			}
		}
		if len(out) == 0 {
			return nil, errfmt.Errorf("no seats requested")
		}
		return out, nil
	}
	f, err := agentfeed.LoadPeerSeats(root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for id, rec := range f.Seats {
		wake := agentfeed.NormalizeWakeMembrane(rec.Wake)
		if wake == agentfeed.WakeMembraneAgentAPI || wake == agentfeed.WakeMembraneMCP {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, errfmt.Errorf("no agentapi/mcp seats in peer_seats.json")
	}
	return ids, nil
}

func personaForSeat(root, seat string) string {
	f, err := agentfeed.LoadPeerSeats(root)
	if err == nil {
		if rec, ok := f.Seats[seat]; ok {
			if p := strings.TrimSpace(rec.PersonaRef); p != "" {
				return p
			}
		}
	}
	return objects.ConstPersonaDefaultOperator
}

func persistSeatPersona(root, seat, persona string) error {
	f, err := agentfeed.LoadPeerSeats(root)
	if err != nil {
		return err
	}
	if f.Seats == nil {
		f.Seats = map[string]agentfeed.PeerSeatRecord{}
	}
	rec := f.Seats[seat]
	rec.PersonaRef = persona
	if strings.TrimSpace(rec.Duty) == "" {
		wake := agentfeed.NormalizeWakeMembrane(rec.Wake)
		if wake == agentfeed.WakeMembraneMCP {
			rec.Duty = agentfeed.SeatKindCoordinator
		} else if wake == agentfeed.WakeMembraneAgentAPI || wake == agentfeed.WakeMembraneStamp {
			rec.Duty = agentfeed.SeatKindWorker
		}
	}
	f.Seats[seat] = rec
	return agentfeed.SavePeerSeats(root, f)
}

func resolveWorkerBinary(root, override string) (string, error) {
	bin := strings.TrimSpace(override)
	if bin == "" {
		bin = paths.ResolveProductCLI(root)
	}
	if !filepath.IsAbs(bin) {
		if p, err := exec.LookPath(bin); err == nil {
			bin = p
		}
	}
	st, err := fileutil.Stat(bin)
	if err != nil || st.IsDir() {
		return "", errfmt.Errorf("product CLI missing or not executable: %s", bin)
	}
	return bin, nil
}

func installOne(ctx context.Context, root, bin, seat, persona string, poll int, execNC, writeOnly bool) error {
	label := UnitLabel(seat)
	logDir := filepath.Join(paths.LogsDirPath(root), paths.MeshStateSubdir)
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("mesh log dir").Wrap(err)
	}
	switch runtime.GOOS {
	case "darwin":
		plist := darwinPlist(root, bin, seat, persona, poll, execNC, logDir)
		path, err := darwinPlistPath(label)
		if err != nil {
			return err
		}
		if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
			return err
		}
		if err := fileutil.WriteFile(path, []byte(plist), paths.FilePerm644); err != nil {
			return errfmt.Newf("write LaunchAgent").Wrap(err)
		}
		if writeOnly {
			return nil
		}
		return loadDarwin(ctx, label, path)
	case "linux":
		body := linuxUnit(root, bin, seat, persona, poll, execNC)
		path, err := linuxUnitPath(label)
		if err != nil {
			return err
		}
		if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
			return err
		}
		if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
			return errfmt.Newf("write systemd unit").Wrap(err)
		}
		if writeOnly {
			return nil
		}
		_ = execwrap.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()
		if err := execwrap.CommandContext(ctx, "systemctl", "--user", "enable", "--now", label+".service").Run(); err != nil {
			return errfmt.Newf("systemctl enable --now").Wrap(err)
		}
		return nil
	default:
		return errfmt.Errorf("seat-worker install: unsupported GOOS %s", runtime.GOOS)
	}
}

func darwinPlistPath(label string) (string, error) {
	home, err := fileutil.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func linuxUnitPath(label string) (string, error) {
	home, err := fileutil.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", label+".service"), nil
}

func seatWorkerCommandArgs(bin, seat, persona string, poll int, execNC bool) []string {
	args := []string{
		bin, "agent", "seat-worker",
		"--agent-id", seat,
		"--persona-ref", persona,
		"--poll-seconds", strconv.Itoa(poll),
	}
	if execNC {
		args = append(args, "--execute-non-comms")
	}
	return append(args, "--timeout", "24h")
}

func programArgsXML(bin, seat, persona string, poll int, execNC bool) string {
	args := seatWorkerCommandArgs(bin, seat, persona, poll, execNC)
	var b strings.Builder
	for _, a := range args {
		fmt.Fprintf(&b, "    <string>%s</string>\n", xmlText(a))
	}
	return b.String()
}

func darwinPlist(root, bin, seat, persona string, poll int, execNC bool, logDir string) string {
	label := UnitLabel(seat)
	home, _ := fileutil.UserHomeDir()
	pathEnv := zqkenv.OSPath().Get()
	if pathEnv == "" {
		pathEnv = "/usr/bin:/bin:/usr/sbin:/sbin"
	}
	stdout := filepath.Join(logDir, "seat-worker-"+sanitizeSeat(seat)+".stdout.log")
	stderr := filepath.Join(logDir, "seat-worker-"+sanitizeSeat(seat)+".stderr.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
    <key>PATH</key>
    <string>%s</string>
    <key>%s</key>
    <string>%s</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, xmlText(label), programArgsXML(bin, seat, persona, poll, execNC),
		xmlText(root), xmlText(home), xmlText(pathEnv),
		xmlText(zqkenv.ProjectRoot().Name()), xmlText(root),
		xmlText(stdout), xmlText(stderr))
}

func linuxUnit(root, bin, seat, persona string, poll int, execNC bool) string {
	args := seatWorkerCommandArgs(bin, seat, persona, poll, execNC)
	execStart := strings.Join(args, " ")
	return fmt.Sprintf(`[Unit]
Description=%s seat-worker %s
After=default.target

[Service]
Type=simple
WorkingDirectory=%s
Environment=%s=%s
ExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, brand.ExecutableName(), seat, root, zqkenv.ProjectRoot().Name(), root, execStart)
}

func loadDarwin(ctx context.Context, label, plist string) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	_ = execwrap.CommandContext(ctx, "launchctl", "bootout", domain+"/"+label).Run()
	_ = execwrap.CommandContext(ctx, "launchctl", "unload", plist).Run()
	var last error
	for i := 0; i < loadAttempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
		err := execwrap.CommandContext(ctx, "launchctl", "bootstrap", domain, plist).Run()
		if err != nil {
			err = execwrap.CommandContext(ctx, "launchctl", "load", "-w", plist).Run()
		}
		if err == nil {
			return nil
		}
		last = err
	}
	return errfmt.Newf("launchctl load %s", label).Wrap(last)
}
