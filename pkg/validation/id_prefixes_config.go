package validation

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kindnames"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// IDPrefixesConfig represents the configuration for kind-to-prefix mappings
// Also includes kind-to-synonym mappings for standardized alias support
type IDPrefixesConfig struct {
	Version         string                `yaml:"version"`
	Description     string                `yaml:"description"`
	KindToPrefixes  map[string][]string   `yaml:"kind_to_prefixes"`
	KindToSynonyms  map[string][]string   `yaml:"kind_to_synonyms,omitempty"` // Optional: explicit synonym mappings
	InferenceRules  InferenceRulesConfig  `yaml:"inference_rules"`
	DefaultStrategy DefaultStrategyConfig `yaml:"default_strategy"`
}

// InferenceRulesConfig defines patterns for inferring prefixes from kinds
// Can also be used for synonym inference
type InferenceRulesConfig struct {
	Patterns        []InferencePattern        `yaml:"patterns"`
	SynonymPatterns []SynonymInferencePattern `yaml:"synonym_patterns,omitempty"` // Optional: patterns for synonym inference
}

// SynonymInferencePattern defines a pattern for inferring synonyms from kind names
type SynonymInferencePattern struct {
	Pattern    string   `yaml:"pattern"`    // Regex pattern to match kind names
	Strategies []string `yaml:"strategies"` // Strategies to apply: "first_letters", "first_4", "abbrev", etc.
	Example    string   `yaml:"example,omitempty"`
}

// InferencePattern defines a regex pattern and prefix template for inference
type InferencePattern struct {
	Pattern        string `yaml:"pattern"`
	PrefixTemplate string `yaml:"prefix_template"`
	Example        string `yaml:"example,omitempty"`
}

// DefaultStrategyConfig defines the default prefix generation strategy
type DefaultStrategyConfig struct {
	Method  string `yaml:"method"`
	Example string `yaml:"example,omitempty"`
}

// ResetGlobalIDPrefixesConfig drops the process-wide prefixes memo (tests that chdir).
func ResetGlobalIDPrefixesConfig() {
	idPrefixConfigs.Reset()
	resetDiscoveredPaths()
}

// LoadIDPrefixesConfig loads the ID prefixes configuration from file
// Logs error events for aggregation/analysis when load fails

func LoadIDPrefixesConfig(configPath string) (*IDPrefixesConfig, error) {
	if configPath == emptyValue {
		configPath = findIDPrefixesConfig()
		if configPath == emptyValue {
			eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())
			err := errfmt.Errorf("ID prefixes config file not found")
			logging.FluentEvent(eventLogger).Error("Failed to load ID prefixes config", err).
				String("event", "config_load").
				String("config_type", "id_prefixes_config").
				String("error_type", "file_not_found").
				String("fallback_to_default", "true").
				Log()
			return nil, err
		}
	}

	cfg, err := idPrefixConfigs.Load(configPath, stampmemo.Of(configPath), func() (*IDPrefixesConfig, error) {
		return parseIDPrefixesConfigFile(configPath)
	})
	if err != nil {
		return nil, err
	}
	return cloneIDPrefixesConfig(cfg), nil
}

func parseIDPrefixesConfigFile(configPath string) (*IDPrefixesConfig, error) {
	eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())

	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		loadErr := errfmt.Newf("failed to read ID prefixes config").Wrap(err)
		errorType := "read_error"
		if strings.Contains(loadErr.Error(), "permission") {
			errorType = "permission_denied"
		}
		logging.FluentEvent(eventLogger).Error("Failed to load ID prefixes config", loadErr).
			String("event", "config_load").
			String("config_type", "id_prefixes_config").
			String("config_file", configPath).
			String("error_type", errorType).
			String("fallback_to_default", "true").
			Log()
		return nil, loadErr
	}

	var config IDPrefixesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		parseErr := errfmt.Newf("failed to parse ID prefixes config").Wrap(err)
		parseDetails := parseErr.Error()
		logging.FluentEvent(eventLogger).Error("Failed to parse ID prefixes config", parseErr).
			String("event", "config_load").
			String("config_type", "id_prefixes_config").
			String("config_file", configPath).
			String("error_type", "parse_error").
			String("parse_error_details", parseDetails).
			String("fallback_to_default", "true").
			Log()
		return nil, parseErr
	}

	return &config, nil
}

// GetGlobalIDPrefixesConfig returns the ID prefixes config for the discovered file.
// All mappings are externalized to id_prefixes_config.yaml
// Returns default config if file doesn't exist (for backward compatibility)
func GetGlobalIDPrefixesConfig() *IDPrefixesConfig {
	configPath := findIDPrefixesConfig()
	cfg, err := LoadIDPrefixesConfig(configPath)
	if err != nil || cfg == nil {
		eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())
		eventLogger.LogWarning("Falling back to default ID prefixes config", logging.String("event", "config_load"),
			logging.String("config_type", "id_prefixes_config"),
			logging.String("config_file", configPath),
			logging.String("fallback_to_default", "true"))
		return getDefaultIDPrefixesConfig()
	}
	return cfg
}

