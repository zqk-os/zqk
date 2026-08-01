package objects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// TraitRegistry defines valid traits and their semantics
type TraitRegistry struct {
	traits         map[string]*TraitDefinition
	conflicts      map[string][]string // trait -> list of conflicting traits
	dependencies   map[string][]string // trait -> list of required traits
	standardTraits []string            // Standard traits that most objects have
	initOnce       sync.Once           // Ensures initialization happens only once
}

// TraitDefinition defines a trait and its properties
type TraitDefinition struct {
	Name        string         // Trait name (e.g., "listable")
	Description string         // What the trait means
	Category    string         // "standard", "domain-specific", "base-group", "system"
	Requires    []string       // Traits that must be present if this trait is used
	Conflicts   []string       // Traits that cannot be used with this trait
	Includes    []string       // Traits included by this trait (for trait groups)
	FieldLevel  bool           // Can this trait be used at field level?
	ObjectLevel bool           // Can this trait be used at object level?
	Config      map[string]any // Trait-specific configuration (e.g., object_config, field_config for snapable)
}

// NewTraitRegistry creates a new trait registry
// Traits are loaded lazily on first use to avoid blocking on filesystem I/O during initialization
func NewTraitRegistry() *TraitRegistry {
	registry := &TraitRegistry{
		traits:       make(map[string]*TraitDefinition),
		conflicts:    make(map[string][]string),
		dependencies: make(map[string][]string),
	}
	// Don't load traits here - load lazily on first use
	// This prevents blocking on filesystem I/O during initialization
	return registry
}

// ensureInitialized ensures traits are loaded (lazy initialization)
// Uses sync.Once to ensure initialization happens only once, even with concurrent access.
// If the registry already has traits (e.g. from LoadTraitsFromDirectory in tests), skip
// loading from the default directory so pre-loaded traits are not overwritten.
func (tr *TraitRegistry) ensureInitialized() {
	tr.initOnce.Do(func() {
		// Already populated (e.g. by test or explicit LoadTraitsFromDirectory)
		if len(tr.traits) > 0 {
			tr.identifyStandardTraits()
			return
		}
		// Load traits from files (if available)
		traitsDir := findTraitsDir()
		if traitsDir != emptyValue {
			// Use a goroutine with timeout to prevent blocking indefinitely
			done := make(chan error, 1)
			goroutinelabels.NewGoroutine("objects", "load traits from directory").StartSimple(func() {
				done <- tr.LoadTraitsFromDirectory(traitsDir)
			})

			// Wait with timeout (5 seconds max)
			select {
			case err := <-done:
				if err != nil || len(tr.traits) == 0 {
					// If loading from files fails or directory is empty, fall back to hardcoded traits
					// This ensures backward compatibility and supports tests with unpopulated layouts
					tr.registerStandardTraits()
				} else {
					// Mark standard traits for GetStandardTraits()
					tr.identifyStandardTraits()
				}
			case <-time.After(5 * time.Second):
				// Timeout - fall back to hardcoded traits
				// This prevents indefinite blocking on filesystem issues
				tr.registerStandardTraits()
			}
		} else {
			// Fall back to hardcoded traits if traits directory not found
			tr.registerStandardTraits()
		}
	})
}

// registerStandardTraits registers all standard traits used in the system
func (tr *TraitRegistry) registerStandardTraits() {
	standardTraits := []*TraitDefinition{
		{
			Name:        "listable",
			Description: "Object can be listed/queried in collections",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
		},
		{
			Name:        "readable",
			Description: "Object/field can be read/retrieved",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
		},
		{
			Name:        "writable",
			Description: "Object/field can be written/created",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
			Requires:    []string{"readable"}, // writable implies readable
		},
		{
			Name:        "modifiable",
			Description: "Object/field can be modified/updated",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
			Requires:    []string{"readable"}, // modifiable implies readable
		},
		{
			Name:        "removable",
			Description: "Object/field can be removed/deleted",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  false,                // Fields are not typically "removable" independently
			Requires:    []string{"readable"}, // removable implies readable
		},
		{
			Name:        "formatable",
			Description: "Object/field can be formatted for display",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
		},
		{
			Name:        "groupable",
			Description: "Object/field can be grouped in collections",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
		},
		{
			Name:        "filterable",
			Description: "Object/field can be filtered in queries",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
		},
		{
			Name:        "sortable",
			Description: "Object/field can be sorted in collections",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
		},
		{
			Name:        "searchable",
			Description: "Object/field can be searched",
			Category:    "standard",
			ObjectLevel: true,
			FieldLevel:  true,
		},
	}

	// Register domain-specific traits
	domainTraits := []*TraitDefinition{
		{
			Name:        "constrainable",
			Description: "Object can have layout/constraint rules applied (domain-specific)",
			Category:    "domain-specific",
			ObjectLevel: true,
			FieldLevel:  false,
		},
	}

	// Register behavior traits required for CLI features to remain functional even
	// if the traits directory fails to load (e.g., load timeout / filesystem issues).
	behaviorTraits := []*TraitDefinition{
		{
			Name:        "auto_status_transitionable",
			Description: "Object can be advanced using --auto-status (lifecycle-driven status advancement).",
			Category:    "behavior",
			Requires:    []string{},
			Conflicts:   []string{},
			Includes:    []string{},
			FieldLevel:  false,
			ObjectLevel: true,
			Config:      map[string]any{},
		},
	}

	// Register all traits
	for _, trait := range standardTraits {
		tr.RegisterTrait(trait)
		tr.standardTraits = append(tr.standardTraits, trait.Name)
	}

	for _, trait := range domainTraits {
		tr.RegisterTrait(trait)
	}

	for _, trait := range behaviorTraits {
		tr.RegisterTrait(trait)
	}
}

