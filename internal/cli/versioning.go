package cli

import (
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

const (
	versionFlagName                 = "version"
	versionFlagHelpTemplate         = "Command version to use (default: %s)"
	versionLogFieldVersion          = "version"
	versionLogFieldCommand          = "command"
	versionLogFieldReplacedBy       = "replaced_by"
	versionLogMsgDeprecatedSelected = "Using deprecated command version"
)

// CommandVersion represents a versioned command
type CommandVersion struct {
	Version     string
	Description string
	Deprecated  bool
	ReplacedBy  string // Version that replaces this one
}

// VersionedCommandRegistry manages multiple versions of the same command
type VersionedCommandRegistry struct {
	commandVersions map[string][]CommandVersion // command -> versions
	defaultVersions map[string]string           // command -> default version
}

// NewVersionedCommandRegistry creates a new versioned command registry
func NewVersionedCommandRegistry() *VersionedCommandRegistry {
	return &VersionedCommandRegistry{
		commandVersions: make(map[string][]CommandVersion),
		defaultVersions: make(map[string]string),
	}
}

// RegisterVersion registers a version of a command
func (vcr *VersionedCommandRegistry) RegisterVersion(command string, version CommandVersion) {
	if vcr.commandVersions[command] == nil {
		vcr.commandVersions[command] = []CommandVersion{}
	}
	vcr.commandVersions[command] = append(vcr.commandVersions[command], version)
}

// SetDefaultVersion sets the default version for a command
func (vcr *VersionedCommandRegistry) SetDefaultVersion(command, version string) {
	vcr.defaultVersions[command] = version
}

// GetDefaultVersion returns the default version for a command
func (vcr *VersionedCommandRegistry) GetDefaultVersion(command string) string {
	if version, ok := vcr.defaultVersions[command]; ok {
		return version
	}
	// Return latest non-deprecated version
	versions := vcr.commandVersions[command]
	for i := len(versions) - 1; i >= 0; i-- {
		if !versions[i].Deprecated {
			return versions[i].Version
		}
	}
	// Fallback to latest version
	if len(versions) > 0 {
		return versions[len(versions)-1].Version
	}
	return ""
}

// CreateVersionedCommand creates a command with version support
// Usage: zqk migrate (uses default version) or zqk migrate --version 2.0.0
func CreateVersionedCommand(
	name string,
	short string,
	long string,
	versions []CommandVersion,
	defaultVersion string,
	createHandler func(version string) func(*cobra.Command, []string) error,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Long:  long,
	}

	// Add version flag
	var versionFlag string
	cmd.Flags().StringVar(&versionFlag, versionFlagName, defaultVersion,
		fmt.Sprintf(versionFlagHelpTemplate, defaultVersion))

	// List available versions in help
	if len(versions) > 1 {
		cmd.Long += "\n\nAvailable versions:"
		for _, v := range versions {
			status := ""
			if v.Deprecated {
				status = " (deprecated"
				if v.ReplacedBy != emptyValue {
					status += fmt.Sprintf(", replaced by %s", v.ReplacedBy)
				}
				status += ")"
			}
			cmd.Long += fmt.Sprintf("\n  %s: %s%s", v.Version, v.Description, status)
		}
	}

	// Create handler that routes to appropriate version
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		selectedVersion := versionFlag
		if selectedVersion == emptyValue {
			selectedVersion = defaultVersion
		}

		// Validate version exists
		versionExists := false
		for _, v := range versions {
			if v.Version == selectedVersion {
				versionExists = true
				if v.Deprecated {
					eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())
					fields := []logging.Field{
						logging.String(versionLogFieldVersion, selectedVersion),
						logging.String(versionLogFieldCommand, name),
					}
					if v.ReplacedBy != emptyValue {
						fields = append(fields, logging.String(versionLogFieldReplacedBy, v.ReplacedBy))
					}
					eventLogger.LogWarning(versionLogMsgDeprecatedSelected, fields...)
				}
				break
			}
		}

		if !versionExists {
			return errfmt.Errorf("version %s not available. Available versions: %s",
				selectedVersion, getVersionList(versions))
		}

		// Route to version-specific handler
		handler := createHandler(selectedVersion)
		return handler(cmd, args)
	}

	return cmd
}

// getVersionList returns a comma-separated list of available versions
func getVersionList(versions []CommandVersion) string {
	var list []string
	for _, v := range versions {
		list = append(list, v.Version)
	}
	return strings.Join(list, ", ")
}

// VersionNegotiation handles version negotiation between CLI and external binaries
type VersionNegotiation struct {
	CLIVersion       string
	MinBinaryVersion string
	MaxBinaryVersion string
	PreferredVersion string
}

// NegotiateVersion negotiates the best version to use
func NegotiateVersion(negotiation VersionNegotiation, availableVersions []string) (string, error) {
	// If preferred version is available, use it
	for _, v := range availableVersions {
		if v == negotiation.PreferredVersion {
			return v, nil
		}
	}

	// Find version within min/max range
	for _, v := range availableVersions {
		// Simple version comparison (can be enhanced with proper semver parsing)
		if negotiation.MinBinaryVersion != emptyValue && v < negotiation.MinBinaryVersion {
			continue
		}
		if negotiation.MaxBinaryVersion != emptyValue && v > negotiation.MaxBinaryVersion {
			continue
		}
		// Use first compatible version
		return v, nil
	}

	return "", errfmt.Errorf("no compatible version found (required: >=%s, <=%s)",
		negotiation.MinBinaryVersion, negotiation.MaxBinaryVersion)
}
