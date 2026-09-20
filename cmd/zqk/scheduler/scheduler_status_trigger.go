package scheduler

import (
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clicontext "github.com/zqk-os/zqk/internal/cli/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/functional"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/strutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// showSchedulerStatus shows scheduler daemon status
func showSchedulerStatus(cmd *cobra.Command) error {
	return runSchedulerControlWithTimeout("scheduler status", func() error {
		ctx := cli.GetContext(cmd)
		if ctx == nil {
			return errfmt.Errorf("failed to get context")
		}

		status, err := getSchedulerStatus(ctx)
		if err != nil {
			// Build status data for output
			statusData := map[string]any{
				schedulerStatusRunning: false,
				objects.FieldKeyStatus: schedulerStatusNotRunning,
			}
			return outputSchedulerStatus(cmd, statusData)
		}

		// Build status data
		statusData := map[string]any{
			schedulerStatusRunning: status.Running,
		}
		if status.ProjectRoot != emptyValue {
			statusData[schedulerFieldProjectRoot] = status.ProjectRoot
		}

		functional.When(func() bool { return !status.Running }).Then(func() {
			statusData[objects.FieldKeyStatus] = schedulerStatusNotRunning
		}).OrElseWhen(func() bool { return status.InProcess }).Then(func() {
			statusData[objects.FieldKeyStatus] = schedulerStatusRunning
			statusData[schedulerFieldInProcess] = true
		}).OrElse(func() {
			statusData[objects.FieldKeyStatus] = schedulerStatusRunning
			statusData[schedulerFieldInProcess] = false
			statusData[schedulerFieldPID] = status.ProcessID
		}).Run()

		return outputSchedulerStatus(cmd, statusData)
	})
}

// outputSchedulerStatus outputs scheduler status in the requested format
func outputSchedulerStatus(cmd *cobra.Command, statusData map[string]any) error {
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, statusData)
	default:
		return outputSchedulerStatusTable(cmd, statusData)
	}
}

// outputSchedulerStatusTable outputs scheduler status as a table
func outputSchedulerStatusTable(cmd *cobra.Command, statusData map[string]any) error {
	var buf strings.Builder

	running, ok := statusData[schedulerStatusRunning].(bool)
	if !ok {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Warn("Failed to parse running status from status data").Log()
	}

	functional.When(func() bool { return !running }).Then(func() {
		buf.WriteString("Scheduler daemon: Not running\n")
		if proot, ok := statusData[schedulerFieldProjectRoot].(string); ok && proot != emptyValue {
			fmt.Fprintf(&buf, "  Checked: %s (run from project dir or set %s)\n", paths.SchedulerPIDFilePath(proot), zqkenv.ProjectRoot().Name())
		} else {
			fmt.Fprintf(&buf, "  No project root (run from project directory or set %s)\n", zqkenv.ProjectRoot().Name())
		}
	}).OrElse(func() {
		buf.WriteString("Scheduler daemon: Running")
		functional.When(func() bool { inProcess, ok := statusData[schedulerFieldInProcess].(bool); return ok && inProcess }).Then(func() {
			buf.WriteString(" (in this process)\n")
		}).OrElseWhen(func() bool { _, ok := statusData[schedulerFieldPID].(int); return ok }).Then(func() {
			fmt.Fprintf(&buf, " (PID: %d)\n", statusData[schedulerFieldPID].(int))
		}).OrElse(func() {
			buf.WriteString("\n")
		}).Run()
	}).Run()

	return cli.WriteOutput(cmd, []byte(buf.String()))
}