// RegisterTrait registers a trait definition
func (tr *TraitRegistry) RegisterTrait(trait *TraitDefinition) {
	tr.traits[trait.Name] = trait

	// Build reverse index for dependencies
	if len(trait.Requires) > 0 {
		tr.dependencies[trait.Name] = trait.Requires
	}

	// Build reverse index for conflicts
	if len(trait.Conflicts) > 0 {
		tr.conflicts[trait.Name] = trait.Conflicts
	}
}

// IsValidTrait checks if a trait name is valid
func (tr *TraitRegistry) IsValidTrait(name string) bool {
	tr.ensureInitialized()
	_, exists := tr.traits[name]
	return exists
}

// GetTrait returns the trait definition for a given name
func (tr *TraitRegistry) GetTrait(name string) (*TraitDefinition, error) {
	tr.ensureInitialized()
	trait, exists := tr.traits[name]
	if !exists {
		return nil, errfmt.Errorf("unknown trait: %s", name)
	}
	return trait, nil
}

// ValidateTraits validates a list of traits and returns validation errors
func (tr *TraitRegistry) ValidateTraits(traits []string, level string) []TraitValidationError {
	return tr.ValidateTraitsWithContext(traits, level, nil)
}

// ValidateTraitsWithContext validates a list of traits and returns validation errors
// contextTraits provides additional traits (e.g., expanded object traits) for dependency checking
func (tr *TraitRegistry) ValidateTraitsWithContext(traits []string, level string, contextTraits []string) []TraitValidationError {
	var errors []TraitValidationError

	// level is "object" or "field"
	// First pass: build trait map and check for duplicates/invalid traits
	traitMap := make(map[string]bool)
	for _, trait := range traits {
		// Check for duplicates
		if traitMap[trait] {
			errors = append(errors, TraitValidationError{
				Trait:    trait,
				Message:  fmt.Sprintf("duplicate trait: %s", trait),
				Category: "duplicate",
			})
			continue
		}
		traitMap[trait] = true

		// Check if trait is valid
		if !tr.IsValidTrait(trait) {
			errors = append(errors, TraitValidationError{
				Trait:    trait,
				Message:  fmt.Sprintf("unknown trait: %s", trait),
				Category: "invalid",
			})
			continue
		}

		// Check if trait is valid for this level
		traitDef, err := tr.GetTrait(trait)
		if err != nil {
			// Already handled by IsValidTrait check above
			continue
		}
		if level == "field" && !traitDef.FieldLevel {
			errors = append(errors, TraitValidationError{
				Trait:    trait,
				Message:  fmt.Sprintf("trait %s cannot be used at field level", trait),
				Category: "level_mismatch",
			})
		}
		if level == "object" && !traitDef.ObjectLevel {
			errors = append(errors, TraitValidationError{
				Trait:    trait,
				Message:  fmt.Sprintf("trait %s cannot be used at object level", trait),
				Category: "level_mismatch",
			})
		}
	}

	// Add context traits to trait map for dependency checking (e.g., object traits for field validation)
	for _, trait := range contextTraits {
		traitMap[trait] = true
	}

	// Second pass: check dependencies and conflicts (after trait map is complete)
	for _, trait := range traits {
		// Skip if trait was invalid (already reported)
		if !tr.IsValidTrait(trait) {
			continue
		}

		// Check dependencies
		if deps, hasDeps := tr.dependencies[trait]; hasDeps {
			for _, dep := range deps {
				if !traitMap[dep] {
					errors = append(errors, TraitValidationError{
						Trait:    trait,
						Message:  fmt.Sprintf("trait %s requires trait %s", trait, dep),
						Category: "missing_dependency",
					})
				}
			}
		}

		// Check conflicts
		if conflicts, hasConflicts := tr.conflicts[trait]; hasConflicts {
			for _, conflict := range conflicts {
				if traitMap[conflict] {
					errors = append(errors, TraitValidationError{
						Trait:    trait,
						Message:  fmt.Sprintf("trait %s conflicts with trait %s", trait, conflict),
						Category: "conflict",
					})
				}
			}
		}
	}

	return errors
}

