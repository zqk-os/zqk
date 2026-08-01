// Package tray loads named shortcuts ("Tray") that expand to zqk argv lists.
// Default entries are embedded; merge with .zqk/tray.yaml (see Load).
//
// Shape is constrained by a JSON Schema (pkg/tray/tray_config.schema.json, mirrored under
// .zqk/cli/specs/schemas/tray_config.schema.json). This is project-local config validated at load
// time—not a process object kind under docs/architecture/. Promoting a tray manifest to a durable,
// auditable object (e.g. for org-wide sharing) would be a separate design; until then, use YAML +
// schema + optional doc_entry links from backlog items.
//
// Related: pkg/clihooks — built-in hook profile (JSON under .zqk/config/) with optional tray_entry per hook.
// External automation contract: docs/architecture/CLI_EXTERNAL_HOOK_PROTOCOL.md
//
// Optional manifest path: [github.com/lanceman/zqk/pkg/datacell.TrayYAMLPath]. See docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md.
package tray

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

//go:embed default_tray.yaml
var defaultTrayYAML []byte

// Entry is one named shortcut in the tray manifest.
type Entry struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Argv        []string `yaml:"argv" json:"argv"`
}

// Config is the on-disk / embedded YAML shape.
type Config struct {
	SchemaRef string  `yaml:"$schema,omitempty" json:"$schema,omitempty"`
	Version   int     `yaml:"version" json:"version"`
	Entries   []Entry `yaml:"entries" json:"entries"`
}

// Merge overlays user entries onto defaults by name (replace if same name; append new names).
func Merge(defaults, user []Entry) []Entry {
	byName := make(map[string]Entry)
	order := make([]string, 0, len(defaults)+len(user))
	for _, e := range defaults {
		if e.Name == "" {
			continue
		}
		byName[e.Name] = e
		order = append(order, e.Name)
	}
	for _, e := range user {
		if e.Name == "" {
			continue
		}
		if _, exists := byName[e.Name]; exists {
			byName[e.Name] = e
			continue
		}
		byName[e.Name] = e
		order = append(order, e.Name)
	}
	out := make([]Entry, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out
}

// Load returns merged tray entries for projectRoot (embedded default + optional .zqk/tray.yaml).
func Load(projectRoot string) ([]Entry, error) {
	var def Config
	if err := yaml.Unmarshal(defaultTrayYAML, &def); err != nil {
		return nil, errfmt.Newf("parse embedded default tray").Wrap(err)
	}
	if err := validateTrayConfig(&def); err != nil {
		return nil, err
	}
	if projectRoot == "" {
		return validateEntries(def.Entries)
	}
	userPath := datacell.TrayYAMLPath(projectRoot)
	data, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return validateEntries(def.Entries)
		}
		return nil, errfmt.Errorf("read %s: %w", userPath, err)
	}
	var user Config
	if err := yaml.Unmarshal(data, &user); err != nil {
		return nil, errfmt.Newf("parse %s", userPath).Wrap(err)
	}
	if err := validateTrayConfig(&user); err != nil {
		return nil, errfmt.Errorf("%s: %w", userPath, err)
	}
	merged := Merge(def.Entries, user.Entries)
	mergedCfg := Config{
		SchemaRef: def.SchemaRef,
		Version:   def.Version,
		Entries:   merged,
	}
	if err := validateTrayConfig(&mergedCfg); err != nil {
		return nil, err
	}
	return validateEntries(merged)
}

func validateEntries(entries []Entry) ([]Entry, error) {
	for i := range entries {
		e := &entries[i]
		e.Name = strings.TrimSpace(e.Name)
		if e.Name == "" {
			return nil, errfmt.Errorf("tray entry %d: missing name", i)
		}
		if len(e.Argv) == 0 {
			return nil, errfmt.Errorf("tray entry %q: argv is empty", e.Name)
		}
	}
	return entries, nil
}

// Find returns the entry named name, or nil if not found.
func Find(entries []Entry, name string) *Entry {
	name = strings.TrimSpace(name)
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

// FormatExplainLine returns a single-line shell-style hint for humans and agents.
func FormatExplainLine(binaryName string, argv []string) string {
	if binaryName == "" {
		binaryName = "zqk"
	}
	var b strings.Builder
	b.WriteString(binaryName)
	for _, a := range argv {
		b.WriteByte(' ')
		if strings.ContainsAny(a, " \t\n\"'") {
			b.WriteString(fmt.Sprintf("%q", a))
		} else {
			b.WriteString(a)
		}
	}
	return b.String()
}

// ListNames returns entry names sorted for stable display.
func ListNames(entries []Entry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}
