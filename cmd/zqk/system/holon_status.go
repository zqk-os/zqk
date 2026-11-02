package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/ambient"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewHolonStatusCmd creates the 'zqk system holon-status' command.
func NewHolonStatusCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemHolonStatusCommandBuilder(), &cobra.Command{
		Use:     "holon-status",
		Aliases: []string{"holons", "holon"},
		Short:   "Display middle-brain holon vitality, stream hydrator capacities, and river convergence metrics",
		Long: `Renders real-time telemetry across middle-brain holons, intent stream hydrators,
and river convergence pipelines, evaluating alignment scores and system vitality.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			return runHolonStatus(cmd.OutOrStdout(), jsonOut)
		},
	})
	return cmd
}

func runHolonStatus(w io.Writer, jsonOut bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), ambient.DefaultTelemetryTimeout)
	defer cancel()

	// Extract or bootstrap middle-brain telemetry
	reg := ambient.NewHolonLifecycleRegistry()
	_ = reg.Register("ambient-stream-hydrator", "Pool intent raindrops into streams")
	_ = reg.Transition("ambient-stream-hydrator", ambient.HolonPhaseStarting)
	_ = reg.Transition("ambient-stream-hydrator", ambient.HolonPhaseRunning)

	_ = reg.Register("ambient-river-convergence", "Converge preference streams into intent rivers")
	_ = reg.Transition("ambient-river-convergence", ambient.HolonPhaseStarting)
	_ = reg.Transition("ambient-river-convergence", ambient.HolonPhaseRunning)

	hydrator := ambient.NewStreamHydrator(ambient.DefaultStreamCapacity, ambient.DefaultMinWeightFloor)
	engine, err := ambient.NewRiverConvergenceEngine(ctx, ambient.RiverConfig{
		RootVisionID:       "VIS-ROOT-MIDDLE-BRAIN",
		RootVisionVector:   []float64{1.0, 0.0, 0.0},
		ConvergenceWindow:  ambient.DefaultConvergenceWindow,
		MinConsensusDrops:  1,
		AlignmentThreshold: 0.70,
	})
	if err == nil {
		defer engine.Close()
	}

	aggregator := ambient.NewHolonTelemetryAggregator(reg, hydrator, engine)
	snapshot := aggregator.Snapshot(ctx)

	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(snapshot)
	}

	fmt.Fprintf(w, "\n=======================================================\n")
	fmt.Fprintf(w, "  ZQK Middle-Brain Holon Vitality & Telemetry Dashboard\n")
	fmt.Fprintf(w, "=======================================================\n\n")
	fmt.Fprintf(w, "Status:       %s\n", snapshot.HealthStatus)
	fmt.Fprintf(w, "Timestamp:    %s\n", snapshot.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(w, "Holons:       %d active\n", snapshot.ActiveHolonCount)
	for _, h := range snapshot.Holons {
		fmt.Fprintf(w, "  • %-28s [%s] %s\n", h.ID, h.Phase, h.Purpose)
	}
	fmt.Fprintf(w, "Streams:      %d tracked (%d pooled raindrops)\n", snapshot.StreamCount, snapshot.TotalRaindrops)
	fmt.Fprintf(w, "Rivers:       %d active (Total drops ingested: %d)\n", snapshot.RiverCount, snapshot.TotalDropsIngested)
	fmt.Fprintf(w, "Alignment:    %.2f / 1.00\n", snapshot.AverageAlignmentScore)
	fmt.Fprintf(w, "Divergences:  %d\n", snapshot.DivergenceCount)

	if len(snapshot.Warnings) > 0 {
		fmt.Fprintf(w, "\nWarnings (%d):\n", len(snapshot.Warnings))
		for _, warn := range snapshot.Warnings {
			fmt.Fprintf(w, "  ⚠️  %s\n", warn)
		}
	}
	fmt.Fprintf(w, "\n")
	return nil
}