// TraitValidationError represents a trait validation error
type TraitValidationError struct {
	Trait    string // The trait that caused the error
	Message  string // Human-readable error message
	Category string // Error category: "invalid", "duplicate", "missing_dependency", "conflict", "level_mismatch"
}

// GetStandardTraits returns the list of standard traits
func (tr *TraitRegistry) GetStandardTraits() []string {
	return tr.standardTraits
}

// GetAllTraits returns all registered trait names
func (tr *TraitRegistry) GetAllTraits() []string {
	tr.ensureInitialized()
	var names []string
	for name := range tr.traits {
		names = append(names, name)
	}
	return names
}

// ValidateTraitFieldConsistency validates that field-level traits are subsets of object-level traits
// Expands trait groups in object traits before comparing
func (tr *TraitRegistry) ValidateTraitFieldConsistency(objectTraits, fieldTraits []string, fieldName string) []TraitValidationError {
	tr.ensureInitialized()
	var errors []TraitValidationError

	// Expand object traits (which may include trait groups) to individual traits
	expandedObjectTraits, err := tr.ExpandTraits(objectTraits)
	if err != nil {
		// If expansion fails, fall back to direct comparison
		expandedObjectTraits = objectTraits
	}

	objectTraitMap := make(map[string]bool)
	for _, trait := range expandedObjectTraits {
		objectTraitMap[trait] = true
	}

	for _, fieldTrait := range fieldTraits {
		if !objectTraitMap[fieldTrait] {
			errors = append(errors, TraitValidationError{
				Trait:    fieldTrait,
				Message:  fmt.Sprintf("field %s declares trait %s but object spec does not have this trait", fieldName, fieldTrait),
				Category: "field_object_mismatch",
			})
		}
	}

	return errors
}

// ExtractFieldTraits extracts traits from a field definition
func ExtractFieldTraits(fieldDef map[string]any) []string {
	traits, ok := fieldDef["traits"].([]any)
	if !ok {
		return []string{}
	}

	var traitStrings []string
	for _, trait := range traits {
		if traitStr, ok := trait.(string); ok {
			traitStrings = append(traitStrings, traitStr)
		}
	}

	return traitStrings
}

// LoadTraitsFromDirectory loads trait definitions from YAML files in a directory
func (tr *TraitRegistry) LoadTraitsFromDirectory(traitsDir string) error {
	entries, err := os.ReadDir(traitsDir)
	if err != nil {
		return errfmt.Errorf("failed to read traits directory %s: %w", traitsDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .yaml files
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		traitPath := filepath.Join(traitsDir, entry.Name())
		trait, err := tr.loadTraitFromFile(traitPath)
		if err != nil {
			// Log error but continue loading other traits
			continue
		}

		// Register the trait
		tr.RegisterTrait(trait)
		if trait.Category == "standard" {
			tr.standardTraits = append(tr.standardTraits, trait.Name)
		}
	}

	return nil
}

// loadTraitFromFile loads a trait definition from a YAML file
func (tr *TraitRegistry) loadTraitFromFile(filePath string) (*TraitDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read trait file %s: %w", filePath, err)
	}

	var traitFile struct {
		Name         string         `yaml:"name"`
		Description  string         `yaml:"description"`
		Category     string         `yaml:"category"`
		ObjectLevel  bool           `yaml:"objectlevel"`
		FieldLevel   bool           `yaml:"fieldlevel"`
		Requires     []string       `yaml:"requires"`
		Conflicts    []string       `yaml:"conflicts"`
		Includes     []string       `yaml:"includes"` // For trait groups
		Version      string         `yaml:"version"`
		Status       string         `yaml:"status"`
		ObjectConfig map[string]any `yaml:"object_config"` // Object-level configuration (e.g., object_query for snapable)
		FieldConfig  map[string]any `yaml:"field_config"`  // Field-level configuration (e.g., data_handlers for snapable)
	}

	if err := yaml.Unmarshal(data, &traitFile); err != nil {
		return nil, errfmt.Errorf("failed to parse trait file %s: %w", filePath, err)
	}

	// Validate required fields
	if traitFile.Name == emptyValue {
		return nil, errfmt.Errorf("trait file %s missing required field 'name'", filePath)
	}
	if traitFile.Description == emptyValue {
		return nil, errfmt.Errorf("trait file %s missing required field 'description'", filePath)
	}
	if traitFile.Category == emptyValue {
		traitFile.Category = "standard" // Default category
	}

	// Only load active traits (skip draft, deprecated, etc.)
	if traitFile.Status != emptyValue && traitFile.Status != ObjectStatusActive {
		return nil, errfmt.Errorf("trait %s has status '%s', skipping", traitFile.Name, traitFile.Status)
	}

	// Build config map from object_config and field_config
	config := make(map[string]any)
	if traitFile.ObjectConfig != nil {
		config["object_config"] = traitFile.ObjectConfig
	}
	if traitFile.FieldConfig != nil {
		config["field_config"] = traitFile.FieldConfig
	}

	trait := &TraitDefinition{
		Name:        traitFile.Name,
		Description: traitFile.Description,
		Category:    traitFile.Category,
		ObjectLevel: traitFile.ObjectLevel,
		FieldLevel:  traitFile.FieldLevel,
		Requires:    traitFile.Requires,
		Conflicts:   traitFile.Conflicts,
		Includes:    traitFile.Includes,
		Config:      config,
	}

	return trait, nil
}

