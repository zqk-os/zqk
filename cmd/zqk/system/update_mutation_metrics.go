package system

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewUpdateMutationMetricsCmd reports process-local storage update mutation-class counters.
func NewUpdateMutationMetricsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Show storage update mutation-class counters",
		"Reports process-local counts by kind and mutation class (runtime_delta, structural, id_change).",
		"",
		"These counters are in-memory for the current process and are reset on process restart.",
	).
		AddExample("Show counters", "%s system update-mutation-metrics").
		AddExample("Reset counters after reading", "%s system update-mutation-metrics --reset").
		ExcludeCommonFlagsWithout("format")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemUpdateMutationMetricsCommandBuilder(), &cobra.Command{
		Use:   "update-mutation-metrics",
		Short: "Show storage update mutation-class counters",
		RunE:  runUpdateMutationMetrics,
	})
	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)
	cmd.Flags().Bool("reset", false, "Reset counters after output")
	return cmd
}

func runUpdateMutationMetrics(cmd *cobra.Command, _ []string) error {
	snapshot := storage.GetUpdateMutationClassSnapshot()
	reset, _ := cmd.Flags().GetBool("reset") //nolint:errcheck

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		if err := cli.FormatOutput(cmd, snapshot); err != nil {
			return err
		}
	default:
		buf := []byte("Update mutation class counters (process-local)\n")
		if len(snapshot) == 0 {
			buf = append(buf, []byte("  (no updates recorded)\n")...)
		} else {
			kinds := make([]string, 0, len(snapshot))
			for kind := range snapshot {
				kinds = append(kinds, kind)
			}
			sort.Strings(kinds)
			for _, kind := range kinds {
				classes := snapshot[kind]
				total := classes["runtime_delta"] + classes["structural"] + classes["id_change"]
				buf = append(buf, []byte(fmt.Sprintf("  %s: total=%d runtime_delta=%d structural=%d id_change=%d\n",
					kind, total, classes["runtime_delta"], classes["structural"], classes["id_change"]))...)
			}
		}
		if err := cli.WriteOutput(cmd, buf); err != nil {
			return err
		}
	}

	if reset {
		storage.ResetUpdateMutationClassMetrics()
	}
	return nil
}
