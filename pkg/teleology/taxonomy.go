package teleology

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TaxonomyConfig declares the allowed functional domain taxonomy and constraints
// enforced across the intake membrane and promotion done-gates (GH-1071).
type TaxonomyConfig struct {
	Enabled           bool     `yaml:"enabled" json:"enabled"`
	InScope           []string `yaml:"in_scope" json:"in_scope"`
	Prohibited        []string `yaml:"prohibited" json:"prohibited"`
	MissionRef        string   `yaml:"mission_ref" json:"mission_ref"`
	StrictEnforcement bool     `yaml:"strict_enforcement" json:"strict_enforcement"`
}

// TeleologicalReceipt represents the cryptographic/deterministic done-gate receipt
// emitted upon evaluating an object against the root mission taxonomy.
type TeleologicalReceipt struct {
	Status            string   `json:"status"` // "TELEOLOGICAL_ALIGNED" or "TELEOLOGICAL_DRIFT_REFUSED"
	Cause             string   `json:"cause,omitempty"`
	Boundary          string   `json:"boundary"`
	MatchedDomain     string   `json:"matched_domain,omitempty"`
	ViolatingDomains  []string `json:"violating_domains,omitempty"`
	AllowedDomains    []string `json:"allowed_domains,omitempty"`
	ProhibitedDomains []string `json:"prohibited_domains,omitempty"`
}

// LoadTaxonomy loads the effective teleological taxonomy configuration from
// project configurations (config/zqk.yaml, .zqk/config.yaml, or policy).
func LoadTaxonomy(projectRoot string) (*TaxonomyConfig, error) {
	cfg := &TaxonomyConfig{
		MissionRef: "MIS-*",
	}

	// 1. Check primary ZqkConfig
	zqkCfg := config.LoadForRoot(projectRoot)
	if zqkCfg != nil && zqkCfg.Teleology.Enabled != nil {
		cfg.Enabled = *zqkCfg.Teleology.Enabled
		cfg.InScope = zqkCfg.Teleology.InScope
		cfg.Prohibited = zqkCfg.Teleology.Prohibited
		if zqkCfg.Teleology.MissionRef != nil && *zqkCfg.Teleology.MissionRef != "" {
			cfg.MissionRef = *zqkCfg.Teleology.MissionRef
		}
		if zqkCfg.Teleology.StrictEnforcement != nil {
			cfg.StrictEnforcement = *zqkCfg.Teleology.StrictEnforcement
		}
		return cfg, nil
	}

	// 2. Check standalone project data config fallback (.zqk/config.yaml or .zqk/teleology.yaml)
	candidateConfigPaths := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigYAMLFileName),
		filepath.Join(projectRoot, paths.ProjectDataDir, "teleology.yaml"),
	}
	for _, p := range candidateConfigPaths {
		if data, err := fileutil.ReadFile(p); err == nil {
			var raw struct {
				Teleology TaxonomyConfig `yaml:"teleology"`
			}
			if err := yaml.Unmarshal(data, &raw); err == nil && raw.Teleology.Enabled {
				res := raw.Teleology
				if res.MissionRef == "" {
					res.MissionRef = "MIS-*"
				}
				return &res, nil
			}
		}
	}

	// 3. Fallback: check environment override
	if os.Getenv("ZQK_TELEOLOGY_ENABLED") == "1" || os.Getenv("ZQK_TELEOLOGY_ENABLED") == "true" {
		cfg.Enabled = true
		if inScope := os.Getenv("ZQK_TELEOLOGY_IN_SCOPE"); inScope != "" {
			for _, s := range strings.Split(inScope, ",") {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					cfg.InScope = append(cfg.InScope, trimmed)
				}
			}
		}
		if prohibited := os.Getenv("ZQK_TELEOLOGY_PROHIBITED"); prohibited != "" {
			for _, s := range strings.Split(prohibited, ",") {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					cfg.Prohibited = append(cfg.Prohibited, trimmed)
				}
			}
		}
		if mission := os.Getenv("ZQK_TELEOLOGY_MISSION_REF"); mission != "" {
			cfg.MissionRef = mission
		}
	}

	return cfg, nil
}

