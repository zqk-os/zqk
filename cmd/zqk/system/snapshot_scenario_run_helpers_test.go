package system

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestValidateSnapshotInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flags   *SnapshotScenarioFlags
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with object ids and policy",
			flags: &SnapshotScenarioFlags{
				ObjectIDs:    []string{"GOAL-001"},
				ChangePolicy: "include",
			},
			wantErr: false,
		},
		{
			name: "valid with kinds and reject policy",
			flags: &SnapshotScenarioFlags{
				Kinds:        []string{"goal"},
				ChangePolicy: "reject",
			},
			wantErr: false,
		},
		{
			name: "valid with reconstruct policy",
			flags: &SnapshotScenarioFlags{
				Kinds:        []string{"goal"},
				ChangePolicy: "reconstruct",
			},
			wantErr: false,
		},
		{
			name: "missing object ids and kinds",
			flags: &SnapshotScenarioFlags{
				ChangePolicy: "include",
			},
			wantErr: true,
			errMsg:  "must specify --object-ids or --kinds",
		},
		{
			name: "invalid change policy",
			flags: &SnapshotScenarioFlags{
				ObjectIDs:    []string{"GOAL-001"},
				ChangePolicy: "invalid_policy",
			},
			wantErr: true,
			errMsg:  "invalid change-policy",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateSnapshotInputs(tc.flags)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errMsg)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveTargetDirectory(t *testing.T) {
	t.Parallel()

	// Explicit target
	explicit := resolveTargetDirectory("custom/snapshot/path")
	if explicit != "custom/snapshot/path" {
		t.Fatalf("expected custom path, got %q", explicit)
	}

	// Empty target should generate timestamped path
	generated := resolveTargetDirectory("")
	if !strings.HasPrefix(generated, "test-scenarios/snapshot-") {
		t.Fatalf("expected generated target prefix 'test-scenarios/snapshot-', got %q", generated)
	}
}

func TestOutputSnapshotConfigurationAndDryRun(t *testing.T) {
	t.Parallel()

	flags := &SnapshotScenarioFlags{
		Target:       "test-target",
		ObjectIDs:    []string{"OBJ-1", "OBJ-2"},
		Kinds:        []string{"goal", "epic"},
		ChangePolicy: "include",
		Adaptors:     []string{"anonymize"},
		DryRun:       true,
	}

	var buf bytes.Buffer
	outputSnapshotConfiguration(flags, &buf)
	configOutput := buf.String()
	if !strings.Contains(configOutput, "Snapshot Configuration:") ||
		!strings.Contains(configOutput, "Target: test-target") ||
		!strings.Contains(configOutput, "OBJ-1") {
		t.Fatalf("unexpected configuration output:\n%s", configOutput)
	}

	buf.Reset()
	handleDryRun(flags, &buf)
	dryRunOutput := buf.String()
	if !strings.Contains(dryRunOutput, "Dry run: Would capture snapshot:") ||
		!strings.Contains(dryRunOutput, "Objects: 2 specified") ||
		!strings.Contains(dryRunOutput, "Kinds: [goal epic]") {
		t.Fatalf("unexpected dry run output:\n%s", dryRunOutput)
	}
}

func TestOutputMetadataDetails(t *testing.T) {
	t.Parallel()

	metadata := &storage.SnapshotMetadata{
		ObjectIDs: []string{"OBJ-1"},
		Hashes: map[string]string{
			"OBJ-1": "abcdef1234567890extra",
		},
		HashSources: map[string]string{
			"OBJ-1": "registry",
		},
		ModificationTimes: map[string]time.Time{
			"OBJ-1": time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC),
		},
	}

	// Non-verbose: should output nothing
	var quietBuf bytes.Buffer
	outputMetadataDetails(metadata, false, &quietBuf)
	if quietBuf.Len() > 0 {
		t.Fatalf("expected no output when verbose=false, got %q", quietBuf.String())
	}

	// Verbose: should output details
	var verboseBuf bytes.Buffer
	outputMetadataDetails(metadata, true, &verboseBuf)
	out := verboseBuf.String()
	if !strings.Contains(out, "Captured metadata for 1 objects") ||
		!strings.Contains(out, "OBJ-1") ||
		!strings.Contains(out, "from registry") {
		t.Fatalf("unexpected verbose metadata output:\n%s", out)
	}
}

func TestBuildAdaptorConfig(t *testing.T) {
	t.Parallel()

	flags := &SnapshotScenarioFlags{
		IDPrefix:  "MOCK-",
		Namespace: "custom-ns",
		FieldOverrides: map[string]string{
			"status": "draft",
		},
		ExcludePII:    true,
		PreserveState: true,
	}

	cfg := buildAdaptorConfig(flags)
	if cfg.IDPrefix != "MOCK-" || cfg.Namespace != "custom-ns" || !cfg.ExcludePII || !cfg.PreserveState {
		t.Fatalf("unexpected adaptor config: %+v", cfg)
	}
	if cfg.FieldOverrides["status"] != "draft" {
		t.Fatalf("expected status override 'draft', got %v", cfg.FieldOverrides["status"])
	}
}

func TestOutputSnapshotSuccess(t *testing.T) {
	t.Parallel()

	scenarioObj := map[string]any{
		objects.FieldKeyID: "SCN-12345",
	}
	var buf bytes.Buffer
	outputSnapshotSuccess(
		scenarioObj,
		"test/target/dir",
		[]map[string]any{{"id": "OBJ-1"}},
		time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC),
		"test/target/dir/snapshot.csnap",
		&buf,
	)
	out := buf.String()
	if !strings.Contains(out, "Snapshot captured successfully") ||
		!strings.Contains(out, "Scenario ID: SCN-12345") ||
		!strings.Contains(out, "Compressed: test/target/dir/snapshot.csnap") {
		t.Fatalf("unexpected success output:\n%s", out)
	}
}

func TestParseSnapshotScenarioFlags(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "snapshot"}
	cmd.Flags().String("target", "", "")
	cmd.Flags().StringSlice("object-ids", nil, "")
	cmd.Flags().StringSlice("kinds", nil, "")
	cmd.Flags().String("change-policy", "reject", "")
	cmd.Flags().StringSlice("adaptors", nil, "")
	cmd.Flags().String("query-preset", "", "")
	cmd.Flags().Bool("compress", false, "")
	cmd.Flags().GetString("compressed-output")
	cmd.Flags().String("compressed-output", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().String("id-prefix", "", "")
	cmd.Flags().String(objects.KindNamespace, "", "")
	cmd.Flags().StringToString("field-override", nil, "")
	cmd.Flags().Bool("exclude-pii", false, "")
	cmd.Flags().Bool("preserve-state", false, "")

	if err := cmd.ParseFlags([]string{
		"--target", "my-target",
		"--object-ids", "OBJ-A,OBJ-B",
		"--kinds", "goal",
		"--change-policy", "include",
		"--compress",
		"--dry-run",
		"--verbose",
		"--id-prefix", "TST-",
		"--exclude-pii",
		"--preserve-state",
	}); err != nil {
		t.Fatal(err)
	}

	flags := parseSnapshotScenarioFlags(cmd)
	if flags.Target != "my-target" ||
		len(flags.ObjectIDs) != 2 ||
		flags.ChangePolicy != "include" ||
		!flags.Compress ||
		!flags.DryRun ||
		!flags.Verbose ||
		flags.IDPrefix != "TST-" ||
		!flags.ExcludePII ||
		!flags.PreserveState {
		t.Fatalf("unexpected parsed flags: %+v", flags)
	}
}
