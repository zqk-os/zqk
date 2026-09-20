//go:build !darwin && !linux

package hostload

import "testing"

func TestStubSensorsUnavailable(t *testing.T) {
	t.Parallel()
	if _, ok := readLoadAvg(); ok {
		t.Fatal("stub loadavg must be unavailable")
	}
	if _, _, ok := readCPUTicks(); ok {
		t.Fatal("stub ticks must be unavailable")
	}
}
