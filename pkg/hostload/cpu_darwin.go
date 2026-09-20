//go:build darwin

package hostload

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

func readLoadAvg() (float64, bool) {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil || len(raw) < 16 {
		return 0, false
	}
	ld0 := binary.LittleEndian.Uint32(raw[0:4])
	var fscale uint64
	switch {
	case len(raw) >= 24:
		fscale = binary.LittleEndian.Uint64(raw[16:24])
	case len(raw) >= 20:
		fscale = binary.LittleEndian.Uint64(raw[12:20])
	default:
		fscale = uint64(binary.LittleEndian.Uint32(raw[12:16]))
	}
	if fscale == 0 {
		return 0, false
	}
	return float64(ld0) / float64(fscale), true
}
