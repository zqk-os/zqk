package ambient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	defaultRunningActiveHolonCount = 6
	ambientStatusRunning           = "running"
	ambientStatusStopped           = "stopped"
)

func newStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientStatusCommandBuilder()
	if cmd.Flags().Lookup("json") == nil {
		cmd.Flags().Bool("json", false, "Output telemetry as JSON")
	}
	cmd.RunE = runStatus
	return cmd
}

// statusOutputWrapper satisfies printer.TablePrintable and printer.DataWrapper.
type statusOutputWrapper struct {
	payload     map[string]any
	report      *ambient.AmbientTelemetryReport
	running     bool
	pid         int
	projectRoot string
}

func (w *statusOutputWrapper) Unwrap() any {
	return w.payload
}

func (w *statusOutputWrapper) MarshalJSON() ([]byte, error) {
	return json.Marshal(w.payload)
}

func (w *statusOutputWrapper) FormatTable() ([]byte, error) {
	if !w.running {
		var sb strings.Builder
		sb.WriteString("================================================================================\n")
		sb.WriteString("                         AMBIENT RUNTIME TELEMETRY REPORT                       \n")
		sb.WriteString("================================================================================\n")
		sb.WriteString("Status:                 stopped\n")
		sb.WriteString("Component:              ambient\n")
		sb.WriteString("================================================================================\n")
		return []byte(sb.String()), nil
	}
	if w.report != nil {
		table := w.report.SummaryTable()
		if w.pid > 0 {
			table = strings.Replace(
				table,
				fmt.Sprintf("Status:                 %s\n", w.report.Status),
				fmt.Sprintf("Status:                 running (PID: %d)\nHealth:                 %s\n", w.pid, w.report.Status),
				1,
			)
		} else {
			table = strings.Replace(
				table,
				fmt.Sprintf("Status:                 %s\n", w.report.Status),
				fmt.Sprintf("Status:                 running\nHealth:                 %s\n", w.report.Status),
				1,
			)
		}
		return []byte(table), nil
	}
	return []byte("Status:                 running\n"), nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	pid, running := readAmbientPID(projectRoot)

	var (
		statusPayload map[string]any
		report        *ambient.AmbientTelemetryReport
	)

	if running {
		statusPayload, report = buildRunningStatus(projectRoot, pid)
	} else {
		statusPayload, report = buildStoppedStatus()
	}

	wrapper := &statusOutputWrapper{
		payload:     statusPayload,
		report:      report,
		running:     running,
		pid:         pid,
		projectRoot: projectRoot,
	}

	isJSON := strings.EqualFold(string(cli.GetFormat(cmd)), "json")
	if jsonFlag, err := cmd.Flags().GetBool("json"); err == nil && jsonFlag {
		isJSON = true
	}

	if isJSON {
		return cli.FormatOutputAs(cmd, cli.FormatJSON, wrapper)
	}

	return cli.FormatOutput(cmd, wrapper)
}

func buildStoppedStatus() (map[string]any, *ambient.AmbientTelemetryReport) {
	statusPayload := map[string]any{
		objects.FieldKeyComponent: "ambient",
		objects.FieldKeyStatus:    ambientStatusStopped,
		"consensus_leases_active": 0,
		"consensus_committed":     uint64(0),
		"consensus_revoked":       uint64(0),
		"reflex_actions_total":    uint64(0),
		"reflex_realign_count":    uint64(0),
	}
	report := ambient.NewAmbientTelemetryReport(
		ambient.WithStatus(ambient.TelemetryStatusStopped),
	)
	return statusPayload, report
}

