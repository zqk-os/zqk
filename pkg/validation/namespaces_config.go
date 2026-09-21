package validation

import (
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kindnames"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NamespacesConfig represents the configuration for namespace mappings
type NamespacesConfig struct {
	Version                    string                     `yaml:"version"`
	Description                string                     `yaml:"description"`
	DefaultNamespace           string                     `yaml:"default_namespace"`
	Namespaces                 map[string]NamespaceConfig `yaml:"namespaces"`
	InferenceRules             []NamespaceInferenceRule   `yaml:"inference_rules"`
	NamespaceLayers            []string                   `yaml:"namespace_layers"`             // Valid namespace layers (e.g., ["zqk", "domain", "integration"])
	NamespaceValidationPattern string                     `yaml:"namespace_validation_pattern"` // Regex pattern for namespace ID validation
	SystemOrigin               string                     `yaml:"system_origin"`                // System origin name (e.g., "zqk")
	ProjectOrigin              string                     `yaml:"project_origin"`               // Project origin name (e.g., "zqk")
}

// NamespaceConfig defines a namespace and its associated kinds
type NamespaceConfig struct {
	Description           string   `yaml:"description"`
	ParentNamespace       string   `yaml:"parent_namespace,omitempty"`       // For subordinate namespaces
	SubordinateNamespaces []string `yaml:"subordinate_namespaces,omitempty"` // Child namespaces
	Kinds                 []string `yaml:"kinds"`
}

// NamespaceInferenceRule defines a pattern for inferring namespaces from kinds
type NamespaceInferenceRule struct {
	Pattern           string `yaml:"pattern"`
	Namespace         string `yaml:"namespace"`          // Direct namespace assignment
	NamespaceTemplate string `yaml:"namespace_template"` // Template for dynamic namespace (e.g., "integration:{domain}")
	Example           string `yaml:"example,omitempty"`
}

const (
	namespaceLayerZQK         = "zqk"
	namespaceLayerDomain      = "domain"
	namespaceLayerIntegration = "integration"
)

// ResetGlobalNamespacesConfig drops the process-wide namespaces memo (tests that chdir).
func ResetGlobalNamespacesConfig() {
	namespaceConfigs.Reset()
}

// LoadNamespacesConfig loads the namespaces configuration from file
func LoadNamespacesConfig(configPath string) (*NamespacesConfig, error) {
	if configPath == emptyValue {
		configPath = findNamespacesConfig()
		if configPath == emptyValue {
			return getDefaultNamespacesConfig(), nil
		}
	}

	cfg, err := namespaceConfigs.Load(configPath, stampmemo.Of(configPath), func() (*NamespacesConfig, error) {
		return parseNamespacesConfigFile(configPath), nil
	})
	if err != nil || cfg == nil {
		return getDefaultNamespacesConfig(), nil
	}
	return cloneNamespacesConfig(cfg), nil
}

func parseNamespacesConfigFile(configPath string) *NamespacesConfig {
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return getDefaultNamespacesConfig()
	}

	var config NamespacesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return getDefaultNamespacesConfig()
	}

	config.validate()
	return &config
}

// GetGlobalNamespacesConfig returns the namespaces config for the discovered file.
func GetGlobalNamespacesConfig() *NamespacesConfig {
	cfg, err := LoadNamespacesConfig(findNamespacesConfig())
	if err != nil || cfg == nil {
		return getDefaultNamespacesConfig()
	}
	return cfg
}

