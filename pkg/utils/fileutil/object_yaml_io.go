package fileutil

import (
	"strconv"
	"sync"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Process-wide cap on concurrent object-YAML reads (open/read). List/CAS fast-paths
// used to bypass the I/O queue; each blocked G mints an OS thread (M) that Darwin
// never returns. Sample 2026-09-02: zqk-stable ~2041 parked Ms.
// TRACK: BLI-CEF-STORAGE-INDEX-CACHE-001
const (
	defaultMaxObjectYAMLIO = 32
	minMaxObjectYAMLIO     = 8
	maxMaxObjectYAMLIO     = 64
)

var (
	objectYAMLIOOnce sync.Once
	objectYAMLIOSem  chan struct{}
	objectYAMLIOMax  int
)

func maxObjectYAMLIO() int {
	objectYAMLIOOnce.Do(func() {
		n := defaultMaxObjectYAMLIO
		if v := zqkenv.MaxObjectYAMLIO().Get(); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				n = parsed
			}
		}
		if n < minMaxObjectYAMLIO {
			n = minMaxObjectYAMLIO
		}
		if n > maxMaxObjectYAMLIO {
			n = maxMaxObjectYAMLIO
		}
		objectYAMLIOMax = n
		objectYAMLIOSem = make(chan struct{}, n)
	})
	return objectYAMLIOMax
}

func acquireObjectYAMLIO() {
	maxObjectYAMLIO()
	objectYAMLIOSem <- struct{}{}
}

func releaseObjectYAMLIO() {
	<-objectYAMLIOSem
}

// ReadFileGated is ReadFile under the process-wide object-YAML I/O gate.
// Use for CAS/list YAML blobs, not for logs or incidental reads.
func ReadFileGated(path string) ([]byte, error) {
	acquireObjectYAMLIO()
	defer releaseObjectYAMLIO()
	return ReadFile(path)
}

// ObjectYAMLIOLimit reports the configured gate size (for tests).
func ObjectYAMLIOLimit() int {
	return maxObjectYAMLIO()
}