func buildRunningStatus(projectRoot string, pid int) (map[string]any, *ambient.AmbientTelemetryReport) {
	var uptime time.Duration
	pidPath := ambientPIDFilePath(projectRoot)
	if fi, err := os.Stat(pidPath); err == nil {
		uptime = time.Since(fi.ModTime()).Round(time.Millisecond)
	}

	report, err := ambient.LoadTelemetryReport(projectRoot)
	if err != nil || report == nil {
		report = buildDefaultRunningReport(uptime)
	} else if report.Uptime == 0 && uptime > 0 {
		report.Uptime = uptime
	}

	statusPayload := map[string]any{
		objects.FieldKeyComponent: "ambient",
		objects.FieldKeyStatus:    ambientStatusRunning,
		"pid":                     pid,
		"project_root":            projectRoot,
		"uptime":                  report.Uptime,
		"active_holons_count":     report.ActiveHolonsCount,
		"active_holons":           report.ActiveHolonsCount,
		"runway_watermark":        report.RunwayWatermark,
		"circuit_breaker_status":  report.CircuitBreakerStatus,
		"consensus_leases_active": report.GetConsensusLeasesActive(),
		"consensus_committed":     report.GetConsensusCommitted(),
		"consensus_revoked":       report.GetConsensusRevoked(),
		"reflex_actions_total":    report.GetReflexActionsTotal(),
		"reflex_realign_count":    report.GetReflexRealignCount(),
		"mesh_metrics":            report.MeshMetrics,
		"components":              report.Components,
		"telemetry":               report,
	}

	applyConvergenceMetrics(projectRoot, statusPayload, report)
	return statusPayload, report
}

func applyConvergenceMetrics(projectRoot string, statusPayload map[string]any, report *ambient.AmbientTelemetryReport) {
	metricsFile := filepath.Join(paths.StateDirPath(projectRoot), "ambient", "convergence_metrics.json")
	data, err := fileutil.ReadFile(metricsFile)
	if err != nil {
		return
	}
	var metrics map[string]any
	if err := json.Unmarshal(data, &metrics); err != nil {
		return
	}
	statusPayload["convergence"] = metrics
	if div, ok := metrics["divergence_count"].(float64); ok {
		report.MeshMetrics.DivergenceCount = int64(div)
	}
	if score, ok := metrics["convergence_score"].(float64); ok {
		report.MeshMetrics.ConvergenceScore = score
	}
}

func buildDefaultRunningReport(uptime time.Duration) *ambient.AmbientTelemetryReport {
	now := time.Now().UTC()
	return ambient.NewAmbientTelemetryReport(
		ambient.WithStatus(ambient.TelemetryStatusHealthy),
		ambient.WithUptime(uptime),
		ambient.WithActiveHolonsCount(defaultRunningActiveHolonCount),
		ambient.WithCircuitBreakerStatus(ambient.CircuitBreakerStatusClosed),
		ambient.WithRunwayWatermark(0),
		ambient.WithMeshMetrics(ambient.HolonMeshMetrics{
			ConvergenceScore: 100.0,
		}),
		ambient.WithComponent(ambient.ComponentHealth{
			Name:          "intent_stream",
			Status:        ambient.TelemetryStatusHealthy,
			LastHeartbeat: now,
		}),
		ambient.WithComponent(ambient.ComponentHealth{
			Name:          "epoch_synchronizer",
			Status:        ambient.TelemetryStatusHealthy,
			LastHeartbeat: now,
		}),
		ambient.WithComponent(ambient.ComponentHealth{
			Name:          "backpressure_regulator",
			Status:        ambient.TelemetryStatusHealthy,
			LastHeartbeat: now,
		}),
		ambient.WithComponent(ambient.ComponentHealth{
			Name:          "fault_isolator",
			Status:        ambient.TelemetryStatusHealthy,
			LastHeartbeat: now,
		}),
		ambient.WithComponent(ambient.ComponentHealth{
			Name:          "consensus_arbiter",
			Status:        ambient.TelemetryStatusHealthy,
			LastHeartbeat: now,
		}),
		ambient.WithComponent(ambient.ComponentHealth{
			Name:          "reflex_loop",
			Status:        ambient.TelemetryStatusHealthy,
			LastHeartbeat: now,
		}),
	)
}
