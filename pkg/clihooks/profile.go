// Package clihooks persists a built-in hook profile (like feature flags): enable/disable and optional tray entry.
// Storage: .zqk/config/cli_hook_profile.json (see [github.com/zqk-os/zqk/pkg/datacell.CLIHookProfilePath]).
//
// External automation must use the zqk CLI only; see docs/architecture/CLI_EXTERNAL_HOOK_PROTOCOL.md — do not
// treat this package as a stable import target for out-of-repo Go code; the contract is CLI + JSON schema + ProtocolVersion.
//
// Broader data-cell layout: docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md.
package clihooks

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Hook is one built-in automation hook the CLI and shell can agree on.
type Hook struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	// TrayEntry names a tray entry (zqk tray list) to run; empty means use the hook’s default script/behavior.
	TrayEntry string `json:"tray_entry,omitempty"`
}

// hookProfileFile is the on-disk JSON document.
type hookProfileFile struct {
	Version int     `json:"version"`
	Hooks   []*Hook `json:"hooks"`
}

// Profile manages CLI hook bindings for a project.
type Profile struct {
	mu       sync.RWMutex
	hooks    map[string]*Hook
	filePath string
}

// NewProfile creates a profile bound to projectRoot/.zqk/config/cli_hook_profile.json.
func NewProfile(projectRoot string) *Profile {
	filePath := datacell.CLIHookProfilePath(projectRoot)
	return &Profile{
		hooks:    make(map[string]*Hook),
		filePath: filePath,
	}
}

// Load reads the profile from disk; missing file uses defaults only.
func (p *Profile) Load() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.applyDefaultsUnlocked()

	if _, err := fileutil.Stat(p.filePath); fileutil.IsNotExist(err) {
		return nil
	}
	data, err := fileutil.ReadFile(p.filePath)
	if err != nil {
		return errfmt.Errorf("read cli hook profile: %w", err)
	}
	var doc hookProfileFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return errfmt.Newf("parse cli hook profile").Wrap(err)
	}
	for _, h := range doc.Hooks {
		if h == nil || h.ID == "" {
			continue
		}
		if _, ok := p.hooks[h.ID]; ok {
			p.hooks[h.ID] = mergeHook(p.hooks[h.ID], h)
		}
	}
	return nil
}

func mergeHook(base, overlay *Hook) *Hook {
	out := *base
	if overlay.Description != "" {
		out.Description = overlay.Description
	}
	out.Enabled = overlay.Enabled
	out.TrayEntry = overlay.TrayEntry
	return &out
}

func (p *Profile) applyDefaultsUnlocked() {
	for _, h := range builtinHooks() {
		if _, exists := p.hooks[h.ID]; !exists {
			c := *h
			p.hooks[h.ID] = &c
		}
	}
}

// Save writes the profile to disk.
func (p *Profile) Save() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.saveUnlocked()
}

// IsEnabled reports whether hook id is enabled (default false if unknown).
func (p *Profile) IsEnabled(id string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	h, ok := nildecode.DecodeNonNilPayload[*Hook](p.hooks[id])
	if !ok {
		return false
	}
	return h.Enabled
}

// Has reports whether id is a known built-in hook.
func (p *Profile) Has(id string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.hooks[id]
	return ok
}

// Get returns a copy of the hook or nil.
func (p *Profile) Get(id string) *Hook {
	p.mu.RLock()
	defer p.mu.RUnlock()
	h, ok := nildecode.DecodeNonNilPayload[*Hook](p.hooks[id])
	if !ok {
		return nil
	}
	c := *h
	return &c
}

// List returns all hooks (sorted by id for stability).
func (p *Profile) List() []*Hook {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Hook, 0, len(p.hooks))
	for _, h := range p.hooks {
		if h == nil {
			continue
		}
		c := *h
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SetEnabled sets enabled for a known hook id.
func (p *Profile) SetEnabled(id string, enabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	h, ok := nildecode.DecodeNonNilPayload[*Hook](p.hooks[id])
	if !ok {
		return errfmt.Errorf("unknown cli hook %q", id)
	}
	h.Enabled = enabled
	return p.saveUnlocked()
}

// SetTrayEntry sets optional tray entry name (empty clears).
func (p *Profile) SetTrayEntry(id string, trayEntry string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	h, ok := nildecode.DecodeNonNilPayload[*Hook](p.hooks[id])
	if !ok {
		return errfmt.Errorf("unknown cli hook %q", id)
	}
	h.TrayEntry = trayEntry
	return p.saveUnlocked()
}

func (p *Profile) saveUnlocked() error {
	if err := fileutil.MkdirAll(filepath.Dir(p.filePath), paths.DirPerm755); err != nil {
		return errfmt.Newf("mkdir for cli hook profile").Wrap(err)
	}
	list := make([]*Hook, 0, len(p.hooks))
	for _, h := range p.hooks {
		list = append(list, h)
	}
	doc := hookProfileFile{Version: 1, Hooks: list}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal cli hook profile").Wrap(err)
	}
	if err := fileutil.WriteFile(p.filePath, data, paths.FilePerm644); err != nil {
		return errfmt.Errorf("write cli hook profile: %w", err)
	}
	return nil
}
