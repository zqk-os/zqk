package ideadapter

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Dedicated adapter failure trail. Profile Fluent logs often never land on disk when
// IDE owns stdio; this file is always inspectable next to mcp-trace.log.
const ideAdapterDiagFile = "ide-adapter.log"

var (
	diagMu   sync.Mutex
	diagPath string
)

// SetDiagLogRoot sets the project root used for .zqk/mcp/logs/ide-adapter.log.
func SetDiagLogRoot(projectRoot string) {
	diagMu.Lock()
	defer diagMu.Unlock()
	if projectRoot == "" {
		diagPath = ""
		return
	}
	diagPath = filepath.Join(paths.MCPDirPath(projectRoot), paths.MCPLogsDir, ideAdapterDiagFile)
}

func diagf(format string, args ...any) {
	diagMu.Lock()
	path := diagPath
	diagMu.Unlock()
	if path == "" {
		return
	}
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
	_, _ = f.WriteString(line)
}
