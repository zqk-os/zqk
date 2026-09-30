package validation

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

const emptyValue = ""

// Namespace layer constants - DEPRECATED: Use NamespacesConfig instead
// These are kept for backward compatibility but should not be used in new code
const (
	NamespaceLayerZqk         = "zqk"
	NamespaceLayerDomain      = "domain"
	NamespaceLayerIntegration = "integration"
)

// NamespaceLayerPattern is the regex pattern for namespace ID validation - DEPRECATED
// Use GetNamespaceValidationPattern() from NamespacesConfig instead
var NamespaceLayerPattern = getNamespaceValidationPattern()

// getNamespaceValidationPattern returns the namespace validation pattern from config
// Falls back to spec file pattern, then to default
func getNamespaceValidationPattern() string {
	// First try to get from config
	config := GetGlobalNamespacesConfig()
	if config != nil {
		pattern := config.GetNamespaceValidationPattern()
		if pattern != emptyValue {
			return pattern
		}
	}

	// Fallback: try to read from spec file
	// This ensures the spec file is the source of truth
	specLoader := objects.GetGlobalSpecLoader()
	if specLoader != nil {
		spec, err := specLoader.LoadSpecWithInheritance("base_object.yaml")
		if err == nil && spec != nil {
			if fields, ok := spec.Fields[objects.FieldKeyNamespaceID].(map[string]any); ok {
				if validation, ok := fields["validation"].(map[string]any); ok {
					if pattern := objects.GetString(validation, "pattern"); pattern != emptyValue {
						return pattern
					}
				}
			}
		}
	}

	// Ultimate fallback
	return `^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$`
}

// ValidNamespaceLayers returns valid namespace layers from config - DEPRECATED
// Use NamespacesConfig.GetNamespaceLayers() instead
var ValidNamespaceLayers = getValidNamespaceLayers()

func getValidNamespaceLayers() []string {
	config := GetGlobalNamespacesConfig()
	if config != nil {
		return config.GetNamespaceLayers()
	}
	return []string{"zqk", "domain", "integration"} // Fallback
}

// Common namespace ID constants - DEPRECATED: Use NamespacesConfig instead
// These are now functions to avoid initialization cycles
var (
	DefaultNamespaceKernel         = getDefaultNamespaceKernel()
	DefaultNamespaceKernelCLI      = getDefaultNamespaceKernelCLI()
	DefaultNamespaceKernelMetrics  = getDefaultNamespaceKernelMetrics()
	DefaultNamespaceOrganizational = getDefaultNamespaceOrganizational()
)

func getDefaultNamespaceKernel() string {
	config := GetGlobalNamespacesConfig()
	if config != nil {
		layers := config.GetNamespaceLayers()
		if len(layers) > 0 {
			return layers[0] + ":kernel"
		}
	}
	return "zqk:kernel" // Fallback
}

func getDefaultNamespaceKernelCLI() string {
	return getDefaultNamespaceKernel() + ":cli"
}

func getDefaultNamespaceKernelMetrics() string {
	return getDefaultNamespaceKernel() + ":metrics"
}

func getDefaultNamespaceOrganizational() string {
	config := GetGlobalNamespacesConfig()
	if config != nil {
		layers := config.GetNamespaceLayers()
		if len(layers) > 1 {
			return layers[1] + ":organizational"
		}
	}
	return "domain:organizational" // Fallback
}

// System origin constants - DEPRECATED: Use NamespacesConfig instead
var (
	DefaultOriginSystem  = getDefaultOriginSystem()
	DefaultOriginProject = getDefaultOriginProject()
)

func getDefaultOriginSystem() string {
	config := GetGlobalNamespacesConfig()
	if config != nil {
		return config.GetSystemOrigin()
	}
	return "zqk" // Fallback
}

func getDefaultOriginProject() string {
	config := GetGlobalNamespacesConfig()
	if config != nil {
		return config.GetProjectOrigin()
	}
	return "zqk" // Fallback
}
