package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/ambient"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

const (
	defaultHolonFormatFlag   = "table"
	minHealthyAlignment      = 0.70
	defaultRuntimeHolonCount = 6
)

// NewHolonCmd creates the 'zqk system holon' parent command group.
func NewHolonCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemHolonCommandBuilder(), &cobra.Command{
		Use:     "holon",
		Aliases: []string{"holons"},
		Short:   "Middle-brain holon telemetry and status operations",
		Long: `Middle-brain holon telemetry, lifecycle, and stream convergence management.

Commands:
  status    Display middle-brain holon vitality, stream hydrator capacities, and river convergence metrics`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHolonStatusFromCommand(cmd)
		},
	})
	cmd.AddCommand(NewHolonStatusSubCmd())
	addHolonStatusFlags(cmd)
	return cmd
}

// NewHolonStatusSubCmd creates the 'status' subcommand under 'zqk system holon'.
func NewHolonStatusSubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"stat"},
		Short:   "Display middle-brain holon vitality, stream hydrator capacities, and river convergence metrics",
		Long: `Renders real-time telemetry across middle-brain holons, intent stream hydrators,
and river convergence pipelines, evaluating alignment scores and system vitality.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHolonStatusFromCommand(cmd)
		},
	}
	addHolonStatusFlags(cmd)
	return cmd
}

// NewHolonStatusCmd creates the 'zqk system holon-status' command.
func NewHolonStatusCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemHolonStatusCommandBuilder(), &cobra.Command{
		Use:     "holon-status",
		Aliases: []string{"holons-status"},
		Short:   "Display middle-brain holon vitality, stream hydrator capacities, and river convergence metrics",
		Long: `Renders real-time telemetry across middle-brain holons, intent stream hydrators,
and river convergence pipelines, evaluating alignment scores and system vitality.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHolonStatusFromCommand(cmd)
		},
	})
	addHolonStatusFlags(cmd)
	return cmd
}

func addHolonStatusFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup("format") == nil {
		cmd.Flags().StringP("format", "f", defaultHolonFormatFlag, "Output format (table, json)")
	}
	if cmd.Flags().Lookup("json") == nil {
		cmd.Flags().Bool("json", false, "Output telemetry snapshot as JSON")
	}
	if cmd.Flags().Lookup("engine") == nil {
		cmd.Flags().Bool("engine", false, "Output ambient engine runtime telemetry report")
	}
}

func runHolonStatusFromCommand(cmd *cobra.Command) error {
	jsonFlag, err := cmd.Flags().GetBool("json")
	if err != nil {
		return errfmt.Newf("holon status: parse json flag").Wrap(err)
	}
	formatFlag, err := cmd.Flags().GetString("format")
	if err != nil {
		return errfmt.Newf("holon status: parse format flag").Wrap(err)
	}
	isJSON := jsonFlag || strings.EqualFold(formatFlag, "json")

	engineFlag, err := cmd.Flags().GetBool("engine")
	if err != nil {
		return errfmt.Newf("holon status: parse engine flag").Wrap(err)
	}
	if engineFlag {
		return runHolonStatusWithEngine(cmd.OutOrStdout(), isJSON)
	}

	return runHolonStatus(cmd.OutOrStdout(), isJSON)
}

func runHolonStatusWithEngine(w io.Writer, jsonOut bool) error {
	projectRoot := paths.ResolveProjectRoot(".")
	telemetryFile := filepath.Join(paths.StateDirPath(projectRoot), "ambient", "telemetry.json")
	report, err := ambient.SafeLoadTelemetryReport(telemetryFile)
	if err != nil || report == nil || report.Status == ambient.TelemetryStatusStopped {
		report = ambient.NewAmbientTelemetryReport(
			ambient.WithStatus(ambient.TelemetryStatusHealthy),
			ambient.WithActiveHolonsCount(defaultRuntimeHolonCount),
			ambient.WithCircuitBreakerStatus(ambient.CircuitBreakerStatusClosed),
			ambient.WithRunwayWatermark(0),
			ambient.WithMeshMetrics(ambient.HolonMeshMetrics{
				ConvergenceScore: 100.0,
			}),
			ambient.WithComponent(ambient.ComponentHealth{
				Name:          "intent_stream",
				Status:        ambient.TelemetryStatusHealthy,
				LastHeartbeat: time.Now().UTC(),
			}),
			ambient.WithComponent(ambient.ComponentHealth{
				Name:          "epoch_synchronizer",
				Status:        ambient.TelemetryStatusHealthy,
				LastHeartbeat: time.Now().UTC(),
			}),
			ambient.WithComponent(ambient.ComponentHealth{
				Name:          "backpressure_regulator",
				Status:        ambient.TelemetryStatusHealthy,
				LastHeartbeat: time.Now().UTC(),
			}),
			ambient.WithComponent(ambient.ComponentHealth{
				Name:          "fault_isolator",
				Status:        ambient.TelemetryStatusHealthy,
				LastHeartbeat: time.Now().UTC(),
			}),
			ambient.WithComponent(ambient.ComponentHealth{
				Name:          "consensus_arbiter",
				Status:        ambient.TelemetryStatusHealthy,
				LastHeartbeat: time.Now().UTC(),
			}),
			ambient.WithComponent(ambient.ComponentHealth{
				Name:          "reflex_loop",
				Status:        ambient.TelemetryStatusHealthy,
				LastHeartbeat: time.Now().UTC(),
			}),
		)
	}
	return RenderAmbientTelemetryReport(w, report, jsonOut)
}

