// Package mcpkeepalive implements the MCP daemon keep-alive heartbeat and the
// LaunchAgent supervision template that keeps the MCP KeepAlive daemon
// supervised across launchd recycles.
//
// Plane: core
// Portable KeepAlive + LaunchAgent Render live here; studio re-exports.
//
// Design: launchd (KeepAlive=true) supervises `mcp supervise --loop`; the loop
// itself re-ensures the daemon every tick and refreshes the keep-alive
// heartbeat file, so status tooling can distinguish a healthy supervised
// daemon from one whose supervisor was recycled or died.
package mcpkeepalive

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	dataDirName     = "mcp"
	keepAlivePrefix = "daemon-"
	keepAliveSuffix = ".keepalive"
	dirPerm         = fileutil.FileMode(0o755)
	filePerm        = fileutil.FileMode(0o644)
)

// DefaultStaleAfter is how old a keep-alive heartbeat may be before callers
// treat the supervised daemon as down.
const DefaultStaleAfter = 15 * time.Second

// KeepAlive tracks the per-port heartbeat file for one MCP daemon.
type KeepAlive struct {
	root  string
	port  string
	path  string
	clock func() time.Time
}

// New builds a KeepAlive bound to projectRoot and daemon port.
func New(projectRoot, port string) *KeepAlive {
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, dataDirName)
	return &KeepAlive{
		root:  projectRoot,
		port:  strings.TrimSpace(port),
		path:  filepath.Join(dir, keepAlivePrefix+strings.TrimSpace(port)+keepAliveSuffix),
		clock: time.Now,
	}
}

// Path returns the heartbeat file location.
func (k *KeepAlive) Path() string { return k.path }

// Write refreshes the heartbeat with the current timestamp.
func (k *KeepAlive) Write() error {
	if err := fileutil.MkdirAll(filepath.Dir(k.path), dirPerm); err != nil {
		return err
	}
	return fileutil.WriteFile(k.path, []byte(strconv.FormatInt(k.clock().UnixNano(), 10)), filePerm)
}

// Read returns the last heartbeat timestamp.
func (k *KeepAlive) Read() (time.Time, error) {
	b, err := fileutil.ReadFile(k.path) //nolint:gosec
	if err != nil {
		return time.Time{}, err
	}
	nanos, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, nanos), nil
}

// Remove deletes the heartbeat file; missing files are not an error.
func (k *KeepAlive) Remove() error {
	return fileutil.RemoveFileIfExists(k.path)
}

// IsFresh reports whether a heartbeat exists and is younger than staleAfter.
func (k *KeepAlive) IsFresh(staleAfter time.Duration) bool {
	ts, err := k.Read()
	if err != nil {
		return false
	}
	return k.clock().Sub(ts) < staleAfter
}
