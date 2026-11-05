package vector

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/events"
	"github.com/zqk-os/zqk/pkg/paths"
	steerpkg "github.com/zqk-os/zqk/pkg/steer"
	vectorpkg "github.com/zqk-os/zqk/pkg/vector"
)

// NewVectorCmd creates the 'zqk vector' command suite.
func NewVectorCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewVectorCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVectorList(cmd, args)
		},
	})

	cmd.AddCommand(NewListCmd())
	cmd.AddCommand(NewEvaluateCmd())
	cmd.AddCommand(NewSteerCmd())
	return cmd
}

// NewListCmd creates the 'zqk vector list' command.
func NewListCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewVectorListCommandBuilder(), &cobra.Command{
		RunE: runVectorList,
	})
}

// NewEvaluateCmd creates the 'zqk vector evaluate' command.
func NewEvaluateCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewVectorEvaluateCommandBuilder(), &cobra.Command{
		RunE: runVectorEvaluate,
	})
}

// NewSteerCmd creates the 'zqk vector steer' command.
func NewSteerCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewVectorSteerCommandBuilder(), &cobra.Command{
		RunE: runVectorSteer,
	})
}

func getRegistry() *vectorpkg.Registry {
	projectRoot := paths.ResolveProjectRoot(".")
	bus := events.GetDefaultBus()
	reg := vectorpkg.NewRegistry(projectRoot, bus)
	_ = reg.Load()
	return reg
}

func runVectorList(cmd *cobra.Command, args []string) error {
	reg := getRegistry()
	prioritized, _ := cmd.Flags().GetBool("prioritized")
	format, _ := cmd.Flags().GetString("format")

	vectors := reg.List(prioritized)

	if format == "json" {
		data, err := json.MarshalIndent(vectors, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	}

	cmd.Printf("%-18s %-14s %-6s %-8s %-10s %-8s %s\n", "VECTOR ID", "DIMENSION", "RANK", "WEIGHT", "THRESHOLD", "SCORE", "STATUS")
	cmd.Println(strings.Repeat("─", 82))
	for _, v := range vectors {
		scoreStr := "-"
		if v.CurrentScore > 0 {
			scoreStr = fmt.Sprintf("%.2f", v.CurrentScore)
		}
		cmd.Printf("%-18s %-14s %-6d %-8.2f %-10.2f %-8s %s\n",
			v.ID, v.Dimension, v.PriorityRank, v.Weight, v.Threshold, scoreStr, strings.ToUpper(v.Status))
	}
	return nil
}

func runVectorEvaluate(cmd *cobra.Command, args []string) error {
	reg := getRegistry()
	all, _ := cmd.Flags().GetBool("all")
	format, _ := cmd.Flags().GetString("format")

	results, err := reg.Evaluate(cmd.Context(), all)
	if err != nil {
		return fmt.Errorf("evaluating vectors: %w", err)
	}

	var totalWeight, weightedScoreSum float64
	breachedCount := 0
	for _, res := range results {
		vec, _ := reg.Get(res.VectorID)
		w := vec.Weight
		if w <= 0 {
			w = 0.5
		}
		totalWeight += w
		weightedScoreSum += w * res.Score
		if res.Breached {
			breachedCount++
		}
	}

	overallScore := 0.0
	if totalWeight > 0 {
		overallScore = math.Round((weightedScoreSum/totalWeight)*100) / 100
	}

	if format == "json" {
		out := map[string]any{
			"results":              results,
			"overall_health_score": overallScore,
			"total_evaluated":      len(results),
			"breached_count":       breachedCount,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	}

	cmd.Println("📊 Vector Evaluation Results:")
	cmd.Println(strings.Repeat("─", 65))
	for _, res := range results {
		statusIndicator := "✓ PASS"
		if res.Breached {
			statusIndicator = "✗ BREACH"
		}
		cmd.Printf("  • %-16s (%-12s): %.2f / %.2f  [%s]\n",
			res.VectorID, res.Dimension, res.Score, res.Threshold, statusIndicator)
	}
	cmd.Println(strings.Repeat("─", 65))
	cmd.Printf("Overall Weighted Health Score: %.2f / 1.00 (Breached: %d/%d)\n",
		overallScore, breachedCount, len(results))
	cmd.Printf("✓ Dispatched event '%s' (%d evaluations)\n", vectorpkg.EventVectorEvaluated, len(results))
	if breachedCount > 0 {
		cmd.Printf("⚠️  Dispatched event '%s' (%d breaches)\n", vectorpkg.EventVectorThresholdBreach, breachedCount)
	}
	return nil
}

func runVectorSteer(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("steering directive required. Usage: %s", paths.CLIUsage("vector", "steer", "<natural language or delta>"))
	}

	reg := getRegistry()
	format, _ := cmd.Flags().GetString("format")
	input := strings.TrimSpace(args[0])

	// Check if input is direct delta format (e.g. VEC-SECURITY=+0.1 or VEC-SECURITY:0.95 or security:0.9)
	var delta vectorpkg.VectorSteerDelta
	if strings.Contains(input, "=") || strings.Contains(input, ":") {
		sep := "="
		if !strings.Contains(input, "=") {
			sep = ":"
		}
		parts := strings.SplitN(input, sep, 2)
		target := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])

		targetID := target
		if !strings.HasPrefix(targetID, "VEC-") {
			targetID = "VEC-" + strings.ToUpper(targetID)
		}

		delta.VectorID = targetID
		delta.Reason = fmt.Sprintf("explicit directive: %s", input)

		if strings.HasPrefix(valStr, "+") || strings.HasPrefix(valStr, "-") {
			v, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				return fmt.Errorf("invalid delta value %q: %w", valStr, err)
			}
			delta.WeightDelta = &v
		} else {
			v, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				return fmt.Errorf("invalid weight value %q: %w", valStr, err)
			}
			delta.NewWeight = &v
		}
	} else {
		// Natural language intent parsing
		intent := steerpkg.ParseIntent(input)
		if len(intent.TargetVectorIDs) == 0 {
			return fmt.Errorf("could not identify target vector in: %q", input)
		}
		delta.VectorID = intent.TargetVectorIDs[0]
		delta.Reason = input
		sign := 1.0
		if intent.Direction == "reduce" {
			sign = -1.0
		}
		wDelta := 0.10 * intent.Intensity * sign
		rDelta := int(math.Round(5.0 * intent.Intensity * sign))
		delta.WeightDelta = &wDelta
		delta.RankDelta = &rDelta
	}

	vec, err := reg.Steer(cmd.Context(), delta)
	if err != nil {
		return fmt.Errorf("steering vector: %w", err)
	}

	_ = reg.Save()

	if format == "json" {
		data, err := json.MarshalIndent(vec, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	}

	cmd.Printf("🎯 Vector Steered Successfully: %s\n", vec.ID)
	cmd.Printf("  Dimension:     %s\n", vec.Dimension)
	cmd.Printf("  New Weight:    %.2f\n", vec.Weight)
	cmd.Printf("  Priority Rank: %d\n", vec.PriorityRank)
	cmd.Printf("  Threshold:     %.2f\n", vec.Threshold)
	cmd.Printf("✓ Dispatched event '%s'\n", vectorpkg.EventVectorSteered)
	return nil
}
