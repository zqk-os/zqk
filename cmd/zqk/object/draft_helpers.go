package object

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/storage"
)

type draftPlaneCLIFlags struct {
	dryRun    bool
	matchOpts storage.ObjectDraftPlaneMatchOptions
}

func parseDraftPlaneCLIFlags(cmd *cobra.Command) (draftPlaneCLIFlags, error) {
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return draftPlaneCLIFlags{}, err
	}
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return draftPlaneCLIFlags{}, err
	}
	kind, err := cmd.Flags().GetString("kind")
	if err != nil {
		return draftPlaneCLIFlags{}, err
	}
	idPrefix, err := cmd.Flags().GetString("id-prefix")
	if err != nil {
		return draftPlaneCLIFlags{}, err
	}
	status, err := cmd.Flags().GetString("status")
	if err != nil {
		return draftPlaneCLIFlags{}, err
	}
	olderThanStr, err := cmd.Flags().GetString("older-than")
	if err != nil {
		return draftPlaneCLIFlags{}, err
	}
	maxN, err := cmd.Flags().GetInt("max")
	if err != nil {
		return draftPlaneCLIFlags{}, err
	}

	var olderThan time.Duration
	if olderThanStr != "" {
		olderThan, err = time.ParseDuration(olderThanStr)
		if err != nil {
			return draftPlaneCLIFlags{}, fmt.Errorf("invalid --older-than %q: %w", olderThanStr, err)
		}
	}

	return draftPlaneCLIFlags{
		dryRun: dryRun,
		matchOpts: storage.ObjectDraftPlaneMatchOptions{
			Kind:      kind,
			IDPrefix:  idPrefix,
			Status:    status,
			OlderThan: olderThan,
			All:       all,
			Max:       maxN,
		},
	}, nil
}