// getDefaultIDPrefixesConfig returns a default config with common prefixes
// Used when config file doesn't exist (for backward compatibility)

func getDefaultIDPrefixesConfig() *IDPrefixesConfig {
	return &IDPrefixesConfig{
		KindToPrefixes: map[string][]string{
			kindnames.AgentInstruction:    {"AGI-"},
			kindnames.AgentSkill:          {"ASK-"},
			kindnames.AgentTask:           {"ATK-"},
			kindnames.Persona:             {"PER-"},
			kindnames.ConvergenceSession:  {"CVS-"},
			kindnames.AgentFeed:           {"AGF-"},
			kindnames.BacklogItem:         {"BLI-"},
			kindnames.Milestone:           {"MIL-"},
			kindnames.Goal:                {"GOAL-"},
			kindnames.Workstream:          {"WS-"},
			kindnames.PriorityPlan:        {"PRI-", "PRIO-"},
			kindnames.Requirement:         {"REQ-", "REQU-"},
			kindnames.TestCase:            {"TEST-"},
			kindnames.Criteria:            {"CRIT-"},
			kindnames.Decision:            {"DEC-"},
			kindnames.Roadmap:             {"ROAD-"},
			kindnames.Mission:             {"MIS-"},
			objects.FieldKeyVision:        {"VIS-"},
			kindnames.Component:           {"COMP-"},
			objects.FieldKeyDisplay:       {"DSP-"},
			kindnames.Account:             {"ACC-"},
			objects.FieldKeyRole:          {"ROL-"},
			kindnames.Release:             {"REL-"},
			kindnames.RemoteKernel:        {"REM-"},
			kindnames.CorporateInitiative: {"CI-"}, kindnames.KindSynonym: {"SYN-"},
			kindnames.AuditEvent:             {"AUD-"},
			kindnames.BaseMetric:             {"BAS-"},
			kindnames.SchedulerHealthMetric:  {"SHM-"},
			kindnames.KindMappingMetric:      {"KMM-"},
			kindnames.CommandMetric:          {"CMD-"},
			kindnames.FileLockMetric:         {"FLM-"},
			kindnames.AuditAggregationMetric: {"AAM-"},
			kindnames.StrategicPlan:          {"STRAT-PLAN-"},
			kindnames.WorkstreamTransition:   {"WST-"},
			kindnames.Policy:                 {"POL-"},
			kindnames.TechnicalDebt:          {"TDE-"},
			objects.KindDocEntry:             {"DOC-"},
		},
		KindToSynonyms: map[string][]string{
			// Two-word kinds
			kindnames.AgentInstruction:    {"agi", "instruction"},
			kindnames.AgentSkill:          {"ask", "skill"},
			kindnames.AgentTask:           {"atk", "task"},
			kindnames.Persona:             {"per"},
			kindnames.ConvergenceSession:  {"cvs"},
			kindnames.AgentFeed:           {"agf", "feed"},
			kindnames.BacklogItem:         {"bli", "bl-item", "backlog", "bl"},
			kindnames.PriorityPlan:        {"pplan", "p-plan", "plan", "pp"},
			kindnames.TestCase:            {"test", "tc", "testcase"},
			kindnames.AuditEvent:          {"ae", "audit", "event"},
			kindnames.ChangeJournalEntry:  {"cje", "journal", "change_journal"},
			kindnames.ObjectSpec:          {"spec", "ospec"},
			kindnames.CorporateInitiative: {"ci", "initiative", "corp_init"},
			// Single-word kinds
			kindnames.Component:     {"comp"},
			kindnames.Criteria:      {"crit"},
			kindnames.Requirement:   {"req", "requ"},
			kindnames.Milestone:     {"mil", "ms"},
			kindnames.Workstream:    {"ws", "work"},
			kindnames.Roadmap:       {"road", "rm"},
			kindnames.Decision:      {"dec"},
			kindnames.Account:       {"acc"},
			kindnames.Goal:          {"go"},
			objects.FieldKeyRole:    {"rol"},
			kindnames.Release:       {"rel"},
			kindnames.Mission:       {"mis"},
			objects.FieldKeyVision:  {"vis"},
			objects.FieldKeyDisplay: {"dsp"},
			kindnames.Template:      {"tpl", "tmpl"},
			kindnames.Lifecycle:     {"lc", "life"},
		},
		InferenceRules: InferenceRulesConfig{
			Patterns: []InferencePattern{},
		},
		DefaultStrategy: DefaultStrategyConfig{
			Method: "first_part_upper_3",
		},
	}
}

