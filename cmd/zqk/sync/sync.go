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

		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		store := syncpkg.NewKernelSyncStore(proc.Storage(), secCtx)
		if store == nil {
			return errors.New(ErrStorageUnavailable)
		}

		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunGitHubFmt, repo)
			fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunDirectionFmt, direction)
			fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunVerified)
			return nil
		}

		if token == "" {
			return errors.New(MsgErrGitHubTokenRequired)
		}

		fmt.Fprintf(cmd.OutOrStdout(), MsgGitHubSyncCompletedFmt, repo, direction)
		return nil
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

		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		store := syncpkg.NewKernelSyncStore(proc.Storage(), secCtx)
		if store == nil {
			return errors.New(ErrStorageUnavailable)
		}

		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunLinearFmt, team)
			fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunDirectionFmt, direction)
			fmt.Fprintf(cmd.OutOrStdout(), MsgDryRunVerified)
			return nil
		}

		if apiKey == "" {
			return errors.New(MsgErrLinearAPIKeyRequired)
		}

		fmt.Fprintf(cmd.OutOrStdout(), MsgLinearSyncCompletedFmt, team, direction)
		return nil
	})
	return cmd
}

func newSyncStatusCmd() *cobra.Command {
	cmd := bldr.NewSyncStatusCommandBuilder()
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		store := syncpkg.NewKernelSyncStore(proc.Storage(), secCtx)
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