// EvaluateTeleologicalAlignment inspects an object against the registered taxonomy.
// Returns a deterministic TeleologicalReceipt.
func EvaluateTeleologicalAlignment(obj map[string]any, taxonomy *TaxonomyConfig) *TeleologicalReceipt {
	boundary := "MIS-*"
	if taxonomy != nil && taxonomy.MissionRef != "" {
		boundary = taxonomy.MissionRef
	}

	if taxonomy == nil || !taxonomy.Enabled {
		return &TeleologicalReceipt{
			Status:   "TELEOLOGICAL_ALIGNED",
			Boundary: boundary,
		}
	}

	domains := ExtractCandidateDomains(obj)

	// Phase 1: Prohibited domain boundary check (Zero-Tolerance Contact Inhibition)
	for _, domain := range domains {
		normDomain := normalizeDomain(domain)
		for _, prohibited := range taxonomy.Prohibited {
			normProh := normalizeDomain(prohibited)
			if normDomain == normProh || strings.Contains(normDomain, normProh) || strings.Contains(normProh, normDomain) {
				return &TeleologicalReceipt{
					Status:            "TELEOLOGICAL_DRIFT_REFUSED",
					Cause:             "Domain out of registered taxonomy",
					Boundary:          boundary,
					MatchedDomain:     domain,
					ViolatingDomains:  []string{domain},
					AllowedDomains:    taxonomy.InScope,
					ProhibitedDomains: taxonomy.Prohibited,
				}
			}
		}
	}

	// Phase 2: In-Scope Taxonomic Boundary Check (if in_scope list declared)
	if len(taxonomy.InScope) > 0 {
		matched := false
		var matchedInScope string
		for _, domain := range domains {
			normDomain := normalizeDomain(domain)
			for _, inScope := range taxonomy.InScope {
				normInScope := normalizeDomain(inScope)
				if normDomain == normInScope || strings.Contains(normDomain, normInScope) || strings.Contains(normInScope, normDomain) {
					matched = true
					matchedInScope = inScope
					break
				}
			}
			if matched {
				break
			}
		}

		if !matched && (taxonomy.StrictEnforcement || len(domains) > 0) {
			return &TeleologicalReceipt{
				Status:            "TELEOLOGICAL_DRIFT_REFUSED",
				Cause:             "Domain out of registered taxonomy",
				Boundary:          boundary,
				ViolatingDomains:  domains,
				AllowedDomains:    taxonomy.InScope,
				ProhibitedDomains: taxonomy.Prohibited,
			}
		}

		return &TeleologicalReceipt{
			Status:         "TELEOLOGICAL_ALIGNED",
			Boundary:       boundary,
			MatchedDomain:  matchedInScope,
			AllowedDomains: taxonomy.InScope,
		}
	}

	return &TeleologicalReceipt{
		Status:   "TELEOLOGICAL_ALIGNED",
		Boundary: boundary,
	}
}

// ExtractCandidateDomains discovers domain and functional classification hints
// from object properties (category, components, code_location, tags, title).
func ExtractCandidateDomains(obj map[string]any) []string {
	var candidates []string
	seen := make(map[string]bool)

	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			candidates = append(candidates, v)
		}
	}

	// 1. Explicit Category
	if cat := objects.GetString(obj, objects.FieldKeyCategory); cat != "" {
		add(cat)
	}

	// 2. Components
	if compList, ok := obj[objects.FieldKeyComponents].([]any); ok {
		for _, c := range compList {
			add(fmt.Sprint(c))
		}
	} else if compList, ok := obj[objects.FieldKeyComponents].([]string); ok {
		for _, c := range compList {
			add(c)
		}
	}

	// 3. Code locations (package leaf segments)
	if locList, ok := obj[objects.FieldKeyCodeLocation].([]any); ok {
		for _, l := range locList {
			pkgPath := fmt.Sprint(l)
			add(extractPackageDomain(pkgPath))
		}
	} else if locList, ok := obj[objects.FieldKeyCodeLocation].([]string); ok {
		for _, l := range locList {
			add(extractPackageDomain(l))
		}
	}

	// 4. Tags
	if tags, ok := obj["tags"].([]any); ok {
		for _, t := range tags {
			add(fmt.Sprint(t))
		}
	} else if tags, ok := obj["tags"].([]string); ok {
		for _, t := range tags {
			add(t)
		}
	}

	// 5. Title & Description Keyword Extraction (domain prefixes or explicit brackets e.g. [dsp], [social])
	title, _ := obj[objects.FieldKeyTitle].(string)
	if title != "" {
		if idx := strings.Index(title, ":"); idx > 0 && idx < 30 {
			add(title[:idx])
		}
		if strings.HasPrefix(title, "[") {
			if endIdx := strings.Index(title, "]"); endIdx > 1 {
				add(title[1:endIdx])
			}
		}
	}

	return candidates
}

func extractPackageDomain(pkgPath string) string {
	pkgPath = strings.TrimPrefix(pkgPath, "./")
	pkgPath = strings.TrimPrefix(pkgPath, "pkg/")
	parts := strings.Split(pkgPath, "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return pkgPath
}

func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.ReplaceAll(d, "-", "_")
	d = strings.ReplaceAll(d, " ", "_")
	return d
}