// GetPrefixesForKind returns the prefixes for a given kind
// Returns empty slice if no mapping exists
func (c *IDPrefixesConfig) GetPrefixesForKind(kind string) []string {
	// Check explicit mapping first
	if prefixes, ok := c.KindToPrefixes[kind]; ok {
		return prefixes
	}

	// Apply inference rules
	return c.inferPrefixFromKind(kind)
}

// HasExplicitMapping checks if a kind has an explicit mapping in the config
// (not inferred)
func (c *IDPrefixesConfig) HasExplicitMapping(kind string) bool {
	_, ok := c.KindToPrefixes[kind]
	return ok
}

// GetSynonymsForKind returns the synonyms for a given kind
// Returns empty slice if no mapping exists
// Uses explicit mappings first, then applies inference patterns if available
func (c *IDPrefixesConfig) GetSynonymsForKind(kind string) []string {
	if c.KindToSynonyms == nil {
		c.KindToSynonyms = make(map[string][]string)
	}

	// Check explicit mapping first
	if synonyms, ok := c.KindToSynonyms[kind]; ok {
		return synonyms
	}

	// Apply inference patterns if available
	if c.InferenceRules.SynonymPatterns != nil {
		if inferred := c.inferSynonymsFromKind(kind); len(inferred) > 0 {
			return inferred
		}
	}

	return []string{}
}

// inferSynonymsFromKind applies inference patterns to determine synonyms from kind
func (c *IDPrefixesConfig) inferSynonymsFromKind(kind string) []string {
	synonyms := []string{}

	for _, pattern := range c.InferenceRules.SynonymPatterns {
		matched, err := matchPattern(pattern.Pattern, kind)
		if err != nil || !matched {
			continue
		}

		// Apply strategies
		for _, strategy := range pattern.Strategies {
			switch strategy {
			case "first_letters":
				// First letter of each word (for multi-word kinds)
				if parts := strings.Split(kind, "_"); len(parts) > 1 {
					abbrev := ""
					for _, part := range parts {
						if len(part) > 0 {
							abbrev += string(part[0])
						}
					}
					if abbrev != emptyValue {
						synonyms = append(synonyms, abbrev)
					}
				}
			case "first_4":
				// First 4 letters (for single-word kinds)
				if !strings.Contains(kind, "_") && len(kind) >= 4 {
					synonyms = append(synonyms, kind[:4])
				}
			case "two_word_abbrev":
				// First letter of first two words (for two-word kinds)
				if parts := strings.Split(kind, "_"); len(parts) == 2 {
					if len(parts[0]) > 0 && len(parts[1]) > 0 {
						abbrev := string(parts[0][0]) + string(parts[1][0])
						synonyms = append(synonyms, abbrev)
					}
				}
			}
		}
	}

	return synonyms
}

// matchPattern matches a simple pattern against a string
// Supports basic patterns like "*_item", "*_metric", etc.
// For full regex support, this could be enhanced
func matchPattern(pattern, text string) (bool, error) {
	// Simple wildcard matching for now
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(text, suffix), nil
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(text, prefix), nil
	}
	// Exact match
	return pattern == text, nil
}

// inferPrefixFromKind applies inference rules to determine prefix from kind
func (c *IDPrefixesConfig) inferPrefixFromKind(kind string) []string {
	// Try inference patterns (simple string matching for now)
	// In the future, could use regexp.Compile for full regex support
	if strings.Contains(kind, "_item") {
		// Handle _item suffix
		base := strings.TrimSuffix(kind, "_item")
		if len(base) >= 3 {
			prefix := strings.ToUpper(base[:3]) + "-"
			return []string{prefix}
		}
	}

	if strings.HasSuffix(kind, "_metric") {
		// Handle _metric suffix
		base := strings.TrimSuffix(kind, "_metric")
		parts := strings.Split(base, "_")
		if len(parts) > 0 && len(parts[0]) >= 3 {
			prefix := strings.ToUpper(parts[0][:3]) + "-"
			return []string{prefix}
		}
	}

	if strings.Contains(kind, "_") {
		// Handle compound words
		parts := strings.Split(kind, "_")
		if len(parts) > 0 && len(parts[0]) >= 3 {
			prefix := strings.ToUpper(parts[0][:3]) + "-"
			return []string{prefix}
		}
	}

	// Default strategy: use first 3 letters of first part
	parts := strings.Split(kind, "_")
	if len(parts) > 0 {
		firstPart := strings.ToUpper(parts[0])
		if len(firstPart) >= 3 {
			prefix := firstPart[:3] + "-"
			return []string{prefix}
		}
	}

	return []string{}
}

// findIDPrefixesConfig finds the ID prefixes config file
// Uses paths configuration instead of hardcoded paths
func findIDPrefixesConfig() string {
	config := GetGlobalPathsConfig()
	if config != nil {
		if path := config.FindPath("id_prefixes_config"); path != emptyValue {
			return path
		}
	}

	// Fallback to default if config not available
	return findPathWithDefaults(filepath.Join(paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile), false)
}
