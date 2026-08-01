package system

import (
	"fmt"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewSnapshotVerifyCmd creates the snapshot-verify command
func NewSnapshotVerifyCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Verify compressed snapshot data integrity against live objects",
		"Verify that a compressed snapshot's expanded data matches the current",
		"live objects in the system. This validates that the snapshot accurately",
		"captured the system state.",
	).
		AddExample("Verify snapshot against live objects", "%s system snapshot-verify test-scenarios/my-snapshot/snapshot.csnap").
		AddExample("Verify with detailed comparison", "%s system snapshot-verify snapshot.csnap --verbose").
		AddExample("Verify specific fields only", "%s system snapshot-verify snapshot.csnap --fields id,kind,status").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSnapshotVerifyCommandBuilder(), &cobra.Command{
		Use:  "snapshot-verify [flags] <csnap-file>",
		Args: cobra.ExactArgs(1),
		RunE: runSnapshotVerify,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringSlice("fields", []string{}, "Specific fields to compare (default: all fields)")
	cmd.Flags().Int("max-objects", 10, "Maximum number of objects to verify (0 = all)")
	cmd.Flags().Bool("strict", false, "Fail on any mismatch (default: report all mismatches)")

	return cmd
}

// runSnapshotVerify executes the snapshot-verify command
func runSnapshotVerify(cmd *cobra.Command, args []string) error {
	csnapFile := args[0]
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	out := cli.CommandOutputWriter(cmd, nil)
	fields, _ := cmd.Flags().GetStringSlice("fields")
	maxObjects, _ := cmd.Flags().GetInt("max-objects")
	strict, _ := cmd.Flags().GetBool("strict")

	// Read and expand snapshot
	_, _ = fmt.Fprintf(out, "Reading compressed snapshot: %s\n", csnapFile)
	expanded, err := readAndExpandSnapshot(csnapFile)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Expanding snapshot (%d objects)...\n", len(expanded))

	// Setup storage
	storageProvider, ctx, err := setupStorageForVerification(cmd)
	if err != nil {
		return err
	}

	// Verify objects
	_, _ = fmt.Fprintf(out, "\nVerifying objects against live system...\n")
	stats := VerificationStats{}
	verifyCount := calculateVerifyCount(len(expanded), maxObjects)
	if maxObjects > 0 && maxObjects < len(expanded) {
		_, _ = fmt.Fprintf(out, "(Verifying first %d of %d objects)\n", verifyCount, len(expanded))
	}

	for i := 0; i < verifyCount; i++ {
		expandedObj := expanded[i]
		objID, ok := expandedObj[objects.FieldKeyID].(string)
		if !ok {
			logging.Fluent(logger).Warn("Expanded object missing ID").Index(i).Log()
			stats.Mismatches++
			continue
		}

		// Verify single object
		match, diff, err := verifySingleObject(ctx, storageProvider, expandedObj, objID, fields, strict, logger)
		if err != nil {
			return err
		}

		if diff == nil {
			// Object not found
			_, _ = fmt.Fprintf(out, "  ❌ %s: Not found in live system\n", objID)
			stats.NotFound++
			continue
		}

		// Output result
		outputVerificationResult(out, objID, match, diff, i, fields)

		if match {
			stats.Verified++
		} else {
			stats.Mismatches++
			if strict {
				return errfmt.Errorf("mismatch detected for object %s", objID)
			}
		}
	}

	// Output summary
	return outputVerificationSummary(out, stats, verifyCount, strict)
}

// compareObjects compares two objects and returns match status and differences
func compareObjects(expanded, live map[string]any, fields []string) (bool, map[string]string) {
	var diffs map[string]string

	// If specific fields requested, only compare those
	if len(fields) > 0 {
		diffs = compareSpecificFields(expanded, live, fields)
	} else {
		diffs = compareAllFields(expanded, live)
	}

	return len(diffs) == 0, diffs
}
