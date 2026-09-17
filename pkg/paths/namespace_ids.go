package paths

import "strings"

const emptyNamespacePrefix = ""

// NamespacePrefixDefault is the default namespace prefix used for kernel namespaces.
// This is separate from product name and executable name to support white-labeling.
const NamespacePrefixDefault = "zqk"

// NamespacePrefix is the current namespace prefix (e.g., "zqk", "acme").
// This can be overridden at runtime from config (brand.namespace_prefix).
var NamespacePrefix = NamespacePrefixDefault

// CLINamespaceID is the subordinate namespace for CLI profiles (e.g., "zqk:kernel:cli").
var CLINamespaceID = NamespacePrefixDefault + ":kernel:cli"

// MetricsNamespaceID is the subordinate namespace for metrics profiles (e.g., "zqk:kernel:metrics").
var MetricsNamespaceID = NamespacePrefixDefault + ":kernel:metrics"

// KernelNamespaceID is the base kernel namespace (e.g., "zqk:kernel").
var KernelNamespaceID = NamespacePrefixDefault + ":kernel"

// SetNamespacePrefix sets the namespace prefix and updates derived namespace IDs.
func SetNamespacePrefix(prefix string) {
	prefix = strings.TrimSpace(prefix)
	if prefix == emptyNamespacePrefix {
		return
	}
	NamespacePrefix = prefix
	KernelNamespaceID = prefix + ":kernel"
	CLINamespaceID = KernelNamespaceID + ":cli"
	MetricsNamespaceID = KernelNamespaceID + ":metrics"
}
