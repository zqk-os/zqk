// Package hostservice registers per-project-root OS supervisor units (launchd/systemd)
// and a host-local registry. See docs/architecture/SCHEDULER_HOST_SERVICE_AND_CLUSTER_STATUS.md.
package hostservice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	DesiredStateEnabled  = "enabled"
	DesiredStateDisabled = "disabled"
	DesiredStateAbsent   = "absent"

	registryFileName = "registry.json"
	registryDirName  = "scheduler-services"
)

// Entry is one registered per-root host unit.
type Entry struct {
	RootID       string    `json:"root_id"`
	AbsRoot      string    `json:"abs_root"`
	UnitLabel    string    `json:"unit_label"`
	InstalledAt  time.Time `json:"installed_at"`
	DesiredState string    `json:"desired_state"`
	BinaryRef    string    `json:"binary_ref"`
	GOOS         string    `json:"goos"`
}

// Registry is the host-local service inventory.
type Registry struct {
	Entries []Entry `json:"entries"`
}

// RootID returns a stable id for an absolute project root + brand.
func RootID(absRoot string) string {
	canon := filepath.Clean(absRoot)
	sum := sha256.Sum256([]byte(brand.ExecutableName() + "\x00" + canon))
	return hex.EncodeToString(sum[:8])
}

// UnitLabel returns the OS unit label for a root id.
func UnitLabel(rootID string) string {
	return "com." + brand.ExecutableName() + ".scheduler." + rootID
}

// RegistryPath is ~/.zqk/scheduler-services/registry.json (brand data home).
func RegistryPath() (string, error) {
	home, err := fileutil.UserHomeDir()
	if err != nil {
		return "", errfmt.Newf("host service registry home").Wrap(err)
	}
	dir := filepath.Join(home, "."+brand.ExecutableName(), registryDirName)
	return filepath.Join(dir, registryFileName), nil
}

// LoadRegistry reads the host registry (empty if missing).
func LoadRegistry() (*Registry, error) {
	path, err := RegistryPath()
	if err != nil {
		return nil, err
	}
	b, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &Registry{Entries: nil}, nil
		}
		return nil, errfmt.Newf("read host service registry").Wrap(err)
	}
	var reg Registry
	if err := json.Unmarshal(b, &reg); err != nil {
		return nil, errfmt.Newf("parse host service registry").Wrap(err)
	}
	return &reg, nil
}

// SaveRegistry writes the host registry transactionally.
func SaveRegistry(reg *Registry) error {
	path, err := RegistryPath()
	if err != nil {
		return err
	}
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return errfmt.Newf("mkdir host service registry").Wrap(err)
	}
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal host service registry").Wrap(err)
	}
	tmp := path + ".tmp"
	if err := fileutil.WriteFile(tmp, append(b, '\n'), paths.FilePerm644); err != nil {
		return errfmt.Newf("write host service registry tmp").Wrap(err)
	}
	return fileutil.Rename(tmp, path)
}

