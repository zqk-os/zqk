package hostservice

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Mesh seat-worker LaunchAgent / systemd labels are root-scoped so two ZQK
// checkouts can both supervise peer-agent-1 without last-install-wins.
// Shape: com.<brand>.mesh.seat-worker.<root_id>.<seat_id>
// Legacy (unscoped): com.<brand>.mesh.seat-worker.<seat_id>
// MCP daemon + PrivilegedWriter use the same root_id pattern (see mcp_pw_unit_labels.go).

var (
	rootIDHexRe          = regexp.MustCompile(`^[0-9a-f]{16}$`)
	goTestTempSegmentRe  = regexp.MustCompile(`^Test[A-Z][A-Za-z0-9_]*`)
	meshSeatWorkerPrefix = func() string {
		return "com." + brand.ExecutableName() + ".mesh.seat-worker."
	}
)

// MeshSeatWorkerUnitLabel returns the root-scoped OS unit label for a seat.
func MeshSeatWorkerUnitLabel(rootID, seatID string) string {
	return meshSeatWorkerPrefix() + strings.TrimSpace(rootID) + "." + sanitizeMeshSeatID(seatID)
}

// LegacyMeshSeatWorkerUnitLabel is the pre-sovereignty seat-only label.
func LegacyMeshSeatWorkerUnitLabel(seatID string) string {
	return meshSeatWorkerPrefix() + sanitizeMeshSeatID(seatID)
}

func sanitizeMeshSeatID(seat string) string {
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

// MeshSeatWorkerLabelParts is the parse result for a mesh seat-worker unit label.
type MeshSeatWorkerLabelParts struct {
	RootID string // empty when Legacy
	SeatID string
	Legacy bool
	Valid  bool
	Label  string
}

// ParseMeshSeatWorkerLabel classifies root-scoped vs legacy mesh seat-worker labels.
func ParseMeshSeatWorkerLabel(label string) MeshSeatWorkerLabelParts {
	label = strings.TrimSpace(label)
	out := MeshSeatWorkerLabelParts{Label: label}
	prefix := meshSeatWorkerPrefix()
	if !strings.HasPrefix(label, prefix) {
		return out
	}
	rest := strings.TrimPrefix(label, prefix)
	if rest == "" {
		return out
	}
	parts := strings.Split(rest, ".")
	if len(parts) >= 2 && rootIDHexRe.MatchString(parts[0]) {
		out.Valid = true
		out.Legacy = false
		out.RootID = parts[0]
		out.SeatID = strings.Join(parts[1:], ".")
		return out
	}
	out.Valid = true
	out.Legacy = true
	out.SeatID = rest
	return out
}

// IsLegacyMeshSeatWorkerLabel reports seat-only (no root_id) mesh seat-worker labels.
func IsLegacyMeshSeatWorkerLabel(label string) bool {
	p := ParseMeshSeatWorkerLabel(label)
	return p.Valid && p.Legacy
}

// LaunchAgentsDir resolves ~/Library/LaunchAgents or ZQK_LAUNCH_AGENTS_DIR override.
func LaunchAgentsDir() (string, error) {
	if v := strings.TrimSpace(zqkenv.LaunchAgentsDir().Get()); v != "" {
		return filepath.Clean(v), nil
	}
	home, err := fileutil.UserHomeDir()
	if err != nil {
		return "", errfmt.Newf("hostservice LaunchAgents home").Wrap(err)
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

// RefuseMeshSeatWorkerRoot fails closed for worktrees, ATK temp checkouts, and Go test TempDirs.
func RefuseMeshSeatWorkerRoot(absRoot string) error {
	abs := filepath.Clean(strings.TrimSpace(absRoot))
	if abs == "" || abs == "." {
		return errfmt.Errorf("mesh seat-worker: empty project root")
	}
	if paths.IsAgentWorktreePath(abs) {
		return errfmt.Errorf("cannot install seat workers on worktree root %s (must bind to seated kernel)", abs)
	}
	slash := filepath.ToSlash(abs)
	if strings.Contains(slash, "/tmp/ATK-") || strings.Contains(slash, "/private/tmp/ATK-") ||
		strings.HasPrefix(slash, "/tmp/ATK-") || strings.HasPrefix(slash, "/private/tmp/ATK-") {
		return errfmt.Errorf("cannot install seat workers on ATK temp root %s", abs)
	}
	for _, seg := range strings.Split(slash, "/") {
		if goTestTempSegmentRe.MatchString(seg) {
			return errfmt.Errorf("cannot install seat workers on Go test TempDir root %s", abs)
		}
	}
	if st, err := fileutil.Stat(abs); err != nil || !st.IsDir() {
		return errfmt.Errorf("mesh seat-worker project root must be an existing directory: %s", abs)
	}
	return nil
}

// RefuseMeshSeatWorkerBinary requires the worker binary to live under the seated root.
func RefuseMeshSeatWorkerBinary(absRoot, binary string) error {
	root := filepath.Clean(absRoot)
	bin := filepath.Clean(binary)
	if !filepath.IsAbs(bin) {
		return errfmt.Errorf("mesh seat-worker binary must be absolute under root: %s", binary)
	}
	rel, err := filepath.Rel(root, bin)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errfmt.Errorf("mesh seat-worker binary %s is not under seated root %s", bin, absRoot)
	}
	return nil
}

// MeshSeatWorkerGCResult is one unit removed by zombie GC.
type MeshSeatWorkerGCResult struct {
	Label   string `json:"label"`
	AbsRoot string `json:"abs_root,omitempty"`
	Reason  string `json:"reason"`
}

// GCMeshSeatWorkerZombies removes legacy unscoped seat-worker units and root-scoped
// units whose WorkingDirectory is missing, under /var/folders|/tmp, or a Go Test path.
// It never deletes a root-scoped unit that still points at a live non-ephemeral root
// (protects foreign checkouts during cleanup-legacy / overseer migration).
func GCMeshSeatWorkerZombies() ([]MeshSeatWorkerGCResult, error) {
	// Darwin LaunchAgents by default; ZQK_LAUNCH_AGENTS_DIR enables the same scan in tests on any GOOS.
	if runtime.GOOS != "darwin" && strings.TrimSpace(zqkenv.LaunchAgentsDir().Get()) == "" {
		return nil, nil
	}
	dir, err := LaunchAgentsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errfmt.Newf("read LaunchAgents").Wrap(err)
	}
	var removed []MeshSeatWorkerGCResult
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".plist") {
			continue
		}
		label := strings.TrimSuffix(ent.Name(), ".plist")
		parsed := ParseMeshSeatWorkerLabel(label)
		if !parsed.Valid {
			continue
		}
		plistPath := filepath.Join(dir, ent.Name())
		absRoot := plistWorkingDirectory(plistPath)
		reason := ""
		if parsed.Legacy {
			reason = "legacy_unscoped_seat_worker"
		} else if absRoot == "" {
			reason = "missing_working_directory"
		} else if err := RefuseMeshSeatWorkerRoot(absRoot); err != nil {
			reason = "ephemeral_or_invalid_root"
		} else if st, serr := fileutil.Stat(absRoot); serr != nil || !st.IsDir() {
			reason = "abs_root_missing"
		}
		if reason == "" {
			continue
		}
		_ = launchctlCmd("bootout", "gui/"+uid(), plistPath)
		_ = launchctlCmd("unload", "-w", plistPath)
		_ = fileutil.Remove(plistPath)
		removed = append(removed, MeshSeatWorkerGCResult{Label: label, AbsRoot: absRoot, Reason: reason})
	}
	return removed, nil
}

