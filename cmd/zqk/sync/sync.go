package sync

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	syncpkg "github.com/zqk-os/zqk/pkg/adapters/sync"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// NewSyncCmd creates the root 'zqk sync' command for external issue synchronization.
func NewSyncCmd() *cobra.Command {
	cmd := bldr.NewSyncCommandBuilder()
	cmd.AddCommand(newGitHubSyncCmd())
	cmd.AddCommand(newLinearSyncCmd())
	cmd.AddCommand(newSyncStatusCmd())
	return cmd
}

func newGitHubSyncCmd() *cobra.Command {
	cmd := bldr.NewSyncGithubCommandBuilder()
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		repo := flags.String(cmd, FlagRepo)
		dryRun := flags.Bool(cmd, FlagDryRun)
		direction := flags.String(cmd, FlagDirection)
		token := flags.String(cmd, FlagToken)
		if err := flags.Err(); err != nil {
			return err
		}

		if token == "" {
			token = os.Getenv(EnvGitHubToken)
		}

		return executeSyncOperation(cmd, proc, syncExecutionParams{
			target:      repo,
			dryRun:      dryRun,
			direction:   direction,
			credential:  token,
			dryRunFmt:   MsgDryRunGitHubFmt,
			requiredErr: MsgErrGitHubTokenRequired,
			completeFmt: MsgGitHubSyncCompletedFmt,
		})
	})
	return cmd
}

func newLinearSyncCmd() *cobra.Command {
	cmd := bldr.NewSyncLinearCommandBuilder()
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		team := flags.String(cmd, FlagTeam)
		dryRun := flags.Bool(cmd, FlagDryRun)
		direction := flags.String(cmd, FlagDirection)
		apiKey := flags.String(cmd, FlagAPIKey)
		if err := flags.Err(); err != nil {
			return err
		}

		if apiKey == "" {
			apiKey = os.Getenv(EnvLinearAPIKey)
		}

		return executeSyncOperation(cmd, proc, syncExecutionParams{
			target:      team,
			dryRun:      dryRun,
			direction:   direction,
			credential:  apiKey,
			dryRunFmt:   MsgDryRunLinearFmt,
			requiredErr: MsgErrLinearAPIKeyRequired,
			completeFmt: MsgLinearSyncCompletedFmt,
		})
	})
	return cmd
}

func newSyncStatusCmd() *cobra.Command {
	cmd := bldr.NewSyncStatusCommandBuilder()
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		store, err := resolveKernelSyncStore(proc)
		if err != nil {
			return err
		}
		items, err := store.ListBacklogItems(proc.OperationContext())
		if err != nil {
			return err
		}

		ghCount := 0
		linearCount := 0
		for _, it := range items {
			if strings.EqualFold(string(it.ExternalSource), string(syncpkg.SourceGitHub)) || strings.HasPrefix(it.ID, PrefixBliGitHub) {
				ghCount++
			} else if strings.EqualFold(string(it.ExternalSource), string(syncpkg.SourceLinear)) || strings.HasPrefix(it.ID, PrefixBliLinear) {
				linearCount++
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), MsgSyncStatusHeader)
		fmt.Fprintf(cmd.OutOrStdout(), MsgSyncStatusGitHubFmt, ghCount)
		fmt.Fprintf(cmd.OutOrStdout(), MsgSyncStatusLinearFmt, linearCount)
		fmt.Fprintf(cmd.OutOrStdout(), MsgSyncStatusTotalFmt, ghCount+linearCount)
		return nil
	})
	return cmd
}

func resolveKernelSyncStore(proc *cli.Processor) (*syncpkg.KernelSyncStore, error) {
	secCtx := proc.SecurityContext()
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	store := syncpkg.NewKernelSyncStore(proc.Storage(), secCtx)
	if store == nil {
		return nil, errors.New(ErrStorageUnavailable)
	}
	return store, nil
}

func printSyncDryRun(cmd *cobra.Command, targetFmt, target, direction string) {
	fmt.Fprintf(cmd.OutOrStdout(), targetFmt, target)
	fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunDirectionFmt, direction)
	fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunVerified)
}

type syncExecutionParams struct {
	target      string
	dryRun      bool
	direction   string
	credential  string
	dryRunFmt   string
	requiredErr string
	completeFmt string
}

func executeSyncOperation(cmd *cobra.Command, proc *cli.Processor, params syncExecutionParams) error {
	if _, err := resolveKernelSyncStore(proc); err != nil {
		return err
	}
	if params.dryRun {
		printSyncDryRun(cmd, params.dryRunFmt, params.target, params.direction)
		return nil
	}
	if params.credential == "" {
		return errors.New(params.requiredErr)
	}
	fmt.Fprintf(cmd.OutOrStdout(), params.completeFmt, params.target, params.direction)
	return nil
}