// identifyStandardTraits identifies which traits are standard based on category
func (tr *TraitRegistry) identifyStandardTraits() {
	tr.standardTraits = []string{}
	for name, trait := range tr.traits {
		if trait.Category == "standard" {
			tr.standardTraits = append(tr.standardTraits, name)
		}
	}
}

// findTraitsDir finds the traits directory relative to the project root
// Uses the same pattern as findSpecsDir for consistency
func findTraitsDir() string {
	// Try common locations
	possiblePaths := []string{
		paths.ProcessInternalTraitsDir,
		filepath.Join("..", paths.ProcessInternalTraitsDir),
		filepath.Join("..", "..", paths.ProcessInternalTraitsDir),
	}

	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	for _, path := range possiblePaths {
		absPath := filepath.Join(wd, path)
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			return absPath
		}
	}

	// Walk up directory tree
	dir := wd
	for {
		potentialPath := filepath.Join(dir, paths.ProcessInternalTraitsDir)
		if info, err := os.Stat(potentialPath); err == nil && info.IsDir() {
			return potentialPath
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// ExpandTraitGroup expands a trait group into its constituent traits
// Returns the list of individual traits that the group includes
func (tr *TraitRegistry) ExpandTraitGroup(traitName string) ([]string, error) {
	trait, err := tr.GetTrait(traitName)
	if err != nil {
		return nil, err
	}

	// If not a trait group, return just the trait itself
	if (trait.Category != "base-group" && trait.Category != "specialized-group") || len(trait.Includes) == 0 {
		return []string{traitName}, nil
	}

	// Expand the group recursively
	var expanded []string
	seen := make(map[string]bool)

	var expandRecursive func(string) error
	expandRecursive = func(name string) error {
		if seen[name] {
			return nil // Already expanded, avoid cycles
		}
		seen[name] = true

		subTrait, err := tr.GetTrait(name)
		if err != nil {
			return errfmt.Errorf("failed to get trait %s: %w", name, err)
		}

		// If this is a group, expand it
		if (subTrait.Category == "base-group" || subTrait.Category == "specialized-group") && len(subTrait.Includes) > 0 {
			for _, included := range subTrait.Includes {
				if err := expandRecursive(included); err != nil {
					return err
				}
			}
		} else {
			// It's a regular trait, add it
			expanded = append(expanded, name)
		}

		return nil
	}

	// Expand all included traits
	for _, included := range trait.Includes {
		if err := expandRecursive(included); err != nil {
			return nil, err
		}
	}

	return expanded, nil
}

// ExpandTraits expands all trait groups in a list of traits into their constituent traits
// This is useful when resolving traits from object specs that may reference base trait groups
func (tr *TraitRegistry) ExpandTraits(traits []string) ([]string, error) {
	tr.ensureInitialized()
	var expanded []string
	seen := make(map[string]bool)

	for _, trait := range traits {
		// Check if it's a trait group
		expandedGroup, err := tr.ExpandTraitGroup(trait)
		if err != nil {
			// If trait doesn't exist, just add it as-is (might be invalid, will be caught by validation)
			if !seen[trait] {
				expanded = append(expanded, trait)
				seen[trait] = true
			}
			continue
		}

		// Add all expanded traits
		for _, expandedTrait := range expandedGroup {
			if !seen[expandedTrait] {
				expanded = append(expanded, expandedTrait)
				seen[expandedTrait] = true
			}
		}
	}

	return expanded, nil
}
