package objects

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// highVolumeKindsYAML mirrors .zqk/specs/configs/high_volume_kinds.yaml structure.
type highVolumeKindsYAML struct {
	Kinds []struct {
		Kind    string `yaml:"kind"`
		Storage string `yaml:"storage"`
	} `yaml:"kinds"`
}

// validateHighVolumeKindsYAMLStructure rejects ambiguous or invalid rows before stream-vs-spec checks.
// Deploy-time gate: duplicate kind keys, empty kind, empty/unknown storage values.
func validateHighVolumeKindsYAMLStructure(cfg *highVolumeKindsYAML) error {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(cfg.Kinds))
	for i, e := range cfg.Kinds {
		k := strings.TrimSpace(e.Kind)
		if k == "" {
			return errfmt.Errorf("kinds[%d]: empty kind", i)
		}
		if _, dup := seen[k]; dup {
			return errfmt.Errorf("duplicate kind %q", k)
		}
		seen[k] = struct{}{}
		s := strings.TrimSpace(strings.ToLower(e.Storage))
		if s == "" {
			return errfmt.Errorf("kind %q: empty storage (allowed: stream, cas)", k)
		}
		// stream = stream create path; cas = HV cache/retention only (e.g. scheduler_job).
		if s != "stream" && s != "cas" {
			return errfmt.Errorf("kind %q: unknown storage %q (allowed: stream, cas)", k, strings.TrimSpace(e.Storage))
		}
	}
	return nil
}

// ValidateHighVolumeStreamKindsMatchSpecIndex ensures that for every kind listed as storage: stream
// in high_volume_kinds.yaml, the materialized spec index declares storage_profile stream (data-cell /
// stream stewardship contract). Matches TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds.
// It also rejects duplicate kind rows and invalid storage values in the YAML (deploy-time gate).
//
// If projectRoot does not contain .zqk/specs/configs/high_volume_kinds.yaml, validation
// is skipped (nil error) so minimal temp trees and partial checkouts keep working.
func ValidateHighVolumeStreamKindsMatchSpecIndex(idx *SpecIndex, projectRoot string) error {
	if projectRoot == "" {
		return nil
	}
	cfgPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, paths.HighVolumeKindsConfigFile)
	if st, err := fileutil.Stat(cfgPath); err != nil || st.IsDir() {
		return nil
	}
	if idx == nil || idx.Kinds == nil {
		return errfmt.Errorf("spec index is nil or has no kinds")
	}
	data, err := fileutil.ReadFile(cfgPath)
	if err != nil {
		return errfmt.Newf("read %s", cfgPath).Wrap(err)
	}
	var cfg highVolumeKindsYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return errfmt.Newf("parse high_volume_kinds").Wrap(err)
	}
	if err := validateHighVolumeKindsYAMLStructure(&cfg); err != nil {
		return errfmt.Errorf("%s: %w", cfgPath, err)
	}

	streamFromYAML := make(map[string]bool)
	for _, e := range cfg.Kinds {
		if e.Kind == "" || e.Storage != "stream" {
			continue
		}
		streamFromYAML[e.Kind] = true
	}

	var msgs []string
	for kind := range streamFromYAML {
		ks, ok := idx.GetKindSummary(kind)
		if !ok {
			msgs = append(msgs, "high_volume_kinds lists stream kind "+kind+" but it is missing from spec_index (check object_specs / kind_mappings)")
			continue
		}
		if ks.StorageProfile != string(datacell.ProfileStream) {
			msgs = append(msgs, "kind "+kind+": spec_index storage_profile="+ks.StorageProfile+", want "+string(datacell.ProfileStream)+" (must match high_volume_kinds stream entry and override auditable cas_entity)")
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return errfmt.Errorf("high_volume_kinds vs spec_index: %s", strings.Join(msgs, "; "))
}
