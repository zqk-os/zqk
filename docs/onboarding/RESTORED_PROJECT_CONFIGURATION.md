# Restored Project Configuration

When restoring a ZQK project from a backup, snapshot, or fresh clone of an existing environment, the project configuration must be initialized before the kernel can operate safely.

The SSOT (Single Source of Truth) for configuring a restored project is the script located at `scripts/configure-restored-project.sh`.

## Instructions

1. **Verify State**: You can check if the project is properly configured by running:
   ```bash
   sh ./scripts/configure-restored-project.sh --check
   ```
   If the exit code is non-zero, the environment needs configuration.

2. **Apply Configuration**: Run the script without the check flag to apply the required changes:
   ```bash
   sh ./scripts/configure-restored-project.sh
   ```

3. **Idempotency Check**: After applying, verifying the state again should return an exit code of 0.

Do not attempt to recreate missing configuration files manually; always use the provided script to ensure compatibility and consistency across all instances.