// ResolveServiceDaemonBinary prefers workshop stable for LaunchAgent/systemd units so
// host supervision matches MCP/scheduler daemon resolution (no tip rebuild split-brain).
// CE may still fall back to tip bin/<exe> when stable is absent.
// TRACK: TDE-1785808957221945000-fcd15e47 — one operational inode for long-lived procs.
func ResolveServiceDaemonBinary(projectRoot string) string {
	if projectRoot == "" {
		return brand.ExecutableName()
	}
	candidates := append(
		paths.StableBinaryCandidates(projectRoot),
		filepath.Join(projectRoot, "bin", brand.ExecutableName()),
		filepath.Join(projectRoot, brand.ExecutableName()),
	)
	for _, c := range candidates {
		if info, err := fileutil.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	if exe, err := fileutil.Executable(); err == nil && exe != "" {
		return exe
	}
	return brand.ExecutableName()
}

// UpsertEntry inserts or replaces an entry by RootID.
func (r *Registry) UpsertEntry(e Entry) {
	for i := range r.Entries {
		if r.Entries[i].RootID == e.RootID {
			r.Entries[i] = e
			return
		}
	}
	r.Entries = append(r.Entries, e)
}

// RemoveEntry drops an entry by RootID.
func (r *Registry) RemoveEntry(rootID string) {
	out := r.Entries[:0]
	for _, e := range r.Entries {
		if e.RootID != rootID {
			out = append(out, e)
		}
	}
	r.Entries = out
}

// FindByRootID returns an entry or false.
func (r *Registry) FindByRootID(rootID string) (Entry, bool) {
	for _, e := range r.Entries {
		if e.RootID == rootID {
			return e, true
		}
	}
	return Entry{}, false
}

// FindByAbsRoot returns an entry matching cleaned abs path.
func (r *Registry) FindByAbsRoot(abs string) (Entry, bool) {
	want := filepath.Clean(abs)
	for _, e := range r.Entries {
		if filepath.Clean(e.AbsRoot) == want {
			return e, true
		}
	}
	return Entry{}, false
}

// PlatformAdapter installs/controls OS units.
type PlatformAdapter interface {
	Install(e Entry) error
	Uninstall(e Entry) error
	Start(e Entry) error
	Stop(e Entry) error
	Status(e Entry) (string, error)
}

// NewAdapter returns the OS adapter for the current GOOS.
func NewAdapter() PlatformAdapter {
	switch runtime.GOOS {
	case "darwin":
		return DarwinAdapter{}
	case "linux":
		return LinuxAdapter{}
	case "windows":
		return WindowsStubAdapter{}
	default:
		return UnsupportedAdapter{GOOS: runtime.GOOS}
	}
}

// UnsupportedAdapter rejects install on unknown OS.
type UnsupportedAdapter struct{ GOOS string }

func (a UnsupportedAdapter) Install(Entry) error {
	return errfmt.Errorf("scheduler host service not supported on GOOS=%s", a.GOOS)
}
func (a UnsupportedAdapter) Uninstall(Entry) error { return a.Install(Entry{}) }
func (a UnsupportedAdapter) Start(Entry) error     { return a.Install(Entry{}) }
func (a UnsupportedAdapter) Stop(Entry) error      { return a.Install(Entry{}) }
func (a UnsupportedAdapter) Status(Entry) (string, error) {
	return "", a.Install(Entry{})
}

// WindowsStubAdapter keeps CLI verbs alive but refuses install until SCM lands.
type WindowsStubAdapter struct{}

func (WindowsStubAdapter) Install(Entry) error {
	return errfmt.Errorf("Windows host service backend not implemented yet (TRACK [REDACTED-ID]); registry/CLI verbs are ready")
}
func (a WindowsStubAdapter) Uninstall(Entry) error { return a.Install(Entry{}) }
func (a WindowsStubAdapter) Start(Entry) error     { return a.Install(Entry{}) }
func (a WindowsStubAdapter) Stop(Entry) error      { return a.Install(Entry{}) }
func (a WindowsStubAdapter) Status(Entry) (string, error) {
	return "unsupported", a.Install(Entry{})
}

// InstallRoot registers and installs a unit for absRoot.
func InstallRoot(absRoot string) (Entry, error) {
	abs, err := filepath.Abs(absRoot)
	if err != nil {
		return Entry{}, errfmt.Newf("abs project root").Wrap(err)
	}
	abs = filepath.Clean(abs)
	if st, err := fileutil.Stat(abs); err != nil || !st.IsDir() {
		return Entry{}, errfmt.Errorf("project root must be an existing directory: %s", abs)
	}
	id := RootID(abs)
	bin := ResolveServiceDaemonBinary(abs)
	e := Entry{
		RootID:       id,
		AbsRoot:      abs,
		UnitLabel:    UnitLabel(id),
		InstalledAt:  time.Now().UTC(),
		DesiredState: DesiredStateEnabled,
		BinaryRef:    bin,
		GOOS:         runtime.GOOS,
	}
	if err := NewAdapter().Install(e); err != nil {
		return Entry{}, err
	}
	reg, err := LoadRegistry()
	if err != nil {
		return Entry{}, err
	}
	reg.UpsertEntry(e)
	if err := SaveRegistry(reg); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Rebind updates abs_root for continuity of root_id.
func Rebind(rootID, newAbs string) (Entry, error) {
	abs, err := filepath.Abs(newAbs)
	if err != nil {
		return Entry{}, errfmt.Newf("abs new path").Wrap(err)
	}
	abs = filepath.Clean(abs)
	reg, err := LoadRegistry()
	if err != nil {
		return Entry{}, err
	}
	e, ok := reg.FindByRootID(rootID)
	if !ok {
		return Entry{}, errfmt.Errorf("root_id %s not in host service registry", rootID)
	}
	adapter := NewAdapter()
	_ = adapter.Stop(e)
	_ = adapter.Uninstall(e)
	e.AbsRoot = abs
	e.BinaryRef = ResolveServiceDaemonBinary(abs)
	if err := adapter.Install(e); err != nil {
		return Entry{}, err
	}
	if e.DesiredState == DesiredStateEnabled {
		_ = adapter.Start(e)
	}
	reg.UpsertEntry(e)
	if err := SaveRegistry(reg); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// GC removes units whose abs_root is missing or desired_state=absent.
func GC() (removed []Entry, err error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	adapter := NewAdapter()
	var keep []Entry
	for _, e := range reg.Entries {
		missing := false
		if st, serr := fileutil.Stat(e.AbsRoot); serr != nil || !st.IsDir() {
			missing = true
		}
		if e.DesiredState == DesiredStateAbsent || missing {
			_ = adapter.Stop(e)
			_ = adapter.Uninstall(e)
			removed = append(removed, e)
			continue
		}
		keep = append(keep, e)
	}
	reg.Entries = keep
	if err := SaveRegistry(reg); err != nil {
		return removed, err
	}
	return removed, nil
}

// UninstallRoot disables and removes a unit.
func UninstallRoot(rootIDOrPath string) error {
	reg, err := LoadRegistry()
	if err != nil {
		return err
	}
	e, ok := reg.FindByRootID(rootIDOrPath)
	if !ok {
		e, ok = reg.FindByAbsRoot(rootIDOrPath)
	}
	if !ok {
		// Try as path → id
		if abs, aerr := filepath.Abs(rootIDOrPath); aerr == nil {
			e, ok = reg.FindByRootID(RootID(abs))
		}
	}
	if !ok {
		return errfmt.Errorf("no host service registry entry for %q", rootIDOrPath)
	}
	adapter := NewAdapter()
	_ = adapter.Stop(e)
	if err := adapter.Uninstall(e); err != nil {
		return err
	}
	reg.RemoveEntry(e.RootID)
	return SaveRegistry(reg)
}

// ResolveEntry finds by root id or path.
func ResolveEntry(rootIDOrPath string) (Entry, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return Entry{}, err
	}
	if e, ok := reg.FindByRootID(rootIDOrPath); ok {
		return e, nil
	}
	if e, ok := reg.FindByAbsRoot(rootIDOrPath); ok {
		return e, nil
	}
	if abs, aerr := filepath.Abs(rootIDOrPath); aerr == nil {
		if e, ok := reg.FindByRootID(RootID(abs)); ok {
			return e, nil
		}
		if e, ok := reg.FindByAbsRoot(abs); ok {
			return e, nil
		}
	}
	return Entry{}, errfmt.Errorf("no host service registry entry for %q", rootIDOrPath)
}

// Orphans lists registry entries whose abs_root no longer exists.
func Orphans() ([]Entry, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range reg.Entries {
		if st, serr := fileutil.Stat(e.AbsRoot); serr != nil || !st.IsDir() {
			out = append(out, e)
		}
	}
	return out, nil
}

// NormalizeDesiredState validates desired_state literals.
func NormalizeDesiredState(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case DesiredStateEnabled, DesiredStateDisabled, DesiredStateAbsent:
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return ""
	}
}

// SetEntryDesiredState persists desired_state for an existing registry entry.
// Used by scheduler service start/stop so status desired= matches OS supervision intent.
func SetEntryDesiredState(rootID, state string) (Entry, error) {
	state = NormalizeDesiredState(state)
	if state == "" {
		return Entry{}, errfmt.Errorf("invalid desired_state %q", state)
	}
	reg, err := LoadRegistry()
	if err != nil {
		return Entry{}, err
	}
	e, ok := reg.FindByRootID(rootID)
	if !ok {
		return Entry{}, errfmt.Errorf("root_id %s not in host service registry", rootID)
	}
	e.DesiredState = state
	reg.UpsertEntry(e)
	if err := SaveRegistry(reg); err != nil {
		return Entry{}, err
	}
	return e, nil
}
