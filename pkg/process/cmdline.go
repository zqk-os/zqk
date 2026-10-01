package process

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
)

// ProcessCommandLine returns the command line string for a process PID across supported platforms.
// On Linux, it inspects /proc/<pid>/cmdline without invoking subprocesses.
// On Darwin and other Unix systems, it queries ps.
// On Windows, it returns an empty string without invoking ps.
func ProcessCommandLine(pid int) string {
	if pid <= 0 {
		return ""
	}
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err == nil && len(data) > 0 {
			parts := strings.Split(string(data), "\x00")
			return strings.TrimSpace(strings.Join(parts, " "))
		}
		return ""
	}
	if runtime.GOOS == "windows" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := execwrap.CommandContext(ctx, "ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
