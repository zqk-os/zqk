package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionedCommandRegistry(t *testing.T) {
	vcr := NewVersionedCommandRegistry()

	vcr.RegisterVersion("migrate", CommandVersion{
		Version:     "1.0.0",
		Description: "Legacy YAML migrate",
		Deprecated:  true,
		ReplacedBy:  "2.0.0",
	})
	vcr.RegisterVersion("migrate", CommandVersion{
		Version:     "2.0.0",
		Description: "CAS streaming migrate",
	})

	// Default fallback to non-deprecated latest
	def := vcr.GetDefaultVersion("migrate")
	if def != "2.0.0" {
		t.Errorf("expected 2.0.0 as default, got %s", def)
	}

	// Explicit default version
	vcr.SetDefaultVersion("migrate", "1.0.0")
	if vcr.GetDefaultVersion("migrate") != "1.0.0" {
		t.Errorf("expected 1.0.0 after explicit set")
	}

	// Unknown command returns empty
	if vcr.GetDefaultVersion("unknown") != "" {
		t.Errorf("expected empty string for unknown command")
	}
}

func TestCreateVersionedCommand(t *testing.T) {
	var executedVersion string
	versions := []CommandVersion{
		{Version: "1.0.0", Description: "v1 description", Deprecated: true, ReplacedBy: "2.0.0"},
		{Version: "2.0.0", Description: "v2 description"},
	}

	cmd := CreateVersionedCommand("test-ver", "test short", "test long", versions, "2.0.0", func(v string) func(*cobra.Command, []string) error {
		return func(c *cobra.Command, args []string) error {
			executedVersion = v
			return nil
		}
	})

	// Execute default version
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing versioned cmd: %v", err)
	}
	if executedVersion != "2.0.0" {
		t.Errorf("expected executedVersion=2.0.0, got %s", executedVersion)
	}

	// Execute deprecated version
	cmd.SetArgs([]string{"--version", "1.0.0"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing deprecated version: %v", err)
	}
	if executedVersion != "1.0.0" {
		t.Errorf("expected executedVersion=1.0.0, got %s", executedVersion)
	}

	// Execute invalid version
	cmd.SetArgs([]string{"--version", "9.9.9"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "version 9.9.9 not available") {
		t.Errorf("expected version not available error, got: %v", err)
	}
}

func TestNegotiateVersion(t *testing.T) {
	// Preferred version matches
	neg := VersionNegotiation{
		PreferredVersion: "1.5.0",
		MinBinaryVersion: "1.0.0",
		MaxBinaryVersion: "2.0.0",
	}
	chosen, err := NegotiateVersion(neg, []string{"1.0.0", "1.5.0", "2.0.0"})
	if err != nil || chosen != "1.5.0" {
		t.Errorf("expected 1.5.0, got %s (err: %v)", chosen, err)
	}

	// Preferred not available, pick first in range
	neg.PreferredVersion = "3.0.0"
	chosen, err = NegotiateVersion(neg, []string{"0.9.0", "1.2.0", "1.8.0", "2.5.0"})
	if err != nil || chosen != "1.2.0" {
		t.Errorf("expected 1.2.0, got %s (err: %v)", chosen, err)
	}

	// Incompatible version range
	neg.MinBinaryVersion = "5.0.0"
	_, err = NegotiateVersion(neg, []string{"1.0.0", "2.0.0"})
	if err == nil {
		t.Errorf("expected error for incompatible version range")
	}
}
