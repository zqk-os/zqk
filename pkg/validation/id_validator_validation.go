package validation

import (
	"regexp"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ValidateID validates an ID against the pattern for the given kind
// ValidateID validates an object ID against configured patterns
// Now supports namespaced IDs (e.g., "zqk:kernel:goal:GOAL-123")
//
//nolint:gocyclo // Function orchestrates multiple validation phases; complexity reduced via helper methods
func (v *IDValidator) ValidateID(id, kind string) (bool, error) {
	if len(id) > MaxObjectIDLength {
		return false, errfmt.Errorf("object id length %d exceeds maximum %d", len(id), MaxObjectIDLength)
	}
	// Ensure patterns are loaded (non-blocking - if already loading, will use what's available)
	if err := v.LoadPatterns(); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Error loading ID patterns in ValidateID: %v\n", err).Log()
	}

	var config *IDPatternConfig
	var exists bool
	if err := concurrency.RunInRLockWithLogger(
		&v.mu,
		LockNameIdValidatorValidateId,
		lockLoggerSystem(),
		func() error {
			var ok bool
			config, ok = v.patterns[kind]
			exists = ok
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Error acquiring read lock for ID pattern validation: %v\n", err).Log()
	}

	if !exists {
		return true, nil // Unknown kind - don't validate (permissive)
	}

	// Special handling for account IDs
	if kind == objects.KindAccount {
		return v.validateAccountID(id, config)
	}

	return v.validateNamespacedActualID(id, config)
}

// validateAccountID validates account IDs with special handling
func (v *IDValidator) validateAccountID(id string, config *IDPatternConfig) (bool, error) {
	// Legacy "account:" prefix is retired (POL-AGENT-ACCOUNT-LOGIN-001).
	// Only valid ACC- prefixes from config are allowed.
	if len(config.Prefixes) > 0 {
		for _, prefix := range config.Prefixes {
			if strings.HasPrefix(id, prefix) {
				return true, nil
			}
		}
	}

	// Also check pattern if available
	if config.Pattern != emptyValue {
		re, err := GetCachedRegexp(config.Pattern)
		if err != nil {
			return false, errfmt.Newf("invalid pattern").Wrap(err)
		}
		if re.MatchString(id) {
			return true, nil
		}
	}

	// If neither prefix nor pattern matches, continue with namespace parsing for other formats
	// (This allows account IDs to be validated as namespaced IDs if they don't match account format)
	return v.validateNamespacedActualID(id, config)
}

func (v *IDValidator) validateNamespacedActualID(id string, config *IDPatternConfig) (bool, error) {
	actualID, err := v.parseAndValidateNamespace(id)
	if err != nil {
		return false, err
	}
	return v.validateActualID(actualID, config)
}

// parseAndValidateNamespace parses namespace and validates format, returns actual ID for validation
func (v *IDValidator) parseAndValidateNamespace(id string) (string, error) {
	parsed := ParseNamespace(id)
	if parsed != nil {
		// Validate namespace format if present
		if err := v.validateNamespaceFormat(parsed); err != nil {
			return "", err
		}

		// Use object ID without namespace for validation
		if parsed.ObjectID != emptyValue {
			return parsed.ObjectID, nil
		}
		// If we have object type but no object ID, the ID might be malformed
		// But we'll still try to validate the rest
		return id, nil
	}

	// ID contains colons but didn't parse as a namespace - might be invalid format
	if strings.Contains(id, ":") {
		if err := v.validateColonFormat(id); err != nil {
			return "", err
		}
	}

	return id, nil
}

// validateNamespaceFormat validates the namespace format
func (v *IDValidator) validateNamespaceFormat(parsed *ParsedNamespace) error {
	if parsed.NamespaceID != emptyValue {
		// Validate namespace ID format using pattern from config or spec
		pattern := getNamespaceValidationPattern()
		namespacePattern := regexp.MustCompile(pattern)
		if !namespacePattern.MatchString(parsed.NamespaceID) {
			return errfmt.Errorf("invalid namespace format: %s", parsed.NamespaceID)
		}
	} else if parsed.Layer != emptyValue {
		// If we parsed a layer but didn't get a namespace ID, the format is invalid
		config := GetGlobalNamespacesConfig()
		var validLayers []string
		if config != nil {
			validLayers = config.GetNamespaceLayers()
		} else {
			validLayers = []string{"zqk", "domain", "integration"} // Fallback
		}
		return errfmt.Errorf("invalid namespace layer: %s (must be one of: %v)", parsed.Layer, validLayers)
	}
	return nil
}

// validateColonFormat validates IDs with colons that didn't parse as namespaces
func (v *IDValidator) validateColonFormat(id string) error {
	parts := strings.Split(id, ":")
	if len(parts) < 2 {
		return nil // Not enough parts to be a namespace
	}

	firstPart := parts[0]
	// Check if it's a valid namespace layer from config
	config := GetGlobalNamespacesConfig()
	if config != nil && !config.IsValidNamespaceLayer(firstPart) {
		// Check if it's a short format (object_type:object_id)
		// Short format should have exactly 2 parts
		if len(parts) > 2 {
			validLayers := config.GetNamespaceLayers()
			return errfmt.Errorf("invalid namespace format: %s (unknown layer: %s, must be one of: %v)", id, firstPart, validLayers)
		}
	}
	return nil
}

// validateActualID validates the actual ID (without namespace) against config
func (v *IDValidator) validateActualID(actualID string, config *IDPatternConfig) (bool, error) {
	// Check if ID starts with any valid prefix (preferred method, config-based)
	if len(config.Prefixes) > 0 {
		if v.validateByPrefixes(actualID, config.Prefixes) {
			return true, nil
		}
		// Prefixes didn't match - fall back to pattern if available
		// This allows patterns like ^POL-[A-Z]+- to accept any category (POL-AGENT-, POL-OBS-, etc.)
		// even if specific prefixes aren't in the config list
		if config.Pattern != emptyValue {
			return v.validateByPattern(actualID, config.Pattern)
		}
		// No pattern fallback - prefix validation failed
		return false, nil
	}

	// No prefixes - use pattern if available
	if config.Pattern != emptyValue {
		return v.validateByPattern(actualID, config.Pattern)
	}

	// No pattern or prefixes - permissive
	return true, nil
}

// validateByPrefixes checks if ID starts with any valid prefix
func (v *IDValidator) validateByPrefixes(id string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// validateByPattern validates ID against a regex pattern
func (v *IDValidator) validateByPattern(id, pattern string) (bool, error) {
	re, err := GetCachedRegexp(pattern)
	if err != nil {
		return false, errfmt.Newf("invalid pattern").Wrap(err)
	}
	return re.MatchString(id), nil
}

// HasLoadedPattern reports whether kind has a configured ID pattern.
// Callers must use this instead of reading patterns or mu.
func (v *IDValidator) HasLoadedPattern(kind string) bool {
	if v == nil {
		return false
	}
	var has bool
	if err := concurrency.RunInRLockWithLogger(
		&v.mu,
		LockNameInstanceValidatorCheckPatterns,
		lockLoggerSystem(),
		func() error {
			has = v.patterns[kind] != nil
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	return has
}

// GetValidPrefixes returns valid prefixes for a given kind
func (v *IDValidator) GetValidPrefixes(kind string) []string {
	var config *IDPatternConfig
	var exists bool
	if err := concurrency.RunInRLockWithLogger(
		&v.mu,
		LockNameIdValidatorGetValidPrefixes,
		lockLoggerSystem(),
		func() error {
			var ok bool
			config, ok = v.patterns[kind]
			exists = ok
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

	if !exists {
		return []string{}
	}

	return config.Prefixes
}

// InferKindFromID attempts to infer the object kind from an ID
// Now supports namespaced IDs (e.g., "zqk:kernel:goal:GOAL-123")
func (v *IDValidator) InferKindFromID(id string) string {
	if err :=
		// Ensure patterns are loaded (non-blocking - if already loading, will use what's available)
		v.LoadPatterns(); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

	var patternsCopy map[string]*IDPatternConfig
	if err := concurrency.RunInRLockWithLogger(
		&v.mu,
		LockNameIdValidatorInferKind,
		lockLoggerSystem(),
		func() error {
			// Copy patterns for processing outside lock
			patternsCopy = make(map[string]*IDPatternConfig, len(v.patterns))
			for kind, config := range v.patterns {
				patternsCopy[kind] = config
			}
			return nil
		},
	); err !=

		// Parse namespace if present
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

	parsed := ParseNamespace(id)
	if parsed != nil && parsed.ObjectType != emptyValue {
		// Namespaced ID with object type - use the object type directly
		return parsed.ObjectType
	}

	// Extract the actual ID to check (without namespace)
	actualID := id
	if parsed != nil && parsed.ObjectID != emptyValue {
		actualID = parsed.ObjectID
	}

	// Check config-based patterns first (they take precedence)
	// Then check spec-based patterns
	// Use longest-prefix match so NAMESPACE-REGISTRY-001 maps to namespace_registry, not namespace
	config := v.getIDPrefixesConfig()
	configKinds := make(map[string]bool)
	if config != nil {
		for kind := range config.KindToPrefixes {
			configKinds[kind] = true
		}
	}

	type prefixKind struct {
		prefix string
		kind   string
	}
	collectCandidates := func(includeConfig bool) []prefixKind {
		var cands []prefixKind
		for kind, patternConfig := range patternsCopy {
			if configKinds[kind] != includeConfig {
				continue
			}
			for _, prefix := range patternConfig.Prefixes {
				if strings.HasPrefix(actualID, prefix) {
					cands = append(cands, prefixKind{prefix: prefix, kind: kind})
				}
			}
		}
		sort.Slice(cands, func(i, j int) bool {
			return len(cands[i].prefix) > len(cands[j].prefix)
		})
		return cands
	}
	// First pass: config-based (longest-prefix match)
	if cands := collectCandidates(true); len(cands) > 0 {
		return cands[0].kind
	}
	// Second pass: spec-based (longest-prefix match)
	if cands := collectCandidates(false); len(cands) > 0 {
		return cands[0].kind
	}
	return ""
}
