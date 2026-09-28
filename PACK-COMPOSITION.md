# Pack Composition Architecture

This directory and repository tree adhere to the ZQK modular pack composition architecture.

For the canonical architecture guide, pack manifest format (`pack.yaml`), builder codegen (`bldr_cli_cmd_v1`), and composition root patterns, please see:

👉 [Modular Pack Composition and Extensibility Architecture Guide](docs/architecture/PACK_COMPOSITION_AND_EXTENSIBILITY.md)

## Summary of Tenets
- **Kernel vs. Pack Boundary**: The kernel validates and persists objects from specs. It does not import pack-specific generated instance builders.
- **Code Generation**: Code generation toolchains produce Go builders under `pkg/cli/bldr_cli_cmd_v1` and pack packages.
- **Composition Root**: Packs are linked at the single static binary entry point (`cmd/zqk`).
- **Dynamic Spec Loading**: Verified packs register formal specs in the Knowledge Kernel graph, ensuring runtime typed behavior and integrity enforcement across all workflows.
