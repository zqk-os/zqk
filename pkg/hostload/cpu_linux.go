//go:build linux

package hostload

import (
	"bytes"
	"strconv"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func readLoadAvg() (float64, bool) {
	data, err := fileutil.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, false
	}
	fields := bytes.Fields(data)
	if len(fields) < 1 {
		return 0, false
	}
	v, err := strconv.ParseFloat(string(fields[0]), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readCPUTicks() (idle, total uint64, ok bool) {
	data, err := fileutil.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	line, _, _ := bytes.Cut(data, []byte("\n"))
	fields := bytes.Fields(line)
	if len(fields) < 5 || string(fields[0]) != "cpu" {
		return 0, 0, false
	}
	var sum uint64
	for i := 1; i < len(fields); i++ {
		v, err := strconv.ParseUint(string(fields[i]), 10, 64)
		if err != nil {
			return 0, 0, false
		}
		sum += v
		// /proc/stat: user nice system idle iowait ...
		if i == 4 || i == 5 {
			idle += v
		}
	}
	if sum == 0 {
		return 0, 0, false
	}
	return idle, sum, true
}
