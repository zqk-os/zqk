package context

import (
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/cli" // ProfileLoader and Profile types
	"github.com/lanceman/zqk/pkg/errfmt"
)

const profileFileExtYAML = ".yaml"

// ProfileSpec represents a loaded profile specification with resolved inheritance
// This is a CLI-specific adapter around the unified profile system
type ProfileSpec struct {
	SchemaVersion string         `yaml:"schema_version"`
	Ontology      string         `yaml:"ontology"` // For backward compatibility, maps to metadata.name
	Extends       string         `yaml:"extends"`  // For backward compatibility, maps to metadata.extends
	Description   string         `yaml:"description"`
	Format        string         `yaml:"format"`
	Verbose       bool           `yaml:"verbose"`
	Quiet         bool           `yaml:"quiet"`
	Flags         map[string]any `yaml:"flags"`
	Commands      map[string]any `yaml:"commands"`
	Storage       map[string]any `yaml:"storage"`
	// Resolved values after inheritance
	ResolvedFormat   string         `yaml:"-"`
	ResolvedVerbose  bool           `yaml:"-"`
	ResolvedQuiet    bool           `yaml:"-"`
	ResolvedFlags    map[string]any `yaml:"-"`
	ResolvedCommands map[string]any `yaml:"-"`
	ResolvedStorage  map[string]any `yaml:"-"`
}

// ProfileLoader loads and resolves profile specifications with inheritance
// Now uses the CLI namespace loader from pkg/cli
type ProfileLoader struct {
	cliLoader *cli.ProfileLoader
	mu        sync.RWMutex //nolint:unused // Reserved for future thread-safety
}

// NewProfileLoader creates a new profile loader
func NewProfileLoader(profilesDir string) *ProfileLoader {
	return &ProfileLoader{
		cliLoader: cli.NewProfileLoader(profilesDir),
	}
}

// LoadProfileWithInheritance loads a profile spec and resolves its inheritance chain
// Results are cached by profile name to avoid reloading the same profile
func (pl *ProfileLoader) LoadProfileWithInheritance(profileFile string) (*ProfileSpec, error) {
	// Convert file name to profile name (remove .yaml extension, handle hyphens/underscores)
	profileName := normalizeProfileName(profileFile)

	// Load using CLI namespace loader
	cliProfile, err := pl.cliLoader.LoadProfile(profileName)
	if err != nil {
		return nil, errfmt.Errorf("load CLI context profile %q (file key %q): %w", profileName, profileFile, err)
	}

	// Convert CLI profile to ProfileSpec
	return convertCLIProfileToSpec(cliProfile), nil
}

// ApplyProfileSpecToContext copies resolved fields from a loaded ProfileSpec onto ctx.
func ApplyProfileSpecToContext(ctx *Context, spec *ProfileSpec) {
	if ctx == nil || spec == nil {
		return
	}
	if spec.ResolvedFormat != emptyValue {
		ctx.Format = spec.ResolvedFormat
	}
	ctx.Verbose = spec.ResolvedVerbose
	ctx.Quiet = spec.ResolvedQuiet

	if n := getPositiveInt(spec.ResolvedStorage, "max_page_size"); n > 0 {
		ctx.StorageMaxPageSize = n
	}
	if n := getPositiveInt(spec.ResolvedStorage, "default_page_size"); n > 0 {
		ctx.StorageDefaultPageSize = n
	}
	if enableGrouping, ok := spec.ResolvedStorage[contextKeyEnableGrouping].(bool); ok {
		ctx.StorageEnableGrouping = enableGrouping
	}
	if n := getPositiveInt(spec.ResolvedStorage, "max_group_size"); n > 0 {
		ctx.StorageMaxGroupSize = n
	}
}

// ApplyNamedProfile resolves profile by CLI name (e.g. "ai-agent", "human") and applies it to ctx.
// ctx.Profile should already be set to the same name by the caller.
func (pl *ProfileLoader) ApplyNamedProfile(ctx *Context, profile string) error {
	if pl == nil || ctx == nil {
		return errfmt.Errorf("profile loader or context is nil")
	}
	profileFile := strings.ReplaceAll(profile, "-", "_") + profileFileExtYAML
	spec, err := pl.LoadProfileWithInheritance(profileFile)
	if err != nil {
		return err
	}
	if spec == nil {
		return errfmt.Errorf("profile %q resolved to nil spec", profile)
	}
	ApplyProfileSpecToContext(ctx, spec)
	return nil
}

// convertCLIProfileToSpec converts a CLI Profile to a ProfileSpec
func convertCLIProfileToSpec(cp *cli.Profile) *ProfileSpec {
	spec := &ProfileSpec{
		SchemaVersion:    cp.SchemaVersion,
		Ontology:         cp.Name, // For backward compatibility
		Extends:          cp.Extends,
		Description:      cp.Description,
		ResolvedFlags:    make(map[string]any),
		ResolvedCommands: make(map[string]any),
		ResolvedStorage:  make(map[string]any),
	}

	// Extract CLI-specific fields from resolved spec
	if format, ok := cp.ResolvedSpec[contextKeyFormat].(string); ok {
		spec.Format = format
		spec.ResolvedFormat = format
	}
	if verbose, ok := cp.ResolvedSpec[contextKeyVerbose].(bool); ok {
		spec.Verbose = verbose
		spec.ResolvedVerbose = verbose
	}
	if quiet, ok := cp.ResolvedSpec[contextKeyQuiet].(bool); ok {
		spec.Quiet = quiet
		spec.ResolvedQuiet = quiet
	}
	if flags, ok := cp.ResolvedSpec[contextKeyFlags].(map[string]any); ok {
		spec.Flags = flags
		spec.ResolvedFlags = flags
	}
	if commands, ok := cp.ResolvedSpec[contextKeyCommands].(map[string]any); ok {
		spec.Commands = commands
		spec.ResolvedCommands = commands
	}
	if storage, ok := cp.ResolvedSpec[contextKeyStorage].(map[string]any); ok {
		spec.Storage = storage
		spec.ResolvedStorage = storage
	}

	return spec
}

// normalizeProfileName converts a profile file name to a profile name
// Handles .yaml extension and hyphen/underscore variations
func normalizeProfileName(profileFile string) string {
	// Remove .yaml extension if present
	name := profileFile
	name = strings.TrimSuffix(name, profileFileExtYAML)

	// Convert hyphens to underscores (for consistency)
	// The CLI loader will try both formats
	return name
}

// ClearCache clears the profile cache, forcing reload of all profiles on next access
func (pl *ProfileLoader) ClearCache() {
	pl.cliLoader.ClearCache()
}

// findProfilesDir attempts to find the profile specs directory
// Delegates to unified loader
//
//nolint:unused // Reserved for future use or legacy compatibility
func findProfilesDir() string {
	// The unified loader will find it automatically
	return ""
}
