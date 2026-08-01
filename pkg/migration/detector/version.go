package detector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// Version represents a semantic version
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string // Pre-release identifier (e.g., "alpha", "beta", "rc1")
	Build string // Build metadata
}

// ParseVersion parses a semantic version string
func ParseVersion(versionStr string) (*Version, error) {
	v := &Version{}

	// Remove leading 'v' if present
	versionStr = strings.TrimPrefix(versionStr, "v")

	// Split on '+' for build metadata
	parts := strings.Split(versionStr, "+")
	if len(parts) > 1 {
		v.Build = parts[1]
	}
	versionStr = parts[0]

	// Split on '-' for pre-release
	parts = strings.Split(versionStr, "-")
	if len(parts) > 1 {
		v.Pre = parts[1]
	}
	versionStr = parts[0]

	// Split on '.' for major.minor.patch
	parts = strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return nil, errfmt.Errorf("invalid version format: %s", versionStr)
	}

	var err error
	if v.Major, err = strconv.Atoi(parts[0]); err != nil {
		return nil, errfmt.Newf("invalid major version").Wrap(err)
	}
	if v.Minor, err = strconv.Atoi(parts[1]); err != nil {
		return nil, errfmt.Newf("invalid minor version").Wrap(err)
	}
	if len(parts) > 2 {
		if v.Patch, err = strconv.Atoi(parts[2]); err != nil {
			return nil, errfmt.Newf("invalid patch version").Wrap(err)
		}
	}

	return v, nil
}

// String returns the version as a string
func (v *Version) String() string {
	version := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != emptyValue {
		version += "-" + v.Pre
	}
	if v.Build != emptyValue {
		version += "+" + v.Build
	}
	return version
}

// Compare compares two versions
// Returns: -1 if v < other, 0 if v == other, 1 if v > other
func (v *Version) Compare(other *Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	// Pre-release versions are considered less than release versions
	if v.Pre != emptyValue && other.Pre == emptyValue {
		return -1
	}
	if v.Pre == emptyValue && other.Pre != emptyValue {
		return 1
	}
	if v.Pre != other.Pre {
		return strings.Compare(v.Pre, other.Pre)
	}
	return 0
}

// LessThan returns true if v < other
func (v *Version) LessThan(other *Version) bool {
	return v.Compare(other) < 0
}

// GreaterThan returns true if v > other
func (v *Version) GreaterThan(other *Version) bool {
	return v.Compare(other) > 0
}

// Equal returns true if v == other
func (v *Version) Equal(other *Version) bool {
	return v.Compare(other) == 0
}

// VersionConstraint represents a version requirement
type VersionConstraint struct {
	MinVersion      *Version   // Minimum required version
	MaxVersion      *Version   // Maximum allowed version (optional)
	ExactVersions   []*Version // Exact versions allowed (optional)
	ExcludeVersions []*Version // Versions to exclude (optional)
}

// ParseVersionConstraint parses a version constraint string
// Supports: ">=1.0.0", "<=2.0.0", "1.0.0", ">=1.0.0 <2.0.0", "^1.0.0" (compatible)
func ParseVersionConstraint(constraintStr string) (*VersionConstraint, error) {
	vc := &VersionConstraint{}

	// Handle caret (^) for compatible versions: ^1.2.3 means >=1.2.3 <2.0.0
	if strings.HasPrefix(constraintStr, "^") {
		versionStr := strings.TrimPrefix(constraintStr, "^")
		version, err := ParseVersion(versionStr)
		if err != nil {
			return nil, err
		}
		vc.MinVersion = version
		// Max version is next major version
		vc.MaxVersion = &Version{
			Major: version.Major + 1,
			Minor: 0,
			Patch: 0,
		}
		return vc, nil
	}

	// Handle >=, <=, >, <, =
	parts := strings.Fields(constraintStr)
	for _, part := range parts {
		if strings.HasPrefix(part, ">=") {
			versionStr := strings.TrimPrefix(part, ">=")
			version, err := ParseVersion(versionStr)
			if err != nil {
				return nil, err
			}
			vc.MinVersion = version
		} else if strings.HasPrefix(part, "<=") {
			versionStr := strings.TrimPrefix(part, "<=")
			version, err := ParseVersion(versionStr)
			if err != nil {
				return nil, err
			}
			vc.MaxVersion = version
		} else if strings.HasPrefix(part, ">") {
			versionStr := strings.TrimPrefix(part, ">")
			version, err := ParseVersion(versionStr)
			if err != nil {
				return nil, err
			}
			// > means >= next patch version
			version.Patch++
			vc.MinVersion = version
		} else if strings.HasPrefix(part, "<") {
			versionStr := strings.TrimPrefix(part, "<")
			version, err := ParseVersion(versionStr)
			if err != nil {
				return nil, err
			}
			vc.MaxVersion = version
		} else if strings.HasPrefix(part, "=") {
			versionStr := strings.TrimPrefix(part, "=")
			version, err := ParseVersion(versionStr)
			if err != nil {
				return nil, err
			}
			vc.ExactVersions = append(vc.ExactVersions, version)
		} else {
			// Assume it's a version string
			version, err := ParseVersion(part)
			if err != nil {
				return nil, err
			}
			vc.ExactVersions = append(vc.ExactVersions, version)
		}
	}

	return vc, nil
}

// Matches checks if a version matches the constraint
func (vc *VersionConstraint) Matches(version *Version) bool {
	// Check exact versions first
	if len(vc.ExactVersions) > 0 {
		for _, exact := range vc.ExactVersions {
			if version.Equal(exact) {
				// Check if excluded
				for _, excluded := range vc.ExcludeVersions {
					if version.Equal(excluded) {
						return false
					}
				}
				return true
			}
		}
		return false
	}

	// Check minimum version
	if vc.MinVersion != nil && version.LessThan(vc.MinVersion) {
		return false
	}

	// Check maximum version (exclusive for <, inclusive for <=)
	// Note: We treat MaxVersion as exclusive (from < constraint)
	// If we need inclusive, we'd need to track the constraint type
	if vc.MaxVersion != nil && !version.LessThan(vc.MaxVersion) {
		return false
	}

	// Check excluded versions
	for _, excluded := range vc.ExcludeVersions {
		if version.Equal(excluded) {
			return false
		}
	}

	return true
}

// VerifyVersionCompatibility verifies if a binary version is compatible
func VerifyVersionCompatibility(binaryVersion string, constraint *VersionConstraint) error {
	version, err := ParseVersion(binaryVersion)
	if err != nil {
		return errfmt.Newf("failed to parse binary version").Wrap(err)
	}

	if !constraint.Matches(version) {
		return errfmt.Errorf("version %s does not match constraint", version.String())
	}

	return nil
}
