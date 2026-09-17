# Tutorial: First Agent Session

Learn how to initialize a multi-agent work session in ZQK, claim tasks from the Knowledge Kernel, and run collaborative execution.

---

## 1. Onboarding Your Agent
Whenever a new AI agent connects to a ZQK repository, prime its instructions from the kernel:

```bash
./bin/zqk system agent-onboard
```
This registers default seating (`peer-agent-1`, `peer-agent-2`), detects the IDE or CLI runtime host, and binds agent governance policies.

---

## 2. Self-Discovering the Active Mission
Instead of waiting for a prompt, run:

```bash
./bin/zqk workflow whats-next
```
The kernel analyzes:
- The active Priority Plan (`PRI-*`)
- Unblocked Backlog Items (`BLI-*`)
- Test Convergence Sessions (`CVS-*`)
- Ambient Alignment Score

---

## 3. Claiming and Executing a Backlog Item
Inspect the assigned backlog item:
```bash
./bin/zqk object get BLI-xxxx
```

Verify that requirements and acceptance criteria are linked:
```bash
./bin/zqk object ref add BLI-xxxx REQ-xxxx CRIT-xxxx PER-ORCH-ALPHA
```

Promote the item to in-progress:
```bash
./bin/zqk object promote BLI-xxxx
```

---

## 4. Completing the Work
After writing code and verifying all unit tests pass:
```bash
# Verify fail-closed pre-commit gates
./scripts/pre-commit-gates.sh

# Commit with standardized message referencing the BLI
git commit -m "feat(module): implement feature (BLI-xxxx)"

# Promote the BLI to complete
./bin/zqk object promote BLI-xxxx
```
