package diagnostics

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
)

const (
	osWindows                    = "windows"
	goroutineSignalHandler       = "diagnostics_signal_handler"
	goroutineCapture             = "diagnostics_capture"
	signalCapturePurposeFmt      = "handling SIGUSR1 for diagnostics capture (prefix: %s)"
	captureAfterSignalPurposeFmt = "capturing diagnostics after SIGUSR1 (prefix: %s)"
	panicValueFmt                = "%v"
	diagnosticsSubdir            = "diagnostics"
	logMsgCapturePanicked        = "Diagnostics capture panicked"
	logMsgCaptureFailed          = "Diagnostics capture failed"
	logMsgCaptured               = "Diagnostics captured"
	logFieldOutputDir            = "output_dir"
	systemProfileName            = string(pkgctx.ProfileSystem)
)

// SetupSignalHandler sets up SIGUSR1 signal handling to capture diagnostics.
// When SIGUSR1 is received, diagnostics are captured to outputDir with the given prefix.
// The handler runs in a background goroutine and respects context cancellation.
func SetupSignalHandler(ctx context.Context, projectRoot, prefix string) {
	if runtime.GOOS == osWindows {
		return // SIGUSR1 not available on Windows
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1)

	bud := goroutinelabels.DefaultBudget()
	goroutinelabels.NewGoroutine(goroutineSignalHandler, fmt.Sprintf(signalCapturePurposeFmt, prefix)).
		WithContext(ctx).
		WithBudget(bud).
		StartSimple(func() {
			defer signal.Stop(sigChan)
			for {
				select {
				case <-ctx.Done():
					return
				case sig, ok := <-sigChan:
					if !ok {
						return
					}
					if sig == syscall.SIGUSR1 {
						// Capture diagnostics in background goroutine so handler can receive more signals
						captureBuilder := goroutinelabels.NewGoroutine(goroutineCapture, fmt.Sprintf(captureAfterSignalPurposeFmt, prefix))
						if bud != nil {
							captureBuilder = captureBuilder.WithBudget(bud)
						}
						captureBuilder.StartSimple(func() {
							// POL-CODE-007: use logging framework for all output
							logger := logging.GetLoggerFromProfile(systemProfileName)
							defer func() {
								if r := recover(); r != nil {
									logging.Fluent(logger).Error(logMsgCapturePanicked, errfmt.Errorf(panicValueFmt, r)).Log()
								}
							}()

							outputDir := filepath.Join(projectRoot, paths.ProjectDataDir, diagnosticsSubdir, prefix)
							if err := CaptureDiagnostics(outputDir, prefix); err != nil {
								logging.Fluent(logger).Error(logMsgCaptureFailed, err).
									String(logFieldOutputDir, outputDir).
									Log()
							} else {
								logging.Fluent(logger).Info(logMsgCaptured).
									String(logFieldOutputDir, outputDir).
									Log()
							}
						})
					}
				}
			}
		})
}
