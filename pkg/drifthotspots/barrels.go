package drifthotspots

import (
	"path/filepath"
	"strings"
)

// IsConstantBarrelPath reports paths where string literals are intentionally centralizing
// ontology/lock/trait names (not ad hoc business-logic magic). Skipping these reduces false
// positives when hunting drift-prone call-site literals.
func IsConstantBarrelPath(path string) bool {
	base := filepath.Base(path)
	if base == "lock_op_names.go" {
		return true
	}
	slash := filepath.ToSlash(path)
	if strings.Contains(slash, "pkg/specbuilder/bldr_v2/") && strings.HasSuffix(base, "_constants.go") {
		return true
	}
	return false
}

// IsDataCatalogPath reports files whose string literals are primarily declarative catalogs
// (fixtures/config mappings/readme generation tables) rather than business-logic call sites.
// We still analyze compare/switch usage in these files; only assignment-style kind literals are skipped.
func IsDataCatalogPath(path string) bool {
	slash := filepath.ToSlash(path)
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test_helper.go") {
		return true
	}
	for _, rel := range []string{
		"pkg/objects/kind_mappings.go",
		"pkg/validation/id_prefixes_config.go",
		"pkg/validation/namespaces_config.go",
		"pkg/specbuilder/bldr_config_v1/kind_mappings_config_builder.go",
		"pkg/specbuilder/bldr_config_v1/id_prefixes_config_builder.go",
		"pkg/specbuilder/bldr_config_v1/namespaces_config_builder.go",
		"scripts/generate-readme-index.go",
		"scripts/generate-process-readme-index.go",
	} {
		if strings.HasSuffix(slash, rel) {
			return true
		}
	}
	return false
}
