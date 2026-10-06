package process

import (
	"sync"
	"time"

	"github.com/mitchellh/go-ps"
)

var (
	procTableCache map[int]int
	procTableTime  time.Time
	procTableMu    sync.RWMutex
	procCacheTTL   = 2 * time.Second
)

// getCachedPPIDMap returns a snapshot map of pid -> ppid.
// Caches for 2 seconds to avoid repeated process table walks during burst ancestor checks.
func getCachedPPIDMap() map[int]int {
	procTableMu.RLock()
	if procTableCache != nil && time.Since(procTableTime) < procCacheTTL {
		m := procTableCache
		procTableMu.RUnlock()
		return m
	}
	procTableMu.RUnlock()

	procTableMu.Lock()
	defer procTableMu.Unlock()
	if procTableCache != nil && time.Since(procTableTime) < procCacheTTL {
		return procTableCache
	}

	procs, err := ps.Processes()
	if err != nil {
		return nil
	}
	m := make(map[int]int, len(procs))
	for _, p := range procs {
		m[p.Pid()] = p.PPid()
	}
	procTableCache = m
	procTableTime = time.Now()
	return m
}

// ParentPID returns the parent PID of pid, or 0 if it cannot be resolved.
func ParentPID(pid int) int {
	if pid <= 1 {
		return 0
	}
	if pmap := getCachedPPIDMap(); pmap != nil {
		if ppid, ok := pmap[pid]; ok {
			return ppid
		}
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
	if ancestor == descendant {
		return true
	}

	pmap := getCachedPPIDMap()
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
		var next int
		if pmap != nil {
			next = pmap[pid]
		} else {
			next = ParentPID(pid)
		}
		if next <= 0 || next == pid {
			return false
		}
		pid = next
	}
	return pid == ancestor
}
