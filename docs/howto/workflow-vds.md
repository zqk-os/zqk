# How-To: Configure Workflow VDS (Version-Driven State)

Learn how Version-Driven State (VDS) automates and validates lifecycle transitions across different schema versions and namespaces.

---

## What is VDS?
VDS (Version-Driven State) links the lifecycle transitions of system objects directly to their schema version and policy definitions. Rather than relying on arbitrary string status mutations, VDS verifies:
1. Valid directional hops along declared lifecycle transition graphs.
2. Mandatory field presence for target stages (e.g. `commit_hashes` required for `complete`).
3. Auditor gates and universal verification assertions (`CRIT-*` pass evidence).

---

## Verifying VDS Compliance

To inspect the VDS integrity of an object before or during promotion:

```bash
# Validate an individual object
zqk validate object BLI-xxxx

# Check composed integrity across the whole workspace
zqk system check
```

---

## Handling Stuck Promotions
If an object fails to promote via `zqk object promote`, VDS reports the precise failure reason:
- Missing `criteria_refs` or `requirement_refs` (shovel-ready gating).
- Unvalidated criteria (`status != validated`).
- Missing commit evidence (`gate.VerifyComplete` failure).

Resolve the diagnostic issue using CLI commands, then re-run `zqk object promote <id>`.
