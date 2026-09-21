package objects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var traitDirs stampmemo.Table[[]*TraitDefinition] // keyed by traits directory (closed per project)

// TraitStatusReactive is the object-level admission flag for status-event listeners.
// TRACK: BLI-1786411312347141000-5f3d9063
const TraitStatusReactive = "status_reactive"

// TraitOpenCountable is the object-level remaining-open capability. Fields live on
// the remaining_open mixin (remaining_open_count). Parallel: occupiable vs occupancy.
// TRACK: POL-ARCH-20260901 / BLI-CEF-CONTAINER-REMAINING-OPEN-001
const TraitOpenCountable = "open_countable"

// TraitOccupiable indicates an object carries an occupancy slot (claimed_by, claimed_at).
// TRACK: TDE-CEF-IN-PROGRESS-REQUIRES-CLAIM-001
const TraitOccupiable = "occupiable"

// TraitFieldReferenceGroup is the field-level group for kernel object pointers
// (*_ref / *_refs). Specs declare this group instead of listing query traits.
const TraitFieldReferenceGroup = "field_reference_group"

// TraitFieldMutableGroup is the field-level group for user-editable fields.
const TraitFieldMutableGroup = "field_mutable_group"

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
	Requires    []string       // Constraint: these traits must be present after expansion
	Conflicts   []string       // Traits that cannot be used with this trait
	Includes    []string       // Composition: conferred by this trait (groups expand away; other traits keep themselves)
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
		if traitsDir == emptyValue {
			tr.registerStandardTraits()
			return
		}
		if err := tr.LoadTraitsFromDirectory(traitsDir); err != nil || len(tr.traits) == 0 {
			tr.registerStandardTraits()
			return
		}
		tr.identifyStandardTraits()
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
		{
			Name:        "completable",
			Description: "Object participates in a work interval (started_at, completed_at).",
			Category:    "behavior",
			Requires:    []string{},
			Conflicts:   []string{},
			Includes:    []string{},
			FieldLevel:  false,
			ObjectLevel: true,
			Config:      map[string]any{},
		},
		{
			Name:        "effort_aware",
			Description: "Object carries planned vs realized effort; requires completable.",
			Category:    "behavior",
			Requires:    []string{"completable"},
			Conflicts:   []string{},
			Includes:    []string{"completable"},
			FieldLevel:  false,
			ObjectLevel: true,
			Config:      map[string]any{},
		},
		{
			Name:        "satisfiable",
			Description: "Object is a predicate whose done-state is that the proposition holds (not a work clock).",
			Category:    "behavior",
			Requires:    []string{},
			Conflicts:   []string{},
			Includes:    []string{},
			FieldLevel:  false,
			ObjectLevel: true,
			Config:      map[string]any{},
		},
		{
			Name:        TraitOccupiable,
			Description: "Object carries an occupancy slot (claimed_by, claimed_at); empty claimed_by is unoccupied.",
			Category:    "behavior",
			Requires:    []string{},
			Conflicts:   []string{},
			Includes:    []string{},
			FieldLevel:  false,
			ObjectLevel: true,
			Config:      map[string]any{},
		},
		{
			Name:        TraitStatusReactive,
			Description: "Object may interpret a catalyst status event (outbound listener stubs); kinds without it no-op.",
			Category:    "behavior",
			Requires:    []string{},
			Conflicts:   []string{},
			Includes:    []string{},
			FieldLevel:  false,
			ObjectLevel: true,
			Config:      map[string]any{},
		},
		{
			Name:        TraitOpenCountable,
			Description: "Object's behavior changes when remaining_open_count hits zero; fields live on remaining_open mixin.",
			Category:    "behavior",
			Requires:    []string{},
			Conflicts:   []string{},
			Includes:    []string{},
			FieldLevel:  false,
			ObjectLevel: true,
			Config:      map[string]any{"count_field": FieldKeyRemainingOpenCount},
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

	// Presence after composition: Includes (including group Includes) satisfy
	// Requires. effort_aware includes completable; base_object_traits includes
	// base_auditable_traits. writable Requires readable but does not Include it
	// — listing only writable still fails.
	presence := tr.traitPresence(traits, contextTraits)

	// Second pass: check dependencies and conflicts against composed presence
	for _, trait := range traits {
		// Skip if trait was invalid (already reported)
		if !tr.IsValidTrait(trait) {
			continue
		}

		// Check dependencies
		if deps, hasDeps := tr.dependencies[trait]; hasDeps {
			for _, dep := range deps {
				if !presence[dep] {
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
				if presence[conflict] {
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

// TraitValidationCategoryRedundantInclude is emitted when an authored trait list
// restates a name already conferred by another listed trait's Includes
// (effort_aware/completable, base_object_traits/base_auditable_traits).
// TRACK: TDE-CEF-TRAIT-INCLUDE-REDUNDANT-001
const TraitValidationCategoryRedundantInclude = "redundant_include"

// TraitValidationError represents a trait validation error
type TraitValidationError struct {
	Trait    string // The trait that caused the error
	Message  string // Human-readable error message
	Category string // Error category: "invalid", "duplicate", "missing_dependency", "conflict", "level_mismatch", "redundant_include"
}

// includeClosure returns trait names conferred by name via Includes (not name itself).
func (tr *TraitRegistry) includeClosure(name string) map[string]bool {
	tr.ensureInitialized()
	out := make(map[string]bool)
	var walk func(string)
	walk = func(n string) {
		n = strings.TrimSpace(n)
		if n == emptyValue {
			return
		}
		def, err := tr.GetTrait(n)
		if err != nil || def == nil {
			return
		}
		for _, inc := range def.Includes {
			inc = strings.TrimSpace(inc)
			if inc == emptyValue || out[inc] {
				continue
			}
			out[inc] = true
			walk(inc)
		}
	}
	walk(name)
	return out
}

// ValidateRedundantIncludes reports authored traits that are already conferred
// by another trait in the same list via Includes. Does not inspect inheritance
// merges (ResolvedTraits); those copy parent authored groups that are nested
// by includes. Call this on spec.Traits.
func (tr *TraitRegistry) ValidateRedundantIncludes(traits []string) []TraitValidationError {
	tr.ensureInitialized()
	var errs []TraitValidationError
	seenRedundant := make(map[string]bool)
	for i, t := range traits {
		t = strings.TrimSpace(t)
		if t == emptyValue || seenRedundant[t] {
			continue
		}
		for j, u := range traits {
			if i == j {
				continue
			}
			u = strings.TrimSpace(u)
			if u == emptyValue {
				continue
			}
			if tr.includeClosure(u)[t] {
				errs = append(errs, TraitValidationError{
					Trait:    t,
					Message:  fmt.Sprintf("trait %s is already included by %s; do not also list it", t, u),
					Category: TraitValidationCategoryRedundantInclude,
				})
				seenRedundant[t] = true
				break
			}
		}
	}
	return errs
}

// StripRedundantIncludedTraits returns traits with Includes-redundant names removed,
// preserving first-seen order of keepers. Used by zqk new object-spec drafts so
// copied ResolvedTraits do not restate nested groups.
func (tr *TraitRegistry) StripRedundantIncludedTraits(traits []string) []string {
	redundant := make(map[string]bool)
	for _, e := range tr.ValidateRedundantIncludes(traits) {
		redundant[e.Trait] = true
	}
	out := make([]string, 0, len(traits))
	seen := make(map[string]bool)
	for _, t := range traits {
		t = strings.TrimSpace(t)
		if t == emptyValue || redundant[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
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

// LoadTraitsFromDirectory loads trait definitions from YAML files in a directory and its subdirectories
func (tr *TraitRegistry) LoadTraitsFromDirectory(traitsDir string) error {
	defs, err := traitDirs.Load(traitsDir, yamlTreeStamp(traitsDir), func() ([]*TraitDefinition, error) {
		return readTraitDir(traitsDir)
	})
	if err != nil {
		return err
	}
	tr.traits = make(map[string]*TraitDefinition, len(defs))
	tr.conflicts = make(map[string][]string)
	tr.dependencies = make(map[string][]string)
	tr.standardTraits = nil
	for _, def := range defs {
		cloned := cloneTraitDefinition(def)
		tr.RegisterTrait(cloned)
		if cloned.Category == "standard" {
			tr.standardTraits = append(tr.standardTraits, cloned.Name)
		}
	}
	return nil
}

func yamlTreeStamp(dir string) stampmemo.Stamp {
	cands := []string{dir}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".yaml") {
			cands = append(cands, path)
		}
		return nil
	})
	return stampmemo.OfAll(cands...)
}

func readTraitDir(traitsDir string) ([]*TraitDefinition, error) {
	var defs []*TraitDefinition
	walkErr := filepath.WalkDir(traitsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		trait, parseErr := parseTraitFile(path)
		if parseErr != nil {
			return nil
		}
		defs = append(defs, trait)
		return nil
	})
	if walkErr != nil {
		return nil, errfmt.Errorf("failed to read traits directory %s: %w", traitsDir, walkErr)
	}
	return defs, nil
}

func cloneTraitDefinition(t *TraitDefinition) *TraitDefinition {
	if t == nil {
		return nil
	}
	out := *t
	out.Requires = append([]string(nil), t.Requires...)
	out.Conflicts = append([]string(nil), t.Conflicts...)
	out.Includes = append([]string(nil), t.Includes...)
	out.Config = cloneAnyMap(t.Config)
	return &out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out[k] = cloneAnyMap(x)
		case []any:
			out[k] = cloneAnySlice(x)
		default:
			out[k] = v
		}
	}
	return out
}

func cloneAnySlice(in []any) []any {
	if in == nil {
		return nil
	}
	out := make([]any, len(in))
	for i, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out[i] = cloneAnyMap(x)
		case []any:
			out[i] = cloneAnySlice(x)
		default:
			out[i] = v
		}
	}
	return out
}

// parseTraitFile loads a trait definition from a YAML file
func parseTraitFile(filePath string) (*TraitDefinition, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read trait file %s: %w", filePath, err)
	}

	var traitFile struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Category    string `yaml:"category"`
		ObjectLevel bool   `yaml:"object_level"`
		FieldLevel  bool   `yaml:"field_level"`
		// TRACK: BLI-KERNEL-WORK-ENVELOPE-001 — drop concatenated aliases after traits/*.yaml are snake_case only.
		ObjectLevelLegacy bool           `yaml:"objectlevel"`
		FieldLevelLegacy  bool           `yaml:"fieldlevel"`
		Requires          []string       `yaml:"requires"`
		Conflicts         []string       `yaml:"conflicts"`
		Includes          []string       `yaml:"includes"` // Composition (groups and behavior facets)
		Version           string         `yaml:"version"`
		Status            string         `yaml:"status"`
		ObjectConfig      map[string]any `yaml:"object_config"` // Object-level configuration (e.g., object_query for snapable)
		FieldConfig       map[string]any `yaml:"field_config"`  // Field-level configuration (e.g., data_handlers for snapable)
		Config            map[string]any `yaml:"config"`        // Top-level trait config (e.g. open_countable count_field)
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

	// Build config map from object_config, field_config, and top-level config.
	config := make(map[string]any)
	if traitFile.ObjectConfig != nil {
		config["object_config"] = traitFile.ObjectConfig
	}
	if traitFile.FieldConfig != nil {
		config["field_config"] = traitFile.FieldConfig
	}
	for k, v := range traitFile.Config {
		if k == emptyValue {
			continue
		}
		config[k] = v
	}

	objectLevel := traitFile.ObjectLevel
	if !objectLevel {
		objectLevel = traitFile.ObjectLevelLegacy
	}
	fieldLevel := traitFile.FieldLevel
	if !fieldLevel {
		fieldLevel = traitFile.FieldLevelLegacy
	}

	trait := &TraitDefinition{
		Name:        traitFile.Name,
		Description: traitFile.Description,
		Category:    traitFile.Category,
		ObjectLevel: objectLevel,
		FieldLevel:  fieldLevel,
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
	return paths.FirstExistingFromCwd(paths.ProcessInternalTraitsDir)
}

func traitExpandsAway(t *TraitDefinition) bool {
	return t != nil && (t.Category == "base-group" || t.Category == "specialized-group") && len(t.Includes) > 0
}

// ExpandTraitGroup expands a trait into the traits that should be considered present.
// Base/specialized groups expand away (the group name is not retained). Other traits
// keep themselves and compose Includes (effort_aware → effort_aware + completable).
func (tr *TraitRegistry) ExpandTraitGroup(traitName string) ([]string, error) {
	if _, err := tr.GetTrait(traitName); err != nil {
		return nil, err
	}

	var expanded []string
	seen := make(map[string]bool)

	var expandRecursive func(string) error
	expandRecursive = func(name string) error {
		if seen[name] {
			return nil
		}
		subTrait, err := tr.GetTrait(name)
		if err != nil {
			return errfmt.Errorf("failed to get trait %s: %w", name, err)
		}
		seen[name] = true
		if !traitExpandsAway(subTrait) {
			expanded = append(expanded, name)
		}
		for _, included := range subTrait.Includes {
			if err := expandRecursive(included); err != nil {
				return err
			}
		}
		return nil
	}

	if err := expandRecursive(traitName); err != nil {
		return nil, err
	}
	return expanded, nil
}

func (tr *TraitRegistry) traitPresence(listed, contextTraits []string) map[string]bool {
	tr.ensureInitialized()
	presence := make(map[string]bool)
	var markIncludes func(name string)
	markIncludes = func(name string) {
		name = strings.TrimSpace(name)
		if name == emptyValue || presence[name] {
			return
		}
		presence[name] = true
		def, err := tr.GetTrait(name)
		if err != nil || def == nil {
			return
		}
		for _, inc := range def.Includes {
			markIncludes(inc)
		}
	}
	add := func(traits []string) {
		if len(traits) == 0 {
			return
		}
		for _, name := range traits {
			markIncludes(name)
		}
		expanded, err := tr.ExpandTraits(traits)
		if err != nil {
			expanded = traits
		}
		for _, trait := range expanded {
			presence[trait] = true
		}
	}
	add(listed)
	add(contextTraits)
	return presence
}

// ExpandTraits expands trait groups and composes Includes on non-group traits.
// This is useful when resolving traits from object specs that may reference
// base trait groups or facets such as effort_aware (which confers completable).
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