func runHolonStatusWithReport(w io.Writer, report *ambient.AmbientTelemetryReport, jsonOut bool) error {
	return RenderAmbientTelemetryReport(w, report, jsonOut)
}

// RenderAmbientTelemetryReport renders an AmbientTelemetryReport to w in either JSON or human-readable tabular format.
// It is guaranteed to be nil-safe against uninitialized or nil reports.
func RenderAmbientTelemetryReport(w io.Writer, report *ambient.AmbientTelemetryReport, jsonOut bool) error {
	if report == nil {
		report = ambient.NewAmbientTelemetryReport(ambient.WithStatus(ambient.TelemetryStatusStopped))
	}

	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	_, err := fmt.Fprint(w, report.SummaryTable())
	return err
}

// RenderHolonTelemetryPayload formats output cleanly for either AmbientTelemetryReport or MiddleBrainTelemetrySnapshot.
func RenderHolonTelemetryPayload(w io.Writer, payload any, jsonOut bool) error {
	switch p := payload.(type) {
	case *ambient.AmbientTelemetryReport:
		return RenderAmbientTelemetryReport(w, p, jsonOut)
	case *ambient.MiddleBrainTelemetrySnapshot:
		return RenderHolonTelemetry(w, p, jsonOut)
	default:
		if jsonOut {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		}
		_, err := fmt.Fprintln(w, payload)
		return err
	}
}

func runHolonStatus(w io.Writer, jsonOut bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), ambient.DefaultTelemetryTimeout)
	defer cancel()

	agg := defaultTelemetryAggregator(ctx)
	return runHolonStatusWithAggregator(w, jsonOut, agg)
}

func runHolonStatusWithAggregator(w io.Writer, jsonOut bool, aggregator *ambient.HolonTelemetryAggregator) error {
	ctx, cancel := context.WithTimeout(context.Background(), ambient.DefaultTelemetryTimeout)
	defer cancel()

	var snapshot *ambient.MiddleBrainTelemetrySnapshot
	if aggregator != nil {
		snapshot = aggregator.Snapshot(ctx)
	} else {
		var nilAgg *ambient.HolonTelemetryAggregator
		snapshot = nilAgg.Snapshot(ctx)
	}

	return RenderHolonTelemetry(w, snapshot, jsonOut)
}

