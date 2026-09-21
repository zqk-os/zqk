package quality

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// ErrNoMatricesConfigured indicates matrix_registry.yaml exists but has no entries in matrices map.
	ErrNoMatricesConfigured = errors.New("no matrices configured in registry")
	// ErrNoDefaultMatrix indicates matrix_registry.yaml has no default_name set and none was requested.
	ErrNoDefaultMatrix = errors.New("no default matrix configured in registry")
)

// MatrixRegistry lists named matrices (CSV + profile paths).
// See docs/quality/matrix_registry.yaml and docs/architecture/MATRIX_CLI_STRATEGY.md.
type MatrixRegistry struct {
	SchemaVersion string                         `yaml:"schema_version"`
	DefaultName   string                         `yaml:"default_name"`
	Matrices      map[string]MatrixRegistryEntry `yaml:"matrices"`
}

// LifecycleTrigger defines a rule for automatically updating matrix rows on lifecycle state transitions.
type LifecycleTrigger struct {
	Kind        string   `yaml:"kind"`
	FromState   string   `yaml:"from_state"` // optional, default empty/wildcard
	ToState     string   `yaml:"to_state"`
	MatchColumn string   `yaml:"match_column"`
	SetPairs    []string `yaml:"set_pairs"`
}

// MatrixRegistryEntry resolves one logical matrix.
type MatrixRegistryEntry struct {
	Description       string             `yaml:"description"`
	CSV               string             `yaml:"csv"`
	Profile           string             `yaml:"profile"`
	SessionRefColumn  string             `yaml:"session_ref_column"`
	ValueMap          map[string]string  `yaml:"value_map"` // optional: user token (lower) -> canonical CSV value for --set
	LifecycleTriggers []LifecycleTrigger `yaml:"lifecycle_triggers"`
}

// MatrixResolution is the resolved registry paths plus optional session column and value_map for matrix CLI.
type MatrixResolution struct {
	CSVPath           string
	ProfilePath       string
	Alias             string
	SessionRefColumn  string
	ValueMap          map[string]string // nil if none; keys are lowercase tokens
	LifecycleTriggers []LifecycleTrigger
}

// ApplyValueMapToUpdates replaces each --set value when the trimmed lowercased value matches a key in valueMap.
func ApplyValueMapToUpdates(updates map[string]string, valueMap map[string]string) {
	if len(valueMap) == 0 || len(updates) == 0 {
		return
	}
	for k, v := range updates {
		lk := strings.ToLower(strings.TrimSpace(v))
		if canon, ok := valueMap[lk]; ok {
			updates[k] = canon
		}
	}
}

func normalizeValueMapKeys(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// LoadMatrixRegistry reads docs/quality/matrix_registry.yaml (or path) from project root.
// If the registry file is missing at the default path, it auto-initializes a default template
// artifact so non-vital matrix operations can continue gracefully.
func LoadMatrixRegistry(projectRoot, registryRel string) (*MatrixRegistry, error) {
	defaultRel := filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")
	isDefault := registryRel == "" || registryRel == defaultRel || filepath.Clean(registryRel) == filepath.Clean(defaultRel)
	if registryRel == "" {
		registryRel = defaultRel
	}
	p := registryRel
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectRoot, registryRel)
	}
	data, err := fileutil.ReadFile(p)
	if err != nil {
		if (os.IsNotExist(err) || fileutil.IsNotExist(err)) && isDefault {
			defaultReg := MatrixRegistry{
				SchemaVersion: "2.0.0",
				DefaultName:   "",
				Matrices:      make(map[string]MatrixRegistryEntry),
			}
			raw, _ := yaml.Marshal(defaultReg)
			if dirErr := fileutil.EnsureDir(filepath.Dir(p)); dirErr == nil {
				_ = fileutil.WriteSecureFile(p, raw)
			}
			return &defaultReg, nil
		}
		return nil, errfmt.Newf("matrix registry").Wrap(err)
	}
	var r MatrixRegistry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, errfmt.Newf("matrix registry yaml").Wrap(err)
	}
	if r.Matrices == nil {
		r.Matrices = make(map[string]MatrixRegistryEntry)
	}
	return &r, nil
}

// Resolve returns the named entry with csv/profile paths joined to projectRoot when relative.
func (r *MatrixRegistry) Resolve(projectRoot, name string) (MatrixRegistryEntry, string, string, error) {
	if strings.TrimSpace(name) == "" {
		name = r.DefaultName
	}
	name = strings.TrimSpace(name)
	regPath := filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")
	if r == nil || len(r.Matrices) == 0 {
		return MatrixRegistryEntry{}, "", "", fmt.Errorf("%w (%s). Define an entry under 'matrices' to report or update.", ErrNoMatricesConfigured, regPath)
	}
	if name == "" {
		return MatrixRegistryEntry{}, "", "", fmt.Errorf("%w (%s). Specify --name <matrix> or set default_name.", ErrNoDefaultMatrix, regPath)
	}
	e, ok := r.Matrices[name]
	if !ok {
		return MatrixRegistryEntry{}, "", "", errfmt.Errorf("unknown matrix name %q (see %s)", name, regPath)
	}
	csv := e.CSV
	if csv != "" && !filepath.IsAbs(csv) {
		csv = filepath.Join(projectRoot, csv)
	}
	prof := e.Profile
	if prof != "" && !filepath.IsAbs(prof) {
		prof = filepath.Join(projectRoot, prof)
	}
	return e, csv, prof, nil
}

// ResolveMatrixForCLI resolves CSV and profile paths for matrix report/get/update commands.
// When matrixOverride is non-empty, profileOverride is required (same rules as --matrix + --profile).
// registryRel may be empty to use the default docs/quality/matrix_registry.yaml under projectRoot.
// ValueMap is nil when using --matrix override or when the registry entry has no value_map.
func ResolveMatrixForCLI(projectRoot, name, registryRel, matrixOverride, profileOverride string) (MatrixResolution, error) {
	if strings.TrimSpace(matrixOverride) != "" {
		csvPath := matrixOverride
		if !filepath.IsAbs(csvPath) {
			csvPath = filepath.Join(projectRoot, csvPath)
		}
		profPath := strings.TrimSpace(profileOverride)
		if profPath == "" {
			return MatrixResolution{}, errfmt.Errorf("--profile is required when using --matrix")
		}
		if !filepath.IsAbs(profPath) {
			profPath = filepath.Join(projectRoot, profPath)
		}
		return MatrixResolution{
			CSVPath:     csvPath,
			ProfilePath: profPath,
			Alias:       "(override)",
		}, nil
	}
	if registryRel == "" {
		registryRel = filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")
	}
	reg, err := LoadMatrixRegistry(projectRoot, registryRel)
	if err != nil {
		return MatrixResolution{}, err
	}
	entry, csv, prof, err := reg.Resolve(projectRoot, name)
	if err != nil {
		return MatrixResolution{}, err
	}
	alias := strings.TrimSpace(name)
	if alias == "" {
		alias = reg.DefaultName
	}
	return MatrixResolution{
		CSVPath:           csv,
		ProfilePath:       prof,
		Alias:             alias,
		SessionRefColumn:  strings.TrimSpace(entry.SessionRefColumn),
		ValueMap:          normalizeValueMapKeys(entry.ValueMap),
		LifecycleTriggers: entry.LifecycleTriggers,
	}, nil
}
