package pack

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var (
	// ErrInvalidHolonSemVer is returned when a swarm package version string does not strictly adhere to SemVer 2.0.0.
	ErrInvalidHolonSemVer = errors.New("invalid holon SemVer 2.0.0 version string")

	// strictSemVerRegex strictly matches Semantic Versioning 2.0.0 specification (https://semver.org/).
	// Disallows leading 'v', requires MAJOR.MINOR.PATCH, disallows leading zeroes in numeric identifiers.
	strictSemVerRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
)

// SemVer represents a structured, validated Semantic Version 2.0.0 holon version.
type SemVer struct {
	Raw        string
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
}

// ParseSemVer parses and strictly validates a SemVer 2.0.0 string.
// Rejects malformed versions such as "vFoo3.14", "latest", "1.0", or versions with a leading "v".
func ParseSemVer(v string) (SemVer, error) {
	matches := strictSemVerRegex.FindStringSubmatch(v)
	if matches == nil {
		return SemVer{}, fmt.Errorf("%w: %q (must strictly follow MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD])", ErrInvalidHolonSemVer, v)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return SemVer{}, fmt.Errorf("%w: invalid major version: %s", ErrInvalidHolonSemVer, matches[1])
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return SemVer{}, fmt.Errorf("%w: invalid minor version: %s", ErrInvalidHolonSemVer, matches[2])
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return SemVer{}, fmt.Errorf("%w: invalid patch version: %s", ErrInvalidHolonSemVer, matches[3])
	}

	return SemVer{
		Raw:        v,
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Build:      matches[5],
	}, nil
}

// String returns the canonical version string.
func (s SemVer) String() string {
	return s.Raw
}
