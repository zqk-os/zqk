# Seat Worker Supervisor Subsystem (`pkg/seatworker`)

`pkg/seatworker` provides OS-level service supervision (via `launchd` on macOS and `systemd` on Linux) to run autonomous agent seats as persistent background daemons.

---

## 1. What is a Seat Worker?

In the ZQK swarm mesh, a **seat** represents an assigned role or slot (e.g. `architect`, `tpm`, `qa-lead`, `worker-1`).

While developer agents often run interactively inside IDE windows, a **Seat Worker** is an autonomous background process that runs continuously on the host operating system. It executes `zqk agent seat-worker` on a continuous polling loop, claiming eligible backlog items (`agent_task`), processing event mesh notifications, and emitting status updates to the shared feed.

---

## 2. When to Use Seat Workers

- **Continuous 24/7 Autonomy**: When you want swarm seats to monitor repos, run verification loops, or handle incoming issues without requiring an open IDE or terminal window.
- **Dedicated Worker Workstations / Servers**: On CI/CD runner nodes or dedicated swarm servers that host multiple background agent seats.
- **Resilience Against Terminal Drops**: Seat workers survive shell disconnections and reboots via native OS init supervisors (`launchd` / `systemd`).

---

## 3. Platform Supervisors

- **macOS (`launchd`)**:
  - Writes property lists to `~/Library/LaunchAgents/com.zqk.mesh.seat-worker.<seat>.plist`.
  - Manages lifecycles via `launchctl bootstrap gui/$UID` and `launchctl bootout gui/$UID`.
- **Linux (`systemd`)**:
  - Writes unit files to `~/.config/systemd/user/zqk-mesh-seat-worker@<seat>.service`.
  - Manages lifecycles via `systemctl --user enable --now` and `systemctl --user daemon-reload`.

---

## 4. Key Limitations & Considerations

1. **Worktree Isolation**: Seat workers cannot be installed on an ephemeral or secondary git worktree; they must be bound directly to the seated repository root (`.zqk/`).
2. **Resource Throttling**: Each active seat worker polls at the configured interval (`--poll-seconds`, default 30s). When deploying dozens of concurrent seats on a single machine, configure staggered polling intervals to avoid I/O lock contention.
3. **Credentials & API Quotas**: Background seat workers invoke LLM providers. Ensure environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, or local Ollama endpoints) are populated in the user environment or supervisor configuration.
