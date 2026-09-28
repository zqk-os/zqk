# Pack composition workspace

This checkout is the kernel side of the pack-composition effort.

Folder: `zqk-pack-composition`. Sibling studio: `../zqke-pack-composition`.

Public `github.com/zqk-os/zqk` stays the beta. `main` here tracks `origin/main`. Stay current with `git pull origin main`, then merge `main` into the working branch.

This tree is for:

- The kernel creates and validates objects from specs. It does not import generated instance builders.
- Code generation stays a tool. Its output lives with the pack that owns the spec.
- A pack is a Go package registered from the composition root (`cmd/zqk`, or a third-party main). One binary, one Go version.
- A kind's generated enum lives with the pack that owns the spec. Shared lifecycle status enums stay in the kernel. Kernel packages do not import pack trees.
- When a pack is uploaded and verified, the kernel records a formal spec so the loaded pack behaves as typed objects inside the kernel.
- Product config is `config/zqk.yaml`. A seat overrides it with `config/zqk-local.yaml` (`paths.project_root`, `paths.aliases`, `cli.binary_path`, `kernel_state.project_root`). Seat lite-files (chat channel, git identity, workspace sync, feature flags, hook profile, idle store, runtime manifest) live under `.zqk/agent-runtime/`. The kernel does not read or write `zqk-settings.yaml`.

Do not push this work onto `zqk-os/zqk` main.
