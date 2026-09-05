//go:build darwin && cgo

package hostload

import "testing"

func TestDarwinCGOReadCPUTicks(t *testing.T) {
	t.Parallel()
	idle, total, ok := readCPUTicks()
	if !ok {
		return
	}
	if idle > total {
		t.Fatalf("idle %d > total %d", idle, total)
	}
}