// getDefaultNamespacesConfig returns default namespace configuration
func getDefaultNamespacesConfig() *NamespacesConfig {
	// Use hardcoded defaults to avoid circular dependency
	defaultKernel := "zqk:kernel"
	defaultKernelCLI := ConstMagicExtracted_26
	defaultKernelMetrics := ConstMagic07379a5c
	defaultOrganizational := ConstMagic09ae1f1a

	config := &NamespacesConfig{
		DefaultNamespace:           defaultKernel,
		NamespaceLayers:            []string{namespaceLayerZQK, namespaceLayerDomain, namespaceLayerIntegration},
		NamespaceValidationPattern: `^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$`,
		SystemOrigin:               namespaceLayerZQK,
		ProjectOrigin:              namespaceLayerZQK,
		Namespaces: map[string]NamespaceConfig{
			defaultKernel: {
				Description: ConstMagice246c5d2,
				Kinds: []string{
					kindnames.Goal, kindnames.Milestone, kindnames.Workstream, kindnames.PriorityPlan, kindnames.BacklogItem,
					kindnames.Requirement, kindnames.Criteria, kindnames.TestCase,
					kindnames.Policy, kindnames.Decision, kindnames.Question,
					kindnames.Mission, kindnames.Vision, kindnames.Roadmap,
					kindnames.ConvergenceSession,
					kindnames.Account, kindnames.Role, kindnames.Component,
					kindnames.AuditEvent, kindnames.ChangeJournalEntry,
					kindnames.BaseMetric, kindnames.CommandMetric,
					kindnames.CodeReference, kindnames.DocEntry,
					kindnames.Persona, kindnames.Resolver, kindnames.Rule, kindnames.Scenario,
					kindnames.SchedulerJob, kindnames.Template,
					kindnames.ContextRefreshSchedule, kindnames.IntegrityManifest,
					kindnames.MetadataPackage, kindnames.RiskBlocker, kindnames.RollbackReport,
					kindnames.Display, kindnames.Namespace, kindnames.NamespaceRegistry, kindnames.DomainRegistry,
					kindnames.StrategicContext, kindnames.StakeholderProfile, kindnames.ImportantDate,
				},
			},
			defaultKernelCLI: {
				Description:     ConstMagicb2fdcc34,
				ParentNamespace: defaultKernel,
				Kinds: []string{
					"cli_profile",
					"cli_config",
				},
			},
			defaultKernelMetrics: {
				Description:     ConstMagic9fb69e5c,
				ParentNamespace: defaultKernel,
				Kinds:           []string{ConstMagicExtracted_27, ConstMagicExtracted_28, ConstMagicExtracted_29},
			},
			defaultOrganizational: {
				Description: ConstMagiceddb716c,
				Kinds: []string{
					kindnames.Organization, kindnames.Division, kindnames.Department, kindnames.Team, kindnames.Partnership,
				},
			},
		},
		InferenceRules: []NamespaceInferenceRule{
			{
				Pattern:   ConstMagicd182d411,
				Namespace: defaultOrganizational,
			},
			{
				Pattern:           ConstMagic6e6ab915,
				NamespaceTemplate: ConstMagic8cf4f0f2,
			},
		},
	}

	// Validate config
	config.validate()
	return config
}

// validate validates the namespaces config and sets defaults if needed
func (c *NamespacesConfig) validate() {
	// Ensure default_namespace is always set
	if c.DefaultNamespace == emptyValue {
		// Use first layer + ":kernel" as default
		if len(c.NamespaceLayers) > 0 {
			c.DefaultNamespace = c.NamespaceLayers[0] + ":kernel"
		} else {
			c.DefaultNamespace = "zqk:kernel" // Ultimate fallback
		}
	}

	// Ensure Namespaces map is initialized
	if c.Namespaces == nil {
		c.Namespaces = make(map[string]NamespaceConfig)
	}

	// Ensure InferenceRules slice is initialized
	if c.InferenceRules == nil {
		c.InferenceRules = []NamespaceInferenceRule{}
	}

	// Set default namespace layers if not configured
	if len(c.NamespaceLayers) == 0 {
		c.NamespaceLayers = []string{namespaceLayerZQK, namespaceLayerDomain, namespaceLayerIntegration}
	}

	// Set default validation pattern if not configured
	if c.NamespaceValidationPattern == emptyValue {
		// Build pattern from configured layers
		layersPattern := strings.Join(c.NamespaceLayers, "|")
		c.NamespaceValidationPattern = `^(` + layersPattern + `):[a-z0-9_]+(:[a-z0-9_]+)*$`
	}

	// Set default system/project origin if not configured
	if c.SystemOrigin == emptyValue {
		c.SystemOrigin = c.NamespaceLayers[0] // Use first layer as default
	}
	if c.ProjectOrigin == emptyValue {
		c.ProjectOrigin = c.SystemOrigin
	}
}

