package healthchk

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/healthcheck"
	_ "github.com/zqk-os/zqk/pkg/healthcheck/monitors" // register built-in monitors
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

const (
	emptyValue            = ""
	healthCheckStatusSkip = "skip"
)

// NewHealthchkCmd returns the healthchk root command (list, update, run, bulk).
func NewHealthchkCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewHealthchkCommandBuilder(), &cobra.Command{
		Use:   "healthchk",
		Short: "Health check registry (list, enable/disable, run monitors)",
		Long:  "List and manage pluggable health monitors. Monitors run on a schedule or on demand and can trigger alerts. Use list/update/run/bulk like object/internal.",
	})
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newBulkCmd())
	return cmd
}

func newListCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewHealthchkListCommandBuilder(), &cobra.Command{
		Use:   "list",
		Short: "List registered health monitors and their config",
		RunE:  runList,
	})
	clipkg.AddListFlags(cmd)
	return cmd
}

// healthchkListSource implements clipkg.ListSource for the health monitor registry.
type healthchkListSource struct {
	projectRoot string
}

func (s healthchkListSource) List(ctx context.Context, opts clipkg.ListOptions) ([]map[string]any, int, error) {
	list := healthcheck.DefaultRegistry.List()
	items := make([]map[string]any, 0, len(list))
	for _, m := range list {
		enabled := healthcheck.DefaultRegistry.IsEnabled(m.ID())
		items = append(items, map[string]any{
			objects.FieldKeyID:      m.ID(),
			objects.FieldKeyName:    m.Name(),
			objects.FieldKeyEnabled: enabled,
		})
	}
	return items, len(items), nil
}

func resolveHealthcheckProjectRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return emptyValue, errfmt.Errorf("failed to get context")
	}
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == emptyValue {
		return emptyValue, errfmt.Errorf("project root not found")
	}
	return projectRoot, nil
}

func resolveBoundRegistry(cmd *cobra.Command) (string, *healthcheck.DefaultRegistryImpl, error) {
	projectRoot, err := resolveHealthcheckProjectRoot(cmd)
	if err != nil {
		return emptyValue, nil, err
	}
	reg, ok := healthcheck.DefaultRegistry.(*healthcheck.DefaultRegistryImpl)
	if !ok {
		return emptyValue, nil, errfmt.Errorf("registry does not support config persistence")
	}
	if err := reg.BindProjectRoot(projectRoot); err != nil {
		return emptyValue, nil, err
	}
	return projectRoot, reg, nil
}

func runList(cmd *cobra.Command, args []string) error {
	projectRoot, _ := resolveHealthcheckProjectRoot(cmd)
	reg, ok := healthcheck.DefaultRegistry.(*healthcheck.DefaultRegistryImpl)
	if ok && projectRoot != emptyValue {
		_ = reg.BindProjectRoot(projectRoot)
	}
	source := healthchkListSource{projectRoot: projectRoot}
	traitRegistry := objects.NewTraitRegistry()
	cfg := &clipkg.ListConfig{
		TraitGroups:   []string{"read_only_group"},
		TraitExpander: traitRegistry,
		TableColumns:  []string{"id", "name", "enabled"},
	}
	runCtx := cli.CommandContextOr(cmd, nil)
	return clipkg.Run(runCtx, cmd, source, cfg, logging.GetCommandOutputWriter(runCtx))
}

func newUpdateCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewHealthchkUpdateCommandBuilder(), &cobra.Command{
		Use:   "update <id>",
		Short: "Enable or disable a monitor",
		Args:  cobra.ExactArgs(1),
		RunE:  runUpdate,
	})
	cmd.Flags().Bool("enabled", true, "Set enabled (true/false)")
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	_, reg, err := resolveBoundRegistry(cmd)
	if err != nil {
		return err
	}
	id := args[0]
	enabled, _ := cmd.Flags().GetBool("enabled")
	if err := reg.SetEnabled(id, enabled); err != nil {
		if fileutil.IsNotExist(err) {
			return errfmt.Errorf("monitor %q not found", id)
		}
		return errfmt.Newf("update monitor").Wrap(err)
	}
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("Monitor %s enabled=%v\n", id, enabled)))
}

func newRunCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewHealthchkRunCommandBuilder(), &cobra.Command{
		Use:   "run [id]",
		Short: "Run one monitor by id or all enabled monitors",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runRun,
	})
	cmd.Flags().String("format", "table", "Output format: table | json")
	return cmd
}

func runRun(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveHealthcheckProjectRoot(cmd)
	if err != nil {
		return err
	}
	reg, ok := healthcheck.DefaultRegistry.(*healthcheck.DefaultRegistryImpl)
	if ok {
		_ = reg.BindProjectRoot(projectRoot)
	}
	id := ""
	if len(args) > 0 {
		id = args[0]
	}
	result, err := healthcheck.DefaultRegistry.Run(cmd.Context(), projectRoot, id)
	if err != nil {
		return err
	}
	// Append change journal entry for single-monitor run (track over time; aggregate/compact via micro-GC)
	if id != emptyValue && result != nil && result.Status != healthCheckStatusSkip {
		_ = storage.CreateHealthCheckChangeJournalEntry(cmd.Context(), projectRoot, nil, id, result.Status, result.Summary, result.Details)
	}
	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		// Subcommand uses local --format (not global --format); use explicit JSON handler + WriteOutput routing.
		if err := cli.FormatOutputAs(cmd, cli.FormatJSON, result); err != nil {
			return err
		}
	} else {
		if err := cli.WriteOutput(cmd, []byte(fmt.Sprintf("status=%s summary=%s\n", result.Status, result.Summary))); err != nil {
			return err
		}
	}
	if result != nil && result.Status == "fail" {
		return errfmt.Errorf("health check failed: %s", result.Summary)
	}
	return nil
}

func newBulkCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewHealthchkBulkCommandBuilder(), &cobra.Command{
		Use: "bulk",
	})
	cmd.AddCommand(newBulkDisableCmd())
	return cmd
}

func newBulkDisableCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewHealthchkBulkDisableCommandBuilder(), &cobra.Command{
		Use:   "disable",
		Short: "Disable multiple monitors by ID (--ids or --ids-file)",
		RunE:  runBulkDisable,
	})
	cmd.Flags().String("ids-file", "", "Path to file containing monitor IDs (one per line or YAML list)")
	cmd.Flags().String("ids", "", "Comma-separated monitor IDs")
	return cmd
}

func runBulkDisable(cmd *cobra.Command, args []string) error {
	_, reg, err := resolveBoundRegistry(cmd)
	if err != nil {
		return err
	}
	idsStr, _ := cmd.Flags().GetString("ids")
	idsFile, _ := cmd.Flags().GetString("ids-file")
	var ids []string
	if idsStr != emptyValue {
		for _, s := range splitIDs(idsStr) {
			if s != emptyValue {
				ids = append(ids, s)
			}
		}
	}
	if idsFile != emptyValue {
		fromFile, err := readIDsFromFile(idsFile)
		if err != nil {
			return errfmt.Newf("read ids-file").Wrap(err)
		}
		ids = append(ids, fromFile...)
	}
	if len(ids) == 0 {
		return errfmt.Errorf("provide --ids or --ids-file")
	}
	for _, id := range ids {
		_ = reg.SetEnabled(id, false)
	}
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("Disabled %d monitor(s)\n", len(ids))))
}

func splitIDs(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func readIDsFromFile(path string) ([]string, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Try YAML array first (e.g. - scheduler_events\n- other)
	var yamlList []string
	if err := yaml.Unmarshal(data, &yamlList); err == nil && len(yamlList) > 0 {
		return yamlList, nil
	}
	// Plain list: one id per line (or "- id" YAML-style)
	var ids []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if when.IsEmpty(line) {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			line = strings.TrimSpace(line[2:])
		}
		if line != emptyValue {
			ids = append(ids, line)
		}
	}
	return ids, nil
}
