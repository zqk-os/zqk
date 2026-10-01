package internal

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// uniqueKindsFromObjectIDs returns distinct canonical kinds inferred from object IDs.
// BulkResult rows for update/delete often only include id; CAS flush needs kinds (see object bulk paths).
func uniqueKindsFromObjectIDs(ids []string) []string {
	seen := make(map[string]bool)
	for _, id := range ids {
		if id == emptyValue {
			continue
		}
		k := objects.GetCanonicalKind(id)
		if k != emptyValue && k != id {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}

// BulkDeleteFlags contains parsed bulk delete command flags
type BulkDeleteFlags struct {
	IDs              []string
	Cascade          bool
	UnlinkReferences bool
	DryRun           bool
}

// parseBulkDeleteFlags parses all bulk delete command flags
func parseBulkDeleteFlags(cmd *cobra.Command, proc *cli.Processor) (*BulkDeleteFlags, error) {
	flags := &BulkDeleteFlags{}

	// Get IDs from --ids or --file using shared utility
	ids, err := clipkg.LoadIDsFromFlags(cmd, proc.Logger())
	if err != nil {
		return nil, err
	}

	flags.IDs = ids

	var flagsBag clipkg.FlagBag
	flags.Cascade = flagsBag.Bool(cmd, "cascade")
	flags.UnlinkReferences = flagsBag.Bool(cmd, "unlink-references")
	flags.DryRun = flagsBag.Bool(cmd, "dry-run")
	if err := flagsBag.Err(); err != nil {
		return nil, err
	}

	if flags.UnlinkReferences && flags.Cascade {
		return nil, errfmt.Errorf("--unlink-references cannot be combined with --cascade")
	}
	// TRACK: fail-closed bulk delete (parity with object delete).
	if !flags.UnlinkReferences && !flags.Cascade {
		return nil, errfmt.Errorf("bulk delete refused: pass --unlink-references or --cascade; refusing to leave GhostRefs")
	}

	return flags, nil
}

// handleDryRunDelete handles dry-run mode for bulk delete
func handleDryRunDelete(cmd *cobra.Command, proc *cli.Processor, ids []string, cascade, unlinkRefs bool) error {
	logging.FluentEvent(proc.Logger()).Info("Dry-run mode: showing what would be deleted").
		Int("count", len(ids)).
		Log()
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Would delete %d internal objects:\n", len(ids))
	for _, id := range ids {
		obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
		if err == nil {
			fmt.Fprintf(&buf, "  - %s", id)
			if kind, ok := obj[objects.FieldKeyKind].(string); ok {
				fmt.Fprintf(&buf, " (kind: %s)", kind)
			}
			if title, ok := obj[objects.FieldKeyTitle].(string); ok {
				fmt.Fprintf(&buf, " - %s", title)
			}
			if storage.IsBuiltIn(obj) {
				buf.WriteString(" [BUILT-IN - WARNING: May break system functionality!]")
			}
			buf.WriteString("\n")
		} else {
			fmt.Fprintf(&buf, "  - %s (not found)\n", id)
		}
	}
	if unlinkRefs {
		buf.WriteString("  Unlink references from dependents before delete: true\n")
	}
	if cascade {
		buf.WriteString("  Cascade: true (would also delete dependents)\n")
	}
	return cli.WriteOutput(cmd, buf.Bytes())
}

// executeBulkDelete executes the bulk delete operation
func executeBulkDelete(proc *cli.Processor, ids []string, cascade, unlinkRefs bool) (*storage.BulkResult, error) {
	// Use processor's WithCLIOperation for authorization
	cliCtx := proc.WithCLIOperation()
	if unlinkRefs {
		cliCtx = storage.WithUnlinkReferencesBeforeDelete(cliCtx)
	}

	result, err := proc.Storage().BulkDelete(cliCtx, proc.SecurityContext(), ids, cascade)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Bulk delete failed", err).Log()
		return nil, errfmt.Newf("bulk delete failed").Wrap(err)
	}

	return result, nil
}
