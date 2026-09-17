package mcp

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"context"
	"time"

	gonet "github.com/shirou/gopsutil/v3/net"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	pkgmcp "github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// DefaultMCPDaemonTCP aliases the package-level default for CLI callers/tests.
const DefaultMCPDaemonTCP = pkgmcp.DefaultDaemonTCP

// resolveTCPFlag returns a trimmed TCP address, falling back to DefaultMCPDaemonTCP.
func resolveTCPFlag(raw string) string {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return DefaultMCPDaemonTCP
	}
	return addr
}

// tcpPort returns the trailing host:port segment used for pid-file naming.
func tcpPort(tcpAddr string) string {
	parts := strings.Split(tcpAddr, ":")
	return parts[len(parts)-1]
}

func projectRootOrResolve(procProjectRoot string) string {
	if strings.TrimSpace(procProjectRoot) != "" {
		return procProjectRoot
	}
	return cli.ResolveProjectRoot(".")
}

func mcpDaemonPIDPath(projectRoot, port string) string {
	return filepath.Join(paths.MCPDirPath(projectRoot), "daemon-"+port+".pid")
}

func mcpSupervisePIDPath(projectRoot, port string) string {
	return filepath.Join(paths.MCPDirPath(projectRoot), "daemon-"+port+".supervise.pid")
}

func mcpDaemonLogPath(projectRoot, port string) string {
	return filepath.Join(paths.LogsDirPath(projectRoot), "mcp-daemon-"+port+".log")
}

func mcpSuperviseLogPath(projectRoot string) string {
	return filepath.Join(paths.LogsDirPath(projectRoot), "mcp-daemon-supervise.log")
}

func writePIDFile(path string, pid int) error {
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil { //nolint:gosec
		return err
	}
	return fileutil.WriteStandardFile(path, []byte(strconv.Itoa(pid)+"\n"))
}

func readPIDFile(path string) (int, bool) {
	b, err := fileutil.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// resolveMCPDaemonBinPath chooses the binary used to spawn `mcp daemon` / ensure.
// Workshop runtime hangs off stable so tip rebuilds (bin/zqk) cannot split-brain
// MCP vs scheduler. Promote tip→stable intentionally via install-zqk-stable.sh.
// Override with ZQK_BIN. Fall back: workshop/repo stable → tip bin/zqk → executable.
// TRACK: TDE-1785808957221945000-fcd15e47 — one operational inode for long-lived procs.
func resolveMCPDaemonBinPath(projectRoot string) string {
	if zqkBin := zqkenv.Bin().Get(); zqkBin != "" && fileutil.IsRegularFile(zqkBin) {
		return zqkBin
	}
	for _, stablePath := range paths.StableBinaryCandidates(projectRoot) {
		if fileutil.IsRegularFile(stablePath) {
			return stablePath
		}
	}
	if defaultBin := paths.RepoBinPath(projectRoot); fileutil.IsRegularFile(defaultBin) {
		return defaultBin
	}
	if exe, err := fileutil.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if fileutil.IsRegularFile(exe) {
			return exe
		}
	}
	return brand.ExecutableName()
}

// getStableBinPath is the historical name used by ensure/supervise tests.
// MCP spawning uses resolveMCPDaemonBinPath (invoking binary first).
func getStableBinPath(projectRoot string) string {
	return resolveMCPDaemonBinPath(projectRoot)
}

func getDaemonPIDFromPort(port string) (int, bool) {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return 0, false
	}
	conns, err := gonet.Connections("tcp")
	if err != nil {
		return 0, false
	}
	for _, c := range conns {
		if c.Status == "LISTEN" && int(c.Laddr.Port) == portInt && c.Pid > 0 {
			return int(c.Pid), true
		}
	}
	return 0, false
}

func getSupervisePID(pidFile string) (int, bool) {
	pid, ok := readPIDFile(pidFile)
	if !ok {
		return 0, false
	}
	if !processAlive(pid) {
		return 0, false
	}
	return pid, true
}

// superviseStatusPayload builds the FormatOutput map for mcp supervise --status.
func superviseStatusPayload(tcpAddr, port, supPidFile string) map[string]any {
	payload := map[string]any{"tcp": tcpAddr}
	if pid, ok := getDaemonPIDFromPort(port); ok {
		daemonStatus := map[string]any{"listening": true, "pid": pid}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Background: request-or-shutdown derived
		defer cancel()
		pd := pkgmcp.NewProxyDaemon(tcpAddr, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
		if diag, err := pd.QueryDiagnostics(ctx); err == nil {
			for k, v := range diag {
				daemonStatus[k] = v
			}
		}

		payload["daemon"] = daemonStatus
	} else {
		payload["daemon"] = map[string]any{"listening": false}
	}
	if pid, ok := getSupervisePID(supPidFile); ok {
		payload["supervisor"] = map[string]any{"running": true, "pid": pid}
	} else {
		payload["supervisor"] = map[string]any{"running": false}
	}
	return payload
}

// killPIDBestEffort signals Kill to a pid if the process can be found.
func killPIDBestEffort(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
