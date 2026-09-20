package process

import (
	"github.com/mitchellh/go-ps"
)

// ParentPID returns the parent PID of pid, or 0 if it cannot be resolved.
func ParentPID(pid int) int {
	if pid <= 1 {
		return 0
	}
	proc, err := ps.FindProcess(pid)
	if err != nil || proc == nil {
		return 0
	}
	return proc.PPid()
}

// IsAncestorPID reports whether ancestor appears in the parent chain of descendant
// (including descendant itself). Walk is bounded to break pid-reuse cycles.
func IsAncestorPID(ancestor, descendant int) bool {
	if ancestor <= 0 || descendant <= 0 {
		return false
	}
	seen := make(map[int]struct{}, 16)
	pid := descendant
	for pid > 1 {
		if pid == ancestor {
			return true
		}
		if _, dup := seen[pid]; dup {
			return false
		}
		seen[pid] = struct{}{}
		next := ParentPID(pid)
		if next <= 0 || next == pid {
			return false
		}
		pid = next
	}
	return pid == ancestor
}
