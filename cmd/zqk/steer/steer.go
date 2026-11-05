package steer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/events"
	"github.com/zqk-os/zqk/pkg/paths"
	steerpkg "github.com/zqk-os/zqk/pkg/steer"
	"github.com/zqk-os/zqk/pkg/vector"
)

// NewSteerCmd creates the 'zqk steer' command group.
func NewSteerCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSteerCommandBuilder(), &cobra.Command{
		RunE: runSteer,
	})

	cmd.AddCommand(NewResetCmd())
	return cmd
}

// NewResetCmd creates the 'zqk steer reset' subcommand.
func NewResetCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSteerResetCommandBuilder(), &cobra.Command{
		RunE: runSteerReset,
	})
}

func runSteer(cmd *cobra.Command, args []string) error {
	projectRoot := paths.ResolveProjectRoot(".")
	bus := events.GetDefaultBus()
	reg := vector.NewRegistry(projectRoot, bus)
	_ = reg.Load()
	metabolizer := steerpkg.NewMetabolizer(projectRoot, reg, bus)

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	format, _ := cmd.Flags().GetString("format")

	if len(args) == 0 {
		st, err := metabolizer.LoadState()
		if err != nil || st == nil || len(st.ActiveOffsets) == 0 {
			cmd.Println("No active steering offsets currently applied.")
			cmd.Printf("Usage: %s\n", paths.CLIUsage("steer", "<natural language intent>"))
			return nil
		}

		if format == "json" {
			data, err := json.MarshalIndent(st, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		cmd.Println("Active Steering State:")
		if st.LastIntent != "" {
			cmd.Printf("  Last Intent: %q (applied %s)\n", st.LastIntent, st.LastAppliedAt.Format("2006-01-02 15:04:05 MST"))
		}
		cmd.Println("  Active Offsets:")
		for k, v := range st.ActiveOffsets {
			cmd.Printf("    • %s: %+.2f\n", k, v)
		}
		return nil
	}

	intentText := strings.TrimSpace(args[0])

	if dryRun {
		parsed := steerpkg.ParseIntent(intentText)
		if format == "json" {
			data, err := json.MarshalIndent(parsed, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		cmd.Printf("🔍 [Dry Run] Parsed Steering Intent: %q\n", intentText)
		cmd.Printf("  Intensity:  %.1fx\n", parsed.Intensity)
		cmd.Printf("  Direction:  %s\n", parsed.Direction)
		cmd.Printf("  Dimensions: %v\n", parsed.Dimensions)
		cmd.Printf("  Targets:    %v\n", parsed.TargetVectorIDs)
		return nil
	}

	outcome, err := metabolizer.Metabolize(cmd.Context(), intentText)
	if err != nil {
		return fmt.Errorf("applying steering: %w", err)
	}

	if format == "json" {
		data, err := json.MarshalIndent(outcome, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	}

	cmd.Printf("🎯 Steering Applied: %q\n", intentText)
	cmd.Printf("  Intensity: %.1fx | Direction: %s\n", outcome.Intent.Intensity, outcome.Intent.Direction)
	if len(outcome.Adjustments) > 0 {
		cmd.Println("  Adjusted Vectors:")
		for _, adj := range outcome.Adjustments {
			cmd.Printf("    • %s (%s): weight %.2f -> %.2f (%+.2f), rank %d -> %d\n",
				adj.VectorID, adj.Dimension, adj.OldWeight, adj.NewWeight, adj.WeightDelta, adj.OldRank, adj.NewRank)
		}
	}
	if len(outcome.SchedulerWeights) > 0 {
		cmd.Println("  Scheduler Queue Weights:")
		for k, v := range outcome.SchedulerWeights {
			cmd.Printf("    • %s: %.2f\n", k, v)
		}
	}
	cmd.Printf("✓ Dispatched event '%s'\n", steerpkg.EventSteeringApplied)
	return nil
}

func runSteerReset(cmd *cobra.Command, args []string) error {
	projectRoot := paths.ResolveProjectRoot(".")
	bus := events.GetDefaultBus()
	reg := vector.NewRegistry(projectRoot, bus)
	metabolizer := steerpkg.NewMetabolizer(projectRoot, reg, bus)

	format, _ := cmd.Flags().GetString("format")

	outcome, err := metabolizer.Reset(cmd.Context())
	if err != nil {
		return fmt.Errorf("resetting steering offsets: %w", err)
	}

	if format == "json" {
		data, err := json.MarshalIndent(outcome, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	}

	cmd.Println("✓ Successfully reset all steering offsets and vector weights to kernel defaults.")
	cmd.Printf("✓ Dispatched event '%s' (reset)\n", steerpkg.EventSteeringApplied)
	return nil
}