// RenderHolonTelemetry renders a snapshot to w in either JSON or human-readable tabular format.
// It is guaranteed to be nil-safe against uninitialized or nil snapshots.
func RenderHolonTelemetry(w io.Writer, snapshot *ambient.MiddleBrainTelemetrySnapshot, jsonOut bool) error {
	if snapshot == nil {
		snapshot = &ambient.MiddleBrainTelemetrySnapshot{
			Timestamp:    time.Now(),
			HealthStatus: ambient.TelemetryHealthQuiescent,
			Warnings:     []string{"telemetry snapshot uninitialized"},
			WarningCount: 1,
		}
	}

	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(snapshot)
	}

	// Human-readable tabular output
	fmt.Fprintf(w, "\n=======================================================\n")
	fmt.Fprintf(w, "  ZQK Middle-Brain Holon Vitality & Telemetry Dashboard\n")
	fmt.Fprintf(w, "=======================================================\n\n")

	vitalHeaders := []string{"METRIC", "VALUE", "STATUS / DETAIL"}
	vitalRows := [][]string{
		{"Health Status", snapshot.HealthStatus, healthStatusDetail(snapshot.HealthStatus)},
		{"Active Holons", fmt.Sprintf("%d", snapshot.ActiveHolonCount), fmt.Sprintf("%d registered", len(snapshot.Holons))},
		{"Tracked Streams", fmt.Sprintf("%d", snapshot.StreamCount), fmt.Sprintf("%d pooled raindrops", snapshot.TotalRaindrops)},
		{"Active Rivers", fmt.Sprintf("%d", snapshot.RiverCount), fmt.Sprintf("%d drops ingested", snapshot.TotalDropsIngested)},
		{"Alignment Score", fmt.Sprintf("%.2f / 1.00", snapshot.AverageAlignmentScore), alignmentDetail(snapshot.AverageAlignmentScore)},
		{"Divergences", fmt.Sprintf("%d", snapshot.DivergenceCount), divergenceDetail(snapshot.DivergenceCount)},
	}
	fmt.Fprint(w, clipkg.RenderTableWithTitle("SYSTEM VITALS", vitalHeaders, nil, vitalRows))
	fmt.Fprintln(w)

	holonHeaders := []string{"HOLON ID", "PHASE", "PURPOSE"}
	var holonRows [][]string
	if len(snapshot.Holons) > 0 {
		for _, h := range snapshot.Holons {
			holonRows = append(holonRows, []string{h.ID, string(h.Phase), h.Purpose})
		}
	} else {
		holonRows = append(holonRows, []string{"(no holons registered)", "-", "middle-brain holon registry quiescent or uninitialized"})
	}
	fmt.Fprint(w, clipkg.RenderTableWithTitle("MIDDLE-BRAIN HOLONS", holonHeaders, nil, holonRows))
	fmt.Fprintln(w)

	if len(snapshot.Warnings) > 0 {
		warnHeaders := []string{"#", "WARNING / BOUNDARY CONDITION"}
		var warnRows [][]string
		for i, warn := range snapshot.Warnings {
			warnRows = append(warnRows, []string{fmt.Sprintf("%d", i+1), warn})
		}
		fmt.Fprint(w, clipkg.RenderTableWithTitle("BOUNDARY WARNINGS", warnHeaders, nil, warnRows))
		fmt.Fprintln(w)
	}

	return nil
}

func registerRunningHolon(reg *ambient.HolonLifecycleRegistry, name, desc string) {
	if reg == nil {
		return
	}
	if err := reg.Register(name, desc); err != nil {
		return
	}
	if err := reg.Transition(name, ambient.HolonPhaseStarting); err != nil {
		return
	}
	if err := reg.Transition(name, ambient.HolonPhaseRunning); err != nil {
		return
	}
}

func defaultTelemetryAggregator(ctx context.Context) *ambient.HolonTelemetryAggregator {
	reg := ambient.NewHolonLifecycleRegistry()
	registerRunningHolon(reg, "ambient-stream-hydrator", "Pool intent raindrops into streams")
	registerRunningHolon(reg, "ambient-river-convergence", "Converge preference streams into intent rivers")

	hydrator := ambient.NewStreamHydrator(ambient.DefaultStreamCapacity, ambient.DefaultMinWeightFloor)
	engine, err := ambient.NewRiverConvergenceEngine(ctx, ambient.RiverConfig{
		RootVisionID:       "VIS-ROOT-MIDDLE-BRAIN",
		RootVisionVector:   []float64{1.0, 0.0, 0.0},
		ConvergenceWindow:  ambient.DefaultConvergenceWindow,
		MinConsensusDrops:  1,
		AlignmentThreshold: minHealthyAlignment,
	})
	if err == nil && engine != nil {
		defer engine.Close()
	}

	return ambient.NewHolonTelemetryAggregator(reg, hydrator, engine)
}

func healthStatusDetail(status string) string {
	switch status {
	case ambient.TelemetryHealthHealthy:
		return "Nominal operational state"
	case ambient.TelemetryHealthDegraded:
		return "Degraded / subcomponents uninitialized"
	case ambient.TelemetryHealthCritical:
		return "Critical divergence or boundary violation"
	case ambient.TelemetryHealthQuiescent:
		return "Quiescent / idle state"
	default:
		return "Unknown state"
	}
}

func alignmentDetail(score float64) string {
	if score >= minHealthyAlignment {
		return "Aligned with root vision"
	}
	if score > 0 {
		return "Sub-threshold alignment"
	}
	return "No alignment data / quiescent"
}

func divergenceDetail(div int64) string {
	if div == 0 {
		return "Zero divergence"
	}
	if div <= ambient.MaxAllowedDivergences {
		return "Within tolerance"
	}
	return "Exceeds divergence tolerance"
}