func plistWorkingDirectory(plistPath string) string {
	b, err := fileutil.ReadFile(plistPath)
	if err != nil {
		return ""
	}
	s := string(b)
	const key = "<key>WorkingDirectory</key>"
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	const open = "<string>"
	j := strings.Index(rest, open)
	if j < 0 {
		return ""
	}
	rest = rest[j+len(open):]
	k := strings.Index(rest, "</string>")
	if k < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:k])
}

// ShouldCleanupLegacyMeshLabel decides whether cleanup-legacy may delete a com.*.mesh.* unit.
// Root-scoped seat-workers for live foreign roots are preserved.
func ShouldCleanupLegacyMeshLabel(label, absRootHint string) bool {
	parsed := ParseMeshSeatWorkerLabel(label)
	if parsed.Valid {
		if parsed.Legacy {
			return true
		}
		root := absRootHint
		if root == "" {
			return false // unknown WD — leave for GCMeshSeatWorkerZombies after parse
		}
		if err := RefuseMeshSeatWorkerRoot(root); err != nil {
			return true
		}
		if st, err := fileutil.Stat(root); err != nil || !st.IsDir() {
			return true
		}
		return false // live foreign (or local) root-scoped unit — keep
	}
	// Non seat-worker mesh labels (night-duty, watchdog, …) stay legacy-cleanable.
	prefix := "com." + brand.ExecutableName() + ".mesh."
	return strings.HasPrefix(label, prefix)
}