// GetNamespaceForKind returns the namespace for a given kind
func (c *NamespacesConfig) GetNamespaceForKind(kind string) string {
	// Ensure default_namespace is set (defensive check)
	if c.DefaultNamespace == emptyValue {
		c.DefaultNamespace = DefaultNamespaceKernel
	}

	// Check explicit mappings first
	for namespaceID, nsConfig := range c.Namespaces {
		for _, nsKind := range nsConfig.Kinds {
			if nsKind == kind {
				return namespaceID
			}
		}
	}

	// Apply inference rules (sorted by priority if available)
	rules := make([]NamespaceInferenceRule, len(c.InferenceRules))
	copy(rules, c.InferenceRules)
	// Note: Could add priority field to NamespaceInferenceRule in the future

	eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())

	for _, rule := range rules {
		re, err := GetCachedRegexp(rule.Pattern)
		if err != nil {
			logging.FluentEvent(eventLogger).Warn(ConstMagiced9d41dc).
				String("pattern", rule.Pattern).
				String("kind", kind).
				WithError(err).
				Log()
			continue
		}
		if re.MatchString(kind) {
			if rule.Namespace != emptyValue {
				return rule.Namespace
			}
			if rule.NamespaceTemplate != emptyValue {
				// Handle template (e.g., "integration:{domain}")
				// For now, simple implementation - can be enhanced later
				if strings.Contains(rule.NamespaceTemplate, "{domain}") {
					// Extract domain from kind
					domain := strings.TrimPrefix(strings.TrimSuffix(kind, "_integration"), "integration_")
					return strings.Replace(rule.NamespaceTemplate, "{domain}", domain, 1)
				}
				return rule.NamespaceTemplate
			}
		}
	}

	// Default fallback (guaranteed to be non-empty due to validation)
	return c.DefaultNamespace
}

// GetAllKindsForNamespace returns all kinds associated with a namespace
func (c *NamespacesConfig) GetAllKindsForNamespace(namespaceID string) []string {
	nsConfig, ok := c.Namespaces[namespaceID]
	if !ok {
		return []string{}
	}
	return nsConfig.Kinds
}

// findNamespacesConfig finds the namespaces config file
func findNamespacesConfig() string {
	pathsConfig := GetGlobalPathsConfig()
	if pathsConfig != nil {
		// Try to find using paths config
		configDir := pathsConfig.FindPath("config_dir")
		if configDir != emptyValue {
			configPath := filepath.Join(configDir, paths.NamespacesConfigFile)
			if _, err := fileutil.Stat(configPath); err == nil {
				return configPath
			}
		}
	}

	// Fallback to default search
	return findPathWithDefaults(filepath.Join(paths.ProcessInternalConfigsDir, paths.NamespacesConfigFile), false)
}

// GetNamespaceLayers returns the list of valid namespace layers
func (c *NamespacesConfig) GetNamespaceLayers() []string {
	if len(c.NamespaceLayers) == 0 {
		return []string{namespaceLayerZQK, namespaceLayerDomain, namespaceLayerIntegration} // Fallback
	}
	return c.NamespaceLayers
}

// GetNamespaceValidationPattern returns the regex pattern for namespace validation
func (c *NamespacesConfig) GetNamespaceValidationPattern() string {
	if c.NamespaceValidationPattern == emptyValue {
		// Build pattern from configured layers
		layersPattern := strings.Join(c.GetNamespaceLayers(), "|")
		return `^(` + layersPattern + `):[a-z0-9_]+(:[a-z0-9_]+)*$`
	}
	return c.NamespaceValidationPattern
}

// GetSystemOrigin returns the system origin name
func (c *NamespacesConfig) GetSystemOrigin() string {
	if c.SystemOrigin == emptyValue {
		layers := c.GetNamespaceLayers()
		if len(layers) > 0 {
			return layers[0] // Use first layer as default
		}
		return namespaceLayerZQK // Ultimate fallback
	}
	return c.SystemOrigin
}

// GetProjectOrigin returns the project origin name
func (c *NamespacesConfig) GetProjectOrigin() string {
	if c.ProjectOrigin == emptyValue {
		return c.GetSystemOrigin()
	}
	return c.ProjectOrigin
}

// IsValidNamespaceLayer checks if a layer name is valid
func (c *NamespacesConfig) IsValidNamespaceLayer(layer string) bool {
	return slices.Contains(c.GetNamespaceLayers(), layer)
}
