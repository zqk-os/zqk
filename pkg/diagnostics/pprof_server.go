package diagnostics

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const (
	defaultPort  = 6060
	emptyPortVal = ""

	// legacyZQKPprof are documented in SCHEDULER_DIAGNOSTICS_VIEWING.md as ZQK_PPROF / ZQK_PPROF_PORT.
	// The scheduler daemon child sets argv[0] to "zqk-scheduler", so brand.EnvPrefix() is ZQK_SCHEDULER and
	// zqkenv.Pprof() already resolves to ZQK_SCHEDULER_PPROF; users often set ZQK_PPROF or ZQK_STABLE_PPROF
	// instead — try those as fallbacks.
	legacyZQKPprof                = "ZQK_PPROF"
	legacyZQKPprofPort            = "ZQK_PPROF_PORT"
	fallbackSchedulerDaemonPprof  = "ZQK_SCHEDULER_PPROF"
	fallbackSchedulerDaemonPort   = "ZQK_SCHEDULER_PPROF_PORT"
	fallbackStableBinaryPprof     = "ZQK_STABLE_PPROF"
	fallbackStableBinaryPprofPort = "ZQK_STABLE_PPROF_PORT"
)

func pprofEnvTruthy(v string) bool {
	return v == "1" || v == "true" || v == "TRUE"
}

func pprofEnabledFromEnv() bool {
	for _, key := range []string{
		zqkenv.Pprof(),
		legacyZQKPprof,
		fallbackSchedulerDaemonPprof,
		fallbackStableBinaryPprof,
	} {
		if pprofEnvTruthy(os.Getenv(key)) {
			return true
		}
	}
	return false
}

func pprofPortFromEnv() int {
	for _, key := range []string{
		zqkenv.PprofPort(),
		legacyZQKPprofPort,
		fallbackSchedulerDaemonPort,
		fallbackStableBinaryPprofPort,
	} {
		if p := os.Getenv(key); p != emptyPortVal {
			if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
				return n
			}
		}
	}
	return defaultPort
}

// StartPprofServerIfEnabled starts an HTTP server on localhost serving /debug/pprof
// when any recognized PPROF env is truthy (brand-prefixed [zqkenv.Pprof], ZQK_PPROF,
// ZQK_SCHEDULER_PPROF for detached daemon, or ZQK_STABLE_PPROF).
// Port from matching *PORT vars in the same precedence order, default 6060.
// Used by the scheduler daemon (and optionally CLI) to capture heap/CPU/goroutine
// profiles for performance investigation. Bind address is always localhost.
func StartPprofServerIfEnabled() {
	if !pprofEnabledFromEnv() {
		return
	}
	port := pprofPortFromEnv()
	addr := fmt.Sprintf("localhost:%d", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second, // allow pprof.Profile (default 30s) to complete
	}
	goroutinelabels.NewGoroutine("diagnostics", "pprof http server").
		StartSimple(func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn("[pprof] server exited").
					String("addr", addr).
					WithError(err).
					Log()
			}
		})
	logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Info("[pprof] enabled").
		URL(fmt.Sprintf("http://%s/debug/pprof/", addr)).
		DiagNote("heap, profile, goroutine, etc.").
		Log()
}
