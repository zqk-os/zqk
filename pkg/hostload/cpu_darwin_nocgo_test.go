//go:build darwin && !cgo

package hostload

import "testing"

func TestDarwinNoCGOTicksUnavailable(t *testing.T) {
	t.Parallel()
	if _, _, ok := readCPUTicks(); ok {
		t.Fatal("nocgo Darwin ticks must fail open")
	}
}
