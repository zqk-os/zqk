# ZQK 5-Minute Quickstart Tutorial

Welcome to ZQK (Zen Quantum Kernel) — an operating system for AI + human hybrid engineering teams. This tutorial gets you from zero to running an autonomous AI agent swarm and navigating the distributed Knowledge Kernel in under 5 minutes.

---

## Prerequisites
- **Go:** 1.26+ installed (`go version`)
- **Git:** 2.40+ installed (`git --version`)
- **Make:** GNU Make

---

## Step 1: Clone and Build the Kernel (1 minute)

```bash
git clone https://github.com/lanceman/zqk.git
cd zqk

# Compile the core CLI binary
make zqk

# (macOS ARM64 only) Codesign the binary ad-hoc:
codesign -s - -f ./bin/zqk
```

Verify that the CLI runs cleanly:
```bash
./bin/zqk --version
```

---

## Step 2: Check System Health (30 seconds)

Run the kernel validator to verify that all system data cells, storage layers, and schema specs are intact:

```bash
./bin/zqk system status
./bin/zqk system check
```
You should see:
```text
System status: healthy
Validation completed: 9200+ objects (succeeded, 0 failed)
```

---

## Step 3: Discover Active Mission & Priorities (1 minute)

ZQK uses self-discovery instead of prompting operators for instructions. Ask the kernel what needs to happen next:

```bash
./bin/zqk workflow whats-next
```
This inspects the active priority plan, checks convergence sessions, evaluates draft planes, and outputs the guiding step for the team.

---

## Step 4: Interact with Knowledge Objects (1 minute)

Everything in ZQK is a typed object governed by a state machine. Inspect active goals and requirements:

```bash
# List high-level goals
./bin/zqk object list goal

# Inspect a specific backlog item
./bin/zqk object list backlog_item --filter priority_tier=P0

# View detailed metadata for an item
./bin/zqk object get BLI-OSS-QUICKSTART-001
```

---

## Step 5: Start the Background Daemons (1.5 minutes)

ZQK includes background daemons for scheduling, MCP (Model Context Protocol) tooling, and multi-agent mesh communication:

```bash
# Install stable kernel binary:
./scripts/install-zqk-stable.sh --force

# Recycle all background daemons (scheduler, MCP, seat-workers):
./scripts/recycle-stable-daemons.sh

# Verify feed doctor health:
.zqk/bin/zqk-stable feed doctor --format json
```
You should see:
```json
{
  "feed_health": "healthy",
  "mcp_subscribers": 1,
  "peer_seats_live": 2,
  "peer_wake_live": true
}
```

---

## Congratulations! 🎉
You have a fully operational ZQK Knowledge Kernel running with live peer agent seating and MCP integration.

### Next Steps
- Read the [CLI User Manual](../manual/CLI_REFERENCE.md) for full syntax and flags.
- Check [Divio Documentation Index](../INDEX.md) for architectural deep dives and How-To guides.
- Read [CONTRIBUTING.md](../../CONTRIBUTING.md) to start building with the swarm!
