package transceiver

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RoutingRuleConfig represents a routing rule loaded from YAML
type RoutingRuleConfig struct {
	Name        string               `yaml:"name"`
	Description string               `yaml:"description,omitempty"`
	Enabled     bool                 `yaml:"enabled"`
	Priority    int                  `yaml:"priority"`
	Match       MessageMatcherConfig `yaml:"match"`
	Actions     []types.Action       `yaml:"actions"`
}

// MessageMatcherConfig represents matching criteria in YAML
type MessageMatcherConfig struct {
	EventType   string            `yaml:"event_type,omitempty"`
	Source      string            `yaml:"source,omitempty"`
	JobID       string            `yaml:"job_id,omitempty"`
	JobCategory string            `yaml:"job_category,omitempty"`
	JobType     string            `yaml:"job_type,omitempty"`
	Severity    string            `yaml:"severity,omitempty"`
	Conditions  []ConditionConfig `yaml:"conditions,omitempty"`
}

// ConditionConfig represents a condition in YAML
type ConditionConfig struct {
	Field    string `yaml:"field"`
	Operator string `yaml:"operator"`
	Value    any    `yaml:"value"`
}

// RoutingRuleLoader loads routing rules from YAML files
type RoutingRuleLoader struct {
	rulesDir string
	logger   logging.Logger
}

// NewRoutingRuleLoader creates a new routing rule loader
func NewRoutingRuleLoader(rulesDir string, logger logging.Logger) *RoutingRuleLoader {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	if rulesDir == emptyValue {
		rulesDir = findRoutingRulesDir()
	}
	return &RoutingRuleLoader{
		rulesDir: rulesDir,
		logger:   logger,
	}
}

// LoadRulesFromFile loads routing rules from a single YAML file
func (rl *RoutingRuleLoader) LoadRulesFromFile(filePath string) ([]RoutingRule, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read routing rules file %s: %w", filePath, err)
	}

	var configs []RoutingRuleConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, errfmt.Errorf("failed to parse routing rules file %s: %w", filePath, err)
	}

	rules := make([]RoutingRule, 0, len(configs))
	//nolint:gocritic // rangeValCopy: RoutingRuleConfig copy acceptable for parsing
	for _, config := range configs {
		rule, err := rl.convertConfigToRule(config)
		if err != nil {
			logging.Fluent(rl.logger).Warn(LogEventSchedulerTransceiverRuleConvertConfigFailed).
				RuleConfigName(config.Name).
				WithError(err).
				Log()
			continue
		}
		rules = append(rules, *rule)
	}

	return rules, nil
}

// LoadAndValidateRules loads and validates routing rules from a file
func (rl *RoutingRuleLoader) LoadAndValidateRules(filePath string, router *Router) ([]RoutingRule, []ValidationError, error) {
	rules, err := rl.LoadRulesFromFile(filePath)
	if err != nil {
		return nil, nil, err
	}

	errors := ValidateRoutingRules(rules, router, rl.logger)
	return rules, errors, nil
}

// LoadAllRules loads all routing rules from the rules directory
func (rl *RoutingRuleLoader) LoadAllRules() ([]RoutingRule, error) {
	if rl.rulesDir == emptyValue {
		logging.Fluent(rl.logger).Debug(LogEventSchedulerTransceiverRuleLoaderNoDir).Log()
		return []RoutingRule{}, nil
	}

	entries, err := fileutil.ReadDir(rl.rulesDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			logging.Fluent(rl.logger).Debug(LogEventSchedulerTransceiverRuleLoaderDirMissing).
				RulesDir(rl.rulesDir).
				Log()
			return []RoutingRule{}, nil
		}
		return nil, errfmt.Newf("failed to read routing rules directory").Wrap(err)
	}

	var allRules []RoutingRule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		filePath := filepath.Join(rl.rulesDir, entry.Name())
		rules, err := rl.LoadRulesFromFile(filePath)
		if err != nil {
			logging.Fluent(rl.logger).Warn(LogEventSchedulerTransceiverRuleLoadFileFailed).
				File(entry.Name()).
				WithError(err).
				Log()
			continue
		}

		allRules = append(allRules, rules...)
		logging.Fluent(rl.logger).Debug(LogEventSchedulerTransceiverRuleLoadedFromFile).
			File(entry.Name()).
			RuleCount(len(rules)).
			Log()
	}

	return allRules, nil
}

// convertConfigToRule converts a RoutingRuleConfig to a RoutingRule
//
//nolint:gocritic,unparam // config passed by value for immutability; error return always nil but kept for interface consistency
func (rl *RoutingRuleLoader) convertConfigToRule(config RoutingRuleConfig) (*RoutingRule, error) {
	// Convert matcher
	matcher := MessageMatcher{
		EventType:   config.Match.EventType,
		Source:      config.Match.Source,
		JobID:       config.Match.JobID,
		JobCategory: config.Match.JobCategory,
		JobType:     config.Match.JobType,
		Severity:    config.Match.Severity,
		Conditions:  make([]Condition, 0, len(config.Match.Conditions)),
	}

	for _, condConfig := range config.Match.Conditions {
		matcher.Conditions = append(matcher.Conditions, Condition(condConfig))
	}

	rule := &RoutingRule{
		Name:        config.Name,
		Description: config.Description,
		Enabled:     config.Enabled,
		Priority:    config.Priority,
		Match:       matcher,
		Actions:     config.Actions,
	}

	return rule, nil
}

// findRoutingRulesDir attempts to find the routing rules directory
func findRoutingRulesDir() string {
	return paths.FirstExistingFromCwd(filepath.Join(paths.ProcessInternalDir, "routing_rules"))
}


// LoadDefaultRules returns default routing rules (can be used as fallback)
func LoadDefaultRules() []RoutingRule {
	// Return empty rules by default - let users configure their own
	// This can be extended with sensible defaults if needed
	return []RoutingRule{}
}
