package system

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/snapshot"
)

// VerificationStats tracks verification statistics
type VerificationStats struct {
	Verified   int
	Mismatches int
	NotFound   int
}

// readAndExpandSnapshot reads and expands a compressed snapshot
func readAndExpandSnapshot(csnapFile string) ([]map[string]any, error) {
	return snapshot.ReadAndExpandSnapshot(csnapFile)
}

// setupStorageForVerification sets up storage provider for verification
func setupStorageForVerification(cmd *cobra.Command) (storage.ObjectStorageProvider, context.Context, error) {
	cliCtx := cli.GetContext(cmd)
	projectRoot := cliCtx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return nil, nil, errfmt.Errorf("project root not found")
	}

	ctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	return storageFactory.GetStorage(), ctx, nil
}

// calculateVerifyCount calculates how many objects to verify
func calculateVerifyCount(totalObjects, maxObjects int) int {
	return snapshot.CalculateVerifyCount(totalObjects, maxObjects)
}

// verifySingleObject verifies a single object
func verifySingleObject(ctx context.Context, storageProvider storage.ObjectStorageProvider, expandedObj map[string]any, objID string, fields []string, strict bool, logger logging.Logger) (bool, map[string]string, error) {
	secCtx := pkgctx.NewSystemSecurityContext()

	// Read live object
	liveObj, err := storageProvider.Read(ctx, secCtx, objID)
	if err != nil {
		if strict {
			return false, nil, errfmt.Errorf("object %s not found in live system", objID)
		}
		return false, nil, nil // Not found, but not strict mode
	}

	// Compare objects
	match, diff := compareObjects(expandedObj, liveObj, fields)
	return match, diff, nil
}

// outputVerificationResult outputs the result of verifying a single object
func outputVerificationResult(out io.Writer, objID string, match bool, diff map[string]string, index int, fields []string) {
	if match {
		if index < 5 { // Show first few matches
			_, _ = fmt.Fprintf(out, "  ✅ %s: Match\n", objID)
		}
	} else {
		_, _ = fmt.Fprintf(out, "  ❌ %s: Mismatch\n", objID)
		if len(fields) == 0 || len(diff) > 0 {
			for field, details := range diff {
				_, _ = fmt.Fprintf(out, "      %s: %s\n", field, details)
			}
		}
	}
}

// outputVerificationSummary outputs the verification summary
func outputVerificationSummary(out io.Writer, stats VerificationStats, verifyCount int, strict bool) error {
	_, _ = fmt.Fprintf(out, "\nVerification Summary:\n")
	_, _ = fmt.Fprintf(out, "  ✅ Verified: %d\n", stats.Verified)
	_, _ = fmt.Fprintf(out, "  ❌ Mismatches: %d\n", stats.Mismatches)
	_, _ = fmt.Fprintf(out, "  ⚠️  Not Found: %d\n", stats.NotFound)
	_, _ = fmt.Fprintf(out, "  Total Checked: %d\n", verifyCount)

	if stats.Mismatches > 0 || stats.NotFound > 0 {
		if strict {
			return errfmt.Errorf("verification failed: %d mismatches, %d not found", stats.Mismatches, stats.NotFound)
		}
		_, _ = fmt.Fprintf(out, "\n⚠️  Verification completed with issues (use --strict to fail on mismatches)\n")
	} else {
		_, _ = fmt.Fprintf(out, "\n✅ All objects verified successfully!\n")
	}

	return nil
}

// isMetadataField checks if a field is a metadata field that should be skipped
func isMetadataField(key string) bool {
	return key == "created_at" || key == "updated_at" || key == "created_by" || key == "updated_by"
}

// compareSpecificFields compares only the specified fields
func compareSpecificFields(expanded, live map[string]any, fields []string) map[string]string {
	diffs := make(map[string]string)
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f] = true
	}

	for field := range fieldSet {
		expVal := expanded[field]
		liveVal := live[field]

		if !reflect.DeepEqual(expVal, liveVal) {
			diffs[field] = fmt.Sprintf("expanded=%v, live=%v", expVal, liveVal)
		}
	}

	return diffs
}

// compareAllFields compares all fields (excluding metadata)
func compareAllFields(expanded, live map[string]any) map[string]string {
	diffs := make(map[string]string)

	// Compare all fields from expanded object
	for key, expVal := range expanded {
		// Skip metadata fields
		if isMetadataField(key) {
			continue
		}

		liveVal, exists := live[key]
		if !exists {
			diffs[key] = "field missing in live object"
			continue
		}

		if !reflect.DeepEqual(expVal, liveVal) {
			diffs[key] = fmt.Sprintf("expanded=%v, live=%v", expVal, liveVal)
		}
	}

	// Check for fields in live that aren't in expanded (new fields)
	for key := range live {
		if _, exists := expanded[key]; !exists {
			// Skip metadata fields
			if !isMetadataField(key) {
				diffs[key] = "field missing in expanded object"
			}
		}
	}

	return diffs
}
