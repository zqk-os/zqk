package pack

import (
	"errors"
	"testing"
)

func TestParseSemVer_Valid(t *testing.T) {
	tests := []struct {
		input      string
		major      int
		minor      int
		patch      int
		prerelease string
		build      string
	}{
		{"0.0.1", 0, 0, 1, "", ""},
		{"1.0.0", 1, 0, 0, "", ""},
		{"0.1.0", 0, 1, 0, "", ""},
		{"2.11.4", 2, 11, 4, "", ""},
		{"1.0.0-alpha", 1, 0, 0, "alpha", ""},
		{"1.0.0-alpha.1", 1, 0, 0, "alpha.1", ""},
		{"1.0.0-0.3.7", 1, 0, 0, "0.3.7", ""},
		{"1.0.0-x.7.z.92", 1, 0, 0, "x.7.z.92", ""},
		{"1.0.0+20130313144700", 1, 0, 0, "", "20130313144700"},
		{"1.0.0-beta+exp.sha.5114f85", 1, 0, 0, "beta", "exp.sha.5114f85"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			sv, err := ParseSemVer(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if sv.Major != tc.major || sv.Minor != tc.minor || sv.Patch != tc.patch {
				t.Errorf("expected %d.%d.%d, got %d.%d.%d", tc.major, tc.minor, tc.patch, sv.Major, sv.Minor, sv.Patch)
			}
			if sv.Prerelease != tc.prerelease {
				t.Errorf("expected prerelease %q, got %q", tc.prerelease, sv.Prerelease)
			}
			if sv.Build != tc.build {
				t.Errorf("expected build %q, got %q", tc.build, sv.Build)
			}
			if sv.String() != tc.input {
				t.Errorf("expected String() == %q, got %q", tc.input, sv.String())
			}
		})
	}
}

func TestParseSemVer_InvalidFailClosed(t *testing.T) {
	invalidVersions := []string{
		"vFoo3.14",
		"v1.0.0", // Leading 'v' prohibited by strict SemVer 2.0.0
		"latest",
		"1",
		"1.0",
		"1.0.0.0",
		"01.1.1", // Leading zero on numeric component prohibited
		"1.01.1",
		"1.1.01",
		"1.0.0-01", // Leading zero on numeric prerelease segment prohibited
		"1.0.0+invalid character",
		"",
		"   ",
		"1.0.0-alpha..1",
		"beta-1.0.0",
	}

	for _, iv := range invalidVersions {
		t.Run(iv, func(t *testing.T) {
			_, err := ParseSemVer(iv)
			if err == nil {
				t.Fatalf("expected error for invalid SemVer %q, got nil", iv)
			}
			if !errors.Is(err, ErrInvalidHolonSemVer) {
				t.Errorf("expected ErrInvalidHolonSemVer, got %v", err)
			}
		})
	}
}
