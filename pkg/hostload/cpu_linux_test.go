//go:build linux

package hostload

import "testing"

func TestLinuxProcSensors(t *testing.T) {
	t.Parallel()
	if _, ok := readLoadAvg(); !ok {
		t.Fatal("/proc/loadavg unreadable")
	}
	idle, total, ok := readCPUTicks()
	if !ok {
		t.Fatal("/proc/stat unreadable")
	}
	if idle > total {
		t.Fatalf("idle %d > total %d", idle, total)
	}
}
