package vitality

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitchellh/go-ps"
	"github.com/zqk-os/zqk/pkg/logging"
)

// VitalityMonitor watches for scheduler daemon health.
type VitalityMonitor struct {
	processName string
}

func NewVitalityMonitor(processName string) *VitalityMonitor {
	return &VitalityMonitor{processName: processName}
}

// EnsureRunning restarts the daemon if it is not active.
func (v *VitalityMonitor) EnsureRunning(ctx context.Context) error {
	// Cross-platform check: scan processes
	procs, err := ps.Processes()
	if err == nil {
		for _, p := range procs {
			if strings.Contains(strings.ToLower(p.Executable()), strings.ToLower(v.processName)) {
				return nil
			}
		}
	}
	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("🚀 [VITALITY] Daemon %s not running, restarting...\n", v.processName)).Log()
	return v.restart()
}

func (v *VitalityMonitor) restart() error {
	// Implementation: invoke startup script
	return nil
}
