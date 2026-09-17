package storage

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
)

// PID shards are named YYYY-MM-DD_pid<PID>_stream.json (see getSegmentPath).
// TRACK: BLI-REDACTED
var streamPIDSegmentRe = regexp.MustCompile(`_pid(\d+)_`)

func streamSegmentWriterPID(name string) (int, bool) {
	m := streamPIDSegmentRe.FindStringSubmatch(name)
	if len(m) < 2 {
		return 0, false
	}
	pid, err := strconv.Atoi(m[1])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func unixPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func streamSegmentWriterStillLive(name string) bool {
	pid, ok := streamSegmentWriterPID(name)
	if !ok {
		return false
	}
	return unixPIDAlive(pid)
}

// streamMergeFilesForDate returns the segment files that may be compacted for date.
// Today's live-PID shards stay on disk (writers still append); dead-PID shards merge.
func streamMergeFilesForDate(date, todayBase string, files []string) (mergeFiles []string, skip bool) {
	if date != todayBase {
		return files, false
	}
	dead := make([]string, 0, len(files))
	for _, file := range files {
		if streamSegmentWriterStillLive(filepath.Base(file)) {
			continue
		}
		dead = append(dead, file)
	}
	if len(dead) == 0 {
		return nil, true
	}
	return dead, false
}
