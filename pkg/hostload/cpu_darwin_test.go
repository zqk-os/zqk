//go:build darwin

package hostload

import "testing"

func TestDarwinSensors_Readable(t *testing.T) {
	t.Parallel()
	load, ok := readLoadAvg()
	if !ok {
		t.Fatal("vm.loadavg sysctl failed")
	}
	if load < 0 {
		t.Fatalf("load %v", load)
	}
	if idle, total, ok := readCPUTicks(); ok {
		if idle > total {
			t.Fatalf("idle %d > total %d", idle, total)
		}
	}
}
