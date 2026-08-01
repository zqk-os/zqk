package scheduler

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// GetRuntimeThreadCount returns the current process OS thread count when available.
// On Linux reads /proc/self/status "Threads:"; on other platforms returns 0
// (Go does not expose thread count in the standard library on Darwin/Windows).
// Used by scheduler health metrics and process dump diagnostics.
func GetRuntimeThreadCount() int {
	if runtime.GOOS != "linux" {
		return 0
	}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	prefix := "Threads:"
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			s := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			n, err := strconv.Atoi(s)
			if err != nil {
				return 0
			}
			return n
		}
	}
	return 0
}