// triggerJob manually triggers a job
func triggerJob(ctx *cli.Context, cmd *cobra.Command, jobID string) error {
	// When ZQK_TEST_ROOT is set, use ctx.ProjectRoot so tests (e.g. TestTriggerJob_SchedulerNotRunning)
	// always operate on the test root and getSchedulerInstance sees consistent status (no daemon).
	var projectRoot string
	if zqkenv.TestRoot().Get() != emptyValue && ctx != nil && ctx.ProjectRoot != emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == emptyValue {
		// Project root from settings; if unavailable use persisted root from 'zqk use'; "." only finds workspace.
		var err error
		projectRoot, _, err = clicontext.ResolveProjectRootFromSettings(".")
		if err != nil || projectRoot == emptyValue {
			projectRoot = cli.ResolveProjectRoot(".")
		}
		if projectRoot == emptyValue && ctx != nil {
			projectRoot = ctx.ProjectRoot
		}
	}
	if projectRoot == emptyValue {
		return errors.New(schedulerErrProjectRootNotFound)
	}
	if _, err := clicontext.LoadBrandSettings(projectRoot); err != nil {
		return errfmt.Errorf(schedulerErrBrandSettingsRequired, err)
	}
	profile := strutil.OrDefault(ctx.Profile, schedulerProfileSystem)

	// Create storage provider for coordinator (best effort); use cache to avoid extra instances
	var storageProvider storagepkg.ObjectStorageProvider
	if projectRoot != emptyValue {
		storageCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), schedulerCoordinatorTimeout)
		provider, factoryErr := storagepkg.GetGlobalStorageProviderCache().GetOrCreate(storageCtx, projectRoot)
		cancel()
		if factoryErr == nil {
			storageProvider = provider
		}
	}

	// Get scheduler instance (must be running in this process)
	sched, status, err := getSchedulerInstance(ctx)
	if err != nil {
		if status != nil && status.Running && !status.InProcess {
			// Scheduler is running in another process - use queue-based triggering
			emitSchedulerTriggerEventViaCoordinator(
				pkgctx.NewSystemContext(),
				projectRoot,
				storageProvider,
				"Scheduler running in another process, using trigger queue",
				jobID,
				profile,
				map[string]any{
					"scheduler_pid": status.ProcessID,
				},
			)
			preCommit, errPreCommit := cmd.Flags().GetBool(schedulerFlagPreCommit)
			if errPreCommit != nil {
				return errfmt.Newf("failed to parse 'pre-commit' flag").Wrap(errPreCommit)
			}
			return enqueueJobTriggerRequest(ctx, cmd, jobID, preCommit)
		}
		return err
	}

	emitSchedulerTriggerEventViaCoordinator(
		pkgctx.NewSystemContext(),
		projectRoot,
		storageProvider,
		"Triggering scheduler job",
		jobID,
		profile,
		nil,
	)

	baseCtx := pkgctx.NewSystemContext()
	preCommit, errPreCommit := cmd.Flags().GetBool(schedulerFlagPreCommit)
	if errPreCommit == nil && preCommit {
		baseCtx = schedulerpkg.ContextWithTriggerOrigin(baseCtx, schedulerpkg.TriggerOriginPreCommit)
	} else {
		baseCtx = schedulerpkg.ContextWithTriggerOrigin(baseCtx, schedulerpkg.TriggerOriginCLISubmit)
	}
	if err := sched.TriggerJob(baseCtx, jobID); err != nil {
		return errfmt.Newf("failed to trigger job").Wrap(err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	schedulerpkg.SLog(logger).Info("Job triggered successfully").
		JobID(jobID).
		Log()
	return nil
}

const (
	// WebSocket protocol magic constants (RFC 6455)
	rfc6455WebSocketGuid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsOpcodeText         = 0x81
	wsLenThresholdSmall  = 125
	wsLenThresholdMedium = 126
	wsLenMarkerLarge     = 127
	wsMaxUint16          = 65535

	// Server configurations
	routeTelemetryWS        = "/api/ws"
	telemetryTickerDuration = 100 * time.Millisecond
	webSocketReadBufferSize = 1024
)

func upgradeToWebSocket(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("webserver doesn't support hijacking")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	h := sha1.New() //nolint:gosec
	h.Write([]byte(key + rfc6455WebSocketGuid))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	_, _ = bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	_, _ = bufrw.WriteString("Upgrade: websocket\r\n")
	_, _ = bufrw.WriteString("Connection: Upgrade\r\n")
	_, _ = bufrw.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n\r\n")
	_ = bufrw.Flush()

	return conn, nil
}

func writeWebSocketTextFrame(conn net.Conn, payload []byte) error {
	var header []byte
	length := len(payload)

	header = append(header, wsOpcodeText)

	if length <= wsLenThresholdSmall {
		header = append(header, byte(length))
	} else if length <= wsMaxUint16 {
		header = append(header, wsLenThresholdMedium, byte(length>>8), byte(length)) //nolint:gosec
	} else {
		header = append(header, wsLenMarkerLarge)
		for i := 7; i >= 0; i-- {
			header = append(header, byte(length>>(i*8))) //nolint:gosec
		}
	}

	_, err := conn.Write(append(header, payload...))
	return err
}
