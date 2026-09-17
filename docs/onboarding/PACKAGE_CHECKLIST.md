# Package Checklist

Before finalizing any community or binary package of the ZQK Kernel, ensure the following fail-closed mechanisms are intact and verified:

- [ ] **Referential Integrity**: The packaging scripts and Git pre-commit hooks MUST NOT permit the deletion of a CAS object if it still has inbound references from surviving objects. This is critical to prevent ghost references. Canonical object: `zqk object get DEC-CAS-GIT-PACKAGE-MUST-NOT-ORPHAN-001` (kind `decision`, folder `.zqk/process/decisions/` — not `.zqk/process/decision/`).

- [ ] **Cross-Platform Verification**: Binaries must compile cleanly for all target OS/Arch combos.

- [ ] **Checksums**: SHA256 checksums must be provided for all artifacts.
