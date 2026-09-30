# Daemon Singleton Mutual Exclusion & Non-Nested Project Roots

## Executive Summary
In distributed agent workflows and autonomous continuous loop executions, multiple background processes operate against the local Knowledge Kernel. To prevent cross-project state corruption, race conditions, and duplicate background tasks, ZQK enforces two fundamental kernel invariants:
1. **Singleton Daemon Instances Per Project Root**: Exactly one instance of any daemon role (`ambient`, `privileged-writer`, `kernel steward`, `overseer`, `scheduler`, and `mcp-daemon`) may execute per project root.
2. **Non-Nested Project Roots**: Project roots (`.zqk/`) cannot be nested within another project root directory hierarchy.

---

## Architecture & Implementation

### 1. Kernel Advisory Locking (`pkg/daemon/singleton`)
Rather than relying on fragile PID files or Unix domain sockets that can race during startup or leave orphaned files after a sudden crash, daemon mutual exclusion is enforced via non-blocking POSIX kernel advisory file locks (`syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)`).

- **Lock Path**: `.zqk/state/daemon_locks/<role>.lock`
- **Ownership Lifetime**: The file descriptor is held open for the entire process duration.
- **Fail-Closed Behavior**: If another daemon instance attempts to start, `syscall.Flock` immediately fails with `EWOULDBLOCK` / `EAGAIN`. The duplicate process exits immediately with a diagnostic message without clobbering state.
- **Automatic Kernel Cleanup**: If a process terminates abnormally (e.g., SIGKILL, panic, power loss), the operating system kernel automatically releases all file locks held on the descriptor.
- **Lifecycle Helpers**:
  - `singleton.AcquireDaemonLock(projectRoot, daemonName)`
  - `singleton.Guard(projectRoot, daemonName)` returning a `defer release()` func.
  - `singleton.RunGuarded(projectRoot, daemonName, fn)` for scoped execution blocks.

### 2. Non-Nested Project Roots Invariant (`pkg/paths/project_root_nesting.go`)
Allowing nested `.zqk` project directories causes ambiguous context resolution, conflicting caches, and cross-project pollution.

- **Ancestor & Descendant Validation**: When locating or validating a project root, the system inspects directory ancestors and descendants to ensure no parent or child directory also contains `.zqk` (excluding `$HOME` user config and system temporary directories).
- **Automated Policy Enforcement**: The `ProjectNestingGate` runs as part of `zqk system check-policy project-nesting` and system integrity validations to fail closed if a nested project root is introduced.

### 3. Resource Hygiene & Telemetry Isolation
- **Telemetry Protection**: Daemon locks under `.zqk/state/daemon_locks/` are permanent singleton mutual exclusion targets and are exempt from ephemeral stale lock cleanup.
- **Active Holder Verification**: `pkg/resourcehygiene.InspectIOResources` tests active lock status via non-blocking flocks to prevent flagging actively running processes as stale.
