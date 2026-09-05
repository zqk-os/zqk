//go:build darwin && cgo

package hostload

import "github.com/shirou/gopsutil/v3/cpu"

// Darwin has no kern.cp_time on macOS 26. Mach host_statistics (via gopsutil)
// supplies idle/user/system ticks so short AV bursts are visible.
func readCPUTicks() (idle, total uint64, ok bool) {
	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return 0, 0, false
	}
	t := times[0]
	idleMs := uint64(t.Idle * 1000)
	totalMs := uint64((t.User + t.System + t.Nice + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal) * 1000)
	if totalMs == 0 {
		return 0, 0, false
	}
	return idleMs, totalMs, true
}
