#!/bin/sh
set -eu

# check-process-fd-leaks.sh
# Release Gate: Verifies that no running zqk process or background daemon
# suffers from file descriptor leaks (e.g. unbounded kqueue/inotify directory walks).

MAX_ALLOWED_FDS="${ZQK_MAX_ALLOWED_FDS:-800}"
FAILED=0

say() {
	printf '%s\n' "$*"
}

fail() {
	printf '\033[31m[FAIL]\033[0m %s\n' "$*" >&2
	FAILED=1
}

pass() {
	printf '\033[32m[PASS]\033[0m %s\n' "$*"
}

say "=========================================================="
say "🔍 [RELEASE GATE] Auditing ZQK Process File Descriptors"
say "Ceiling: ${MAX_ALLOWED_FDS} open file descriptors per process"
say "=========================================================="

get_fd_count() {
	pid="$1"
	if command -v lsof >/dev/null 2>&1; then
		lsof -p "$pid" 2>/dev/null | wc -l | tr -d ' '
	elif [ -d "/proc/$pid/fd" ]; then
		ls -1 "/proc/$pid/fd" 2>/dev/null | wc -l | tr -d ' '
	else
		echo "-1"
	fi
}

# 1. Audit all running zqk processes
say "--- Phase 1: Live Process Audit ---"
ZQK_PIDS=$(pgrep -f "bin/zqk" 2>/dev/null || true)
if [ -n "$ZQK_PIDS" ]; then
	for pid in $ZQK_PIDS; do
		# Ignore self if called via zqk
		if [ "$pid" = "$$" ]; then
			continue
		fi
		cmd_name=$(ps -p "$pid" -o command= 2>/dev/null | awk '{print $1, $2, $3}' || echo "zqk")
		fd_count=$(get_fd_count "$pid")
		if [ "$fd_count" -ge 0 ]; then
			if [ "$fd_count" -gt "$MAX_ALLOWED_FDS" ]; then
				fail "PID $pid ($cmd_name) has $fd_count open file descriptors (exceeds $MAX_ALLOWED_FDS limit!)"
			else
				pass "PID $pid ($cmd_name): $fd_count FDs (healthy)"
			fi
		fi
	done
else
	say "No existing background zqk processes detected on host."
fi

# 2. Phase 2: Active FSWatcher regression test
say "--- Phase 2: Active Ambient Daemon Regression Test ---"
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
ZQK_BIN="${ROOT}/bin/zqk"
if [ ! -x "$ZQK_BIN" ]; then
	say "Compiling bin/zqk for test..."
	go build -trimpath -o "$ZQK_BIN" "$ROOT/cmd/zqk"
fi

TEST_TMPDIR=$(mktemp -d "${TMPDIR:-/tmp}/zqk-fd-test.XXXXXX")
cleanup_test() {
	if [ -n "${TEST_PID:-}" ]; then
		kill "$TEST_PID" 2>/dev/null || true
		wait "$TEST_PID" 2>/dev/null || true
	fi
	rm -rf "$TEST_TMPDIR"
}
trap cleanup_test EXIT HUP INT TERM

ORIG_PWD=$(pwd)
cd "$ROOT"
"$ZQK_BIN" ambient daemon >/dev/null 2>&1 &
TEST_PID=$!
# Allow daemon to initialize and walk directories
sleep 2
fd_count=$(get_fd_count "$TEST_PID")
kill "$TEST_PID" 2>/dev/null || true
wait "$TEST_PID" 2>/dev/null || true
unset TEST_PID
cd "$ORIG_PWD"

if [ "$fd_count" -ge 0 ]; then
	if [ "$fd_count" -gt "$MAX_ALLOWED_FDS" ]; then
		fail "Spawned ambient daemon had $fd_count open file descriptors (exceeds $MAX_ALLOWED_FDS limit!)"
	else
		pass "Spawned ambient daemon initialized cleanly with $fd_count FDs (ceiling: $MAX_ALLOWED_FDS)"
	fi
fi

say "=========================================================="
if [ "$FAILED" -ne 0 ]; then
	say "❌ [RELEASE GATE FAILED] Process file descriptor leak detected!"
	exit 1
else
	say "✓ [RELEASE GATE PASSED] All zqk processes satisfy I/O descriptor hygiene."
fi
