package builders

import (
	"bytes"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// SpecChange represents a detected change in a spec file
type SpecChange struct {
	Ontology       string // Spec ontology
	Filepath       string // Path to the YAML file
	CurrentVersion string // Current schema_version in the YAML file
	LatestBuilder  string // Latest builder version available
	HasChanges     bool   // Whether file differs from generated version
	DiffSummary    string // Summary of differences
}

// SpecChangeDetector detects changes between YAML files and generated builder versions
type SpecChangeDetector struct {
	registry  *VersionedBuilderRegistry
	specsDir  string
	generator *SpecGenerator
}

// NewSpecChangeDetector creates a new spec change detector
func NewSpecChangeDetector(specsDir string) *SpecChangeDetector {
	return &SpecChangeDetector{
		registry:  GetGlobalRegistry(),
		specsDir:  specsDir,
		generator: NewSpecGenerator(""), // Will generate to temp location for comparison
	}
}

// DetectChanges detects changes in spec files by comparing them with generated versions
func (d *SpecChangeDetector) DetectChanges() ([]SpecChange, error) {
	var changes []SpecChange

	// Get all ontologies with builders
	ontologies := d.registry.GetAllOntologies()

	for _, ontology := range ontologies {
		specFile := filepath.Join(d.specsDir, ontology+sbcore.FileExtYAML)

		// Check if file exists
		if _, err := fileutil.Stat(specFile); fileutil.IsNotExist(err) {
			continue // File doesn't exist, skip
		}

		// Load current YAML file
		currentData, err := fileutil.ReadFile(specFile)
		if err != nil {
			continue // Skip files that can't be read
		}

		var currentSpec objects.Spec
		if err := yaml.Unmarshal(currentData, &currentSpec); err != nil {
			continue // Skip files that can't be parsed
		}

		// Get latest builder version
		latestVersion, err := d.registry.GetLatestVersion(ontology)
		if err != nil {
			continue // No builder available
		}

		// Generate spec from latest builder
		generatedData, err := d.generator.GenerateSpecsToYAML(ontology, latestVersion)
		if err != nil {
			continue // Can't generate from builder
		}

		var generatedSpec objects.Spec
		if err := yaml.Unmarshal(generatedData, &generatedSpec); err != nil {
			continue // Can't parse generated spec
		}

		// Compare specs (simple comparison for now - can be enhanced)
		hasChanges := !specsEqual(&currentSpec, &generatedSpec)

		if hasChanges {
			change := SpecChange{
				Ontology:       ontology,
				Filepath:       specFile,
				CurrentVersion: currentSpec.SchemaVersion,
				LatestBuilder:  latestVersion,
				HasChanges:     true,
				DiffSummary:    summarizeDiff(&currentSpec, &generatedSpec),
			}
			changes = append(changes, change)
		}
	}

	return changes, nil
}

// specsEqual compares two specs for equality using YAML marshaling
// This provides a robust comparison that handles nested structures
func specsEqual(s1, s2 *objects.Spec) bool {
	// Normalize by marshaling to YAML and comparing bytes
	// This handles all fields including nested structures
	y1, err1 := yaml.Marshal(s1)
	y2, err2 := yaml.Marshal(s2)

	if err1 != nil || err2 != nil {
		// If marshaling fails, fall back to basic comparison
		return basicSpecsEqual(s1, s2)
	}

	// Compare normalized YAML
	return bytes.Equal(y1, y2)
}

// basicSpecsEqual performs basic field-by-field comparison
func basicSpecsEqual(s1, s2 *objects.Spec) bool {
	if s1.Ontology != s2.Ontology {
		return false
	}
	if s1.Description != s2.Description {
		return false
	}
	if s1.Visibility != s2.Visibility {
		return false
	}
	if s1.Extends != s2.Extends {
		return false
	}

	// Compare traits (order-independent)
	if len(s1.Traits) != len(s2.Traits) {
		return false
	}
	traitMap1 := make(map[string]bool)
	for _, t := range s1.Traits {
		traitMap1[t] = true
	}
	for _, t := range s2.Traits {
		if !traitMap1[t] {
			return false
		}
	}

	// Compare field counts
	if len(s1.Fields) != len(s2.Fields) {
		return false
	}

	// Field comparison would require deep comparison of field definitions
	// For now, just check field names exist
	for fieldName := range s1.Fields {
		if _, exists := s2.Fields[fieldName]; !exists {
			return false
		}
	}

	return true
}

// summarizeDiff creates a summary of differences between specs
func summarizeDiff(current, generated *objects.Spec) string {
	var diffs []string

	if current.SchemaVersion != generated.SchemaVersion {
		diffs = append(diffs, fmt.Sprintf("%s: %s -> %s", objects.FieldKeySchemaVersion, generated.SchemaVersion, current.SchemaVersion))
	}

	if len(current.Fields) != len(generated.Fields) {
		diffs = append(diffs, fmt.Sprintf("field count: %d -> %d", len(generated.Fields), len(current.Fields)))
	}

	// Find added/removed fields
	addedFields := []string{}
	removedFields := []string{}

	for fieldName := range current.Fields {
		if _, exists := generated.Fields[fieldName]; !exists {
			addedFields = append(addedFields, fieldName)
		}
	}
	for fieldName := range generated.Fields {
		if _, exists := current.Fields[fieldName]; !exists {
			removedFields = append(removedFields, fieldName)
		}
	}

	if len(addedFields) > 0 {
		diffs = append(diffs, fmt.Sprintf("added fields: %v", addedFields))
	}
	if len(removedFields) > 0 {
		diffs = append(diffs, fmt.Sprintf("removed fields: %v", removedFields))
	}

	if len(diffs) == 0 {
		return "changes detected (detailed comparison needed)"
	}

	return fmt.Sprintf("%s", diffs)
}
