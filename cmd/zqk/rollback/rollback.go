package rollback

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/rollback"
	"github.com/zqk-os/zqk/pkg/storage"
)

const emptyValue = ""

// NewRollbackCmd returns the rollback root command (list, apply, reconstruct, retain).
// Subcommands are built from command specs via generated builders; RunE is bound here.
func NewRollbackCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewRollbackCommandBuilder(), &cobra.Command{
		Use:   "rollback",
		Short: "List and apply rollback points (lifecycle/maintenance snapshots)",
		Long:  "List rollback points, apply a stored point (quick path), reconstruct state at a timestamp (beyond-threshold path), or trim the store (retain).",
	})
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newApplyCmd())
	cmd.AddCommand(newReconstructCmd())
	cmd.AddCommand(newRetainCmd())
	return cmd
}

// newListCmd returns the list subcommand from the generated builder (spec: rollback/list_command.yaml).
func newListCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewRollbackListCommandBuilder()
	cmd.RunE = runList
	return cmd
}

func runList(cmd *cobra.Command, _ []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return errfmt.Errorf("not a ZQK project (no project root found)")
	}
	lastN, _ := cmd.Flags().GetInt("last")
	withinStr, _ := cmd.Flags().GetString("within")
	var within time.Duration
	if withinStr != emptyValue {
		var err error
		within, err = time.ParseDuration(withinStr)
		if err != nil {
			return errfmt.Newf("invalid --within duration").Wrap(err)
		}
	}
	metas, err := rollback.List(projectRoot, lastN, within)
	if err != nil {
		return errfmt.Newf("rollback list").Wrap(err)
	}
	out := map[string]any{"points": metas}
	return cli.FormatOutput(cmd, out)
}

// newApplyCmd returns the apply subcommand from the generated builder (spec: rollback/apply_command.yaml).
func newApplyCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewRollbackApplyCommandBuilder()
	cmd.RunE = runApply
	return cmd
}

func resolveRollbackStorageAndContext(cmd *cobra.Command) (string, storage.ObjectStorageProvider, context.Context, error) {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return "", nil, nil, errfmt.Errorf("not a ZQK project (no project root found)")
	}
	provider, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
	if err != nil {
		return "", nil, nil, errfmt.Newf("storage").Wrap(err)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	return projectRoot, provider, ctx, nil
}

func runApply(cmd *cobra.Command, args []string) error {
	projectRoot, provider, ctx, err := resolveRollbackStorageAndContext(cmd)
	if err != nil {
		return err
	}
	pointID := args[0]
	if err := rollback.Apply(ctx, projectRoot, pointID, provider); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("%v (if pruned, use: zqk rollback reconstruct --scope-id <kind:id:toStatus> --timestamp <RFC3339>)", err)))
		}
		return err
	}
	out := map[string]any{"applied": pointID}
	return cli.FormatOutput(cmd, out)
}

// newReconstructCmd returns the reconstruct subcommand from the generated builder (spec: rollback/reconstruct_command.yaml).
func newReconstructCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewRollbackReconstructCommandBuilder()
	_ = cmd.MarkFlagRequired("scope-id")
	_ = cmd.MarkFlagRequired("timestamp")
	cmd.RunE = runReconstruct
	return cmd
}

func runReconstruct(cmd *cobra.Command, _ []string) error {
	scopeType, _ := cmd.Flags().GetString("scope-type")
	scopeID, _ := cmd.Flags().GetString("scope-id")
	timestampStr, _ := cmd.Flags().GetString("timestamp")
	if scopeID == emptyValue || timestampStr == emptyValue {
		return errfmt.Errorf("--scope-id and --timestamp are required")
	}
	targetTimestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return errfmt.Newf("invalid --timestamp (use RFC3339)").Wrap(err)
	}
	projectRoot, provider, ctx, err := resolveRollbackStorageAndContext(cmd)
	if err != nil {
		return err
	}
	refs := lifecycle.RecomputeRefsFromScope(ctx, scopeType, scopeID, provider)
	if len(refs) == 0 {
		return errfmt.Errorf("no refs for scope %s:%s", scopeType, scopeID)
	}
	logger := GetLoggerFromProfile(cmd)
	if err := rollback.ApplyReconstruct(ctx, projectRoot, targetTimestamp, refs, provider, logger); err != nil {
		return err
	}
	out := map[string]any{
		"reconstructed":             true,
		"scope_type":                scopeType,
		objects.FieldKeyScopeID:     scopeID,
		"timestamp":                 targetTimestamp.Format(time.RFC3339),
		objects.FieldKeyObjectCount: len(refs),
	}
	return cli.FormatOutput(cmd, out)
}

// newRetainCmd returns the retain subcommand from the generated builder (spec: rollback/retain_command.yaml).
func newRetainCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewRollbackRetainCommandBuilder()
	cmd.RunE = runRetain
	return cmd
}

func runRetain(cmd *cobra.Command, _ []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return errfmt.Errorf("not a ZQK project (no project root found)")
	}
	if err := rollback.Retain(projectRoot); err != nil {
		return errfmt.Newf("rollback retain").Wrap(err)
	}
	out := map[string]any{"retained": true}
	return cli.FormatOutput(cmd, out)
}

// GetLoggerFromProfile returns a Logger for the command's profile (for use with rollback.ApplyReconstruct).
func GetLoggerFromProfile(cmd *cobra.Command) logging.Logger {
	profile := cli.GetProfile(cmd)
	if profile == emptyValue {
		profile = string(pkgctx.ProfileSystem)
	}
	return logging.GetLoggerFromProfile(profile)
}
