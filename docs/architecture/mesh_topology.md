# Dynamic Mesh Topology Configuration

To ensure ZQK can scale from local dual-agent instances up to entire fleets of TPMs and DGX-hosted local models, we need to decouple the seat logic from hardcoded assumptions. I have captured this as a formal requirement in the kernel (`REQ-1788577958898090000-36483a7c`).

Below is a proposed architectural direction for making the mesh completely configurable.

## 1. The `MeshTopology` Object (or Config)
We should introduce a declarative cluster configuration (either a new Kernel Object like `mesh_topology` or a project-level `mesh.yaml` config). This will replace the hardcoded `peer_seats.json` generation and the rigid `install-seat-workers.sh`.

```yaml
kind: mesh_topology
id: MSH-DGX-CLUSTER-001
title: DGX Spark 10x Worker Topology
seats:
  - id: tpm-primary
    persona_ref: PER-ORCH-ALPHA
    wake_mechanism: agentapi        # Woken via conversation UUID (e.g., Antigravity/Cursor)
    concurrency_limit: 1

  - id: tpm-secondary
    persona_ref: PER-ORCH-BETA
    wake_mechanism: agentapi
    concurrency_limit: 1

  - id: worker-pool-alpha
    persona_ref: PER-WORKER-ALPHA
    wake_mechanism: stamp           # Polling/watchdog driven
    instances: 5                    # Spawns 5 identical daemons
    runtime: qwen3.6-local
```

## 2. Dynamic Daemon Provisioning
Instead of `install-seat-workers.sh` looping over hardcoded IDs, we would run:
```bash
zqk system topology apply MSH-DGX-CLUSTER-001
```
This command would:
1. Parse the topology definition.
2. Generate `peer_seats.json` mappings dynamically.
3. Automatically provision launchd/systemd background services for any seat using a `stamp` (polling) wake mechanism based on the `instances` count.

## 3. Runtime/Model Abstraction
Currently, the worker daemons assume a specific Python wrapper (`seat_worker.py`). By introducing a `runtime` field, we can support pluggable adapters:
* `qwen-local`: Uses local inference endpoints.
* `dgx-spark`: Dispatches payload to a remote compute cluster.
* `headless-gemini`: A headless background process using the Gemini API.

---
> [!NOTE]
> Since we are letting the current R27 pipeline soak, we don't need to implement this immediately. However, having the architecture drafted ensures that when we are ready to scale, we have a clear path to headless fleet orchestration.
