package quality

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
)

// MatrixListEntry is one row in a matrix list result.
type MatrixListEntry struct {
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	CSV              string `json:"csv"`
	Profile          string `json:"profile"`
	SessionRefColumn string `json:"session_ref_column,omitempty"`
	ValueMapKeys     int    `json:"value_map_key_count,omitempty"`
}

// MatrixListResult is machine output for zqk matrix list.
type MatrixListResult struct {
	RegistryPath string            `json:"registry_path"`
	DefaultName  string            `json:"default_name"`
	Matrices     []MatrixListEntry `json:"matrices"`
}

// ListMatricesFromRegistry loads the registry and returns sorted entries (by name).
func ListMatricesFromRegistry(projectRoot, registryRel string) (*MatrixListResult, error) {
	regRel := registryRel
	if regRel == "" {
		regRel = filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")
	}
	reg, err := LoadMatrixRegistry(projectRoot, regRel)
	if err != nil {
		return nil, err
	}
	absReg := regRel
	if !filepath.IsAbs(absReg) {
		absReg = filepath.Join(projectRoot, regRel)
	}

	names := make([]string, 0, len(reg.Matrices))
	for k := range reg.Matrices {
		names = append(names, k)
	}
	sort.Strings(names)

	out := make([]MatrixListEntry, 0, len(names))
	for _, name := range names {
		e := reg.Matrices[name]
		ent := MatrixListEntry{
			Name:             name,
			Description:      strings.TrimSpace(e.Description),
			CSV:              e.CSV,
			Profile:          e.Profile,
			SessionRefColumn: strings.TrimSpace(e.SessionRefColumn),
		}
		if vm := normalizeValueMapKeys(e.ValueMap); len(vm) > 0 {
			ent.ValueMapKeys = len(vm)
		}
		out = append(out, ent)
	}

	return &MatrixListResult{
		RegistryPath: filepath.Clean(absReg),
		DefaultName:  strings.TrimSpace(reg.DefaultName),
		Matrices:     out,
	}, nil
}
