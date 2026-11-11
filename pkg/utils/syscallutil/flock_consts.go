package syscallutil

// Portable flock / process helpers for product builds.
// TRACK: g13-windows-cross-compile (open-core)

const (
	LockEx = 2 // syscall.LOCK_EX on unix
	LockNB = 4 // syscall.LOCK_NB on unix
	LockUn = 8 // syscall.LOCK_UN on unix
)

// EWouldBlock is the portable would-block errno for non-blocking flock.
// On unix this matches syscall.EWOULDBLOCK / unix.EWOULDBLOCK.
var EWouldBlock = errWouldBlock()
