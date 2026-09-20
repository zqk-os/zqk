//go:build darwin && !cgo

package hostload

// Without CGO there is no Mach host_statistics. Load average still works;
// idle-tick sensing fails open (LevelHeadroom unless loadavg says otherwise).
func readCPUTicks() (idle, total uint64, ok bool) { return 0, 0, false }
