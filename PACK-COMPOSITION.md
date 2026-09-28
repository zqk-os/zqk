# Pack Composition Architecture

This directory and repository tree adhere to the ZQK modular pack composition architecture.

For the canonical architecture guide, pack manifest format (`pack.yaml`), builder codegen (`bldr_cli_cmd_v1`), and composition root patterns, please see:

👉 [Modular Pack Composition and Extensibility Architecture Guide](docs/architecture/PACK_COMPOSITION_AND_EXTENSIBILITY.md)

## Core Architectural Invariants
- **Kernel vs. Pack Boundary**: The kernel creates and validates objects from specs. It does not import pack-specific generated instance builders.
- **Code Generation**: Code generation stays a build-time tool. Its output lives with the pack that owns the spec under `pkg/cli/bldr_cli_cmd_v1` and pack trees.
- **Composition Root**: A pack is a Go package registered from the composition root (`cmd/zqk`, or a third-party main). One binary, one Go version.
- **Enums & Types**: A kind's generated enum lives with the pack that owns the spec. Shared lifecycle status enums stay in the kernel. Kernel packages do not import pack trees.
- **Dynamic Spec Loading**: When a pack is uploaded and verified, the kernel records a formal spec so the loaded pack behaves as typed objects inside the kernel without rebuilding.
- **Product & Seat Configuration**: Product config is `config/zqk.yaml`. A seat overrides it with `config/zqk-local.yaml` (`paths.project_root`, `paths.aliases`, `cli.binary_path`, `kernel_state.project_root`). Seat lite-files (chat channel, git identity, workspace sync, feature flags, hook profile, idle store, runtime manifest) live under `.zqk/agent-runtime/`. The kernel does not read or write `zqk-settings.yaml`.
