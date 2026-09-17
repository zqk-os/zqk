//go:build !darwin && !linux

package hostload

func readLoadAvg() (float64, bool) { return 0, false }

func readCPUTicks() (idle, total uint64, ok bool) { return 0, 0, false }
