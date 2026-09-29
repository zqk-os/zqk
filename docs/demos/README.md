# ZQK Core Interactive Demonstrations & Visual Showcase

> **Principle:** Agents manage the work. ZQK enforces the physics.

The demonstrations in `scripts/demos/` showcase the core architectural guarantees of the **Zen Quantum Kernel (ZQK)**. Unlike conventional AI coding tools where agent memory is volatile, execution state is lost on crashes, and security is left to LLM prompt compliance, ZQK operates as a **durable, cellular operating system and knowledge kernel**.

Every demo in this suite performs **100% authentic operations** against the live kernel, with zero simulated mocks.

---

## Quickstart: Running the Demonstrations

```bash
# Run the interactive menu (select 1-5, or run all)
./scripts/demos/run-all-demos.sh

# Run all 5 demos in non-interactive batch mode
./scripts/demos/run-all-demos.sh --batch

# Run all 5 demos and capture fresh high-fidelity SVG screenshots
./scripts/demos/run-all-demos.sh --batch --capture

# Run a specific demo directly
./scripts/demos/day_zero_dx_demo.sh
./scripts/demos/time_travel_demo.sh
./scripts/demos/containment_breach_demo.sh
./scripts/demos/crypto_audit_demo.sh
./scripts/demos/object_inspector_policy_studio_demo.sh
```

---

## Demo 1: Day-0 Greenfield DX & Instant Agent Orientation

**Thesis:** Infrastructure should feel effortless. A developer or agent swarm must be able to initialize a brand-new project and achieve full knowledge-kernel governance and autonomous orientation in seconds.

### Key Capabilities Demonstrated:
- **Zero-Config Bootstrap:** `zqk system init --project-name <name>` in an isolated workspace.
- **Agent Host Detection & Seating:** `zqk system agent-onboard` detects the IDE (Cursor, VS Code, Windsurf, Cline) and seeds vendor-neutral seating directives.
- **Instant CAS Object Minting:** Questions, goals, and tasks are immediately indexed into Content-Addressable Storage (CAS).
- **Autonomous Next-Action Discovery:** Agents run `zqk workflow whats-next` to discover their orientation without asking a human "what should I do?".
- **Zero-Defect System Health:** `zqk system check` validates the CAS membrane, process locks, and storage integrity out-of-the-box in < 3 seconds.

![Demo 1: Day-0 Greenfield DX](screenshots/demo1_dayzero_dx.svg)

---

## Demo 2: The "Kill -9" Resurrection & Crash Consistency

**Thesis:** Most autonomous AI agents keep state in volatile RAM. When killed, context dies. ZQK treats agent state as a durable, transaction-isolated cellular microkernel.

### Key Capabilities Demonstrated:
- **Durable CAS Persistence:** Graph mutations and seat credentials are written to durable storage before acknowledgment.
- **Unannounced SIGKILL (kill -9):** A host process holding an active write lock is forcefully terminated mid-flight with `kill -9`.
- **Autonomous Self-Remedy:** `zqk system check --auto-remedy` autonomously detects abandoned PIDs and stale locks, cleans them, and validates all SHA-256 CAS object leaves.
- **Zero Data Loss:** All state, keystore credentials, and plan items resume with 100% integrity without requiring re-prompting.

![Demo 2: Kill -9 Resurrection](screenshots/demo2_kill9_resurrection.svg)

---

## Demo 3: Dynamic Membrane & Fail-Closed Prompt Injection Intercept

**Thesis:** Holon as Boundaries. An LLM cannot be trusted to self-regulate its host machine access. In ZQK, the kernel—not the model—enforces the physics.

### Key Capabilities Demonstrated:
- **Adversarial Bait Scenario:** An agent is instructed via a hidden prompt injection to exfiltrate host secrets (`/etc/passwd`, `~/.ssh/id_rsa`) or write outside the project root (`../../../../tmp/pwned.txt`).
- **Live MCP Server Connection:** Real JSON-RPC client session connected to `zqk mcp serve`.
- **Fail-Closed Membrane Intercept:** The kernel intercepts the path traversal before filesystem access, returning:
  `✖ Error: access denied: path escapes sandbox root: path escapes project root`
- **Invariant Verification:** Automated test suite verifies 11 path-traversal attack vectors fail closed.

![Demo 3: Dynamic Membrane Intercept](screenshots/demo3_containment_breach.svg)

---

## Demo 4: Verifiable Cryptographic Ledger & CISA OpenVEX Attestation

**Thesis:** Anyone can generate code. ZQK generates cryptographically verifiable proof of how the code was made. You don't have to trust the LLM; you can mathematically verify the exact constraint and seating path taken.

### Key Capabilities Demonstrated:
- **Multi-Agent Seating Issuance:** `zqk keystore issue` assigns cryptographic API credentials, seating tokens, and SHA-256 fingerprints to distinct agent roles (e.g. `Swarm-Architect`, `Swarm-Security-Reviewer`).
- **Protected Vault Keystore:** Agent credentials and cryptographic hashes are stored in the isolated kernel vault.
- **CISA OpenVEX v0.2.0 Attestation:** Automatically generates signed, machine-readable vulnerability and supply-chain attestations.
- **Independent Verification:** Cryptographic verification gate confirms OpenVEX schema conformance and tamper evidence.

![Demo 4: Cryptographic Ledger & OpenVEX](screenshots/demo4_crypto_audit.svg)

---

## Demo 5: Interactive Object Inspector, Live Policy Studio & Unified QA

**Thesis:** Managing complex knowledge kernel graphs should not require memorizing dozens of CLI flags or parsing walls of raw YAML. Humans get clean, interactive drill-downs and policy studios; autonomous AI agents receive dense, token-optimized semantic projections.

### Key Capabilities Demonstrated:
- **Dual Human/Agent Projections:** Token-optimized JSON (`-f json`) for LLM context windows, and structured Terminal Design System (TDS) cards for human operators.
- **Modular Cards:** CAS storage profiles (hash, size, permissions), ontology traits, and unbroken lineage radar.
- **Live Policy Rule Studio:** Real-time DSL dry-run evaluation (`--policy-studio`) checking governance rules against live objects.
- **Unified Mission Control QA:** `zqk test dashboard --check-dod` verifies Definition of Done (DoD) compliance and test-to-criteria lineage across the entire graph.

![Demo 5: Object Inspector & Policy Studio](screenshots/demo5_object_inspector.svg)
