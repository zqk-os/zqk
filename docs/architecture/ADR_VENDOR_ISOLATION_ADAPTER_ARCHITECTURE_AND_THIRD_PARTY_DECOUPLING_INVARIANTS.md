# ADR: Vendor Isolation Adapter Architecture & Third-Party Decoupling Invariants

## Status
Accepted

## Context
Prior to this architectural refactoring, host integration and MCP installer components in \`pkg/mcp/install.go\` directly referenced third-party proprietary vendor names, configuration file paths, and environment directories (such as Cursor, Windsurf, Claude Code, Cline, and Antigravity). This coupling violated core architectural isolation principles:
1. **Third-Party Product Direct Naming in Core Packages**: Vendor-specific product names and conventions must not pollute foundational runtime packages.
2. **Extensibility Bottlenecks**: Adding support for a new IDE or local developer agent required modifying core MCP installation loops and branching logic.
3. **Foreign Configuration Leakage**: Heuristics for skipping foreign or unowned configuration files were hardcoded as ad-hoc substring matches rather than modular adapter obligations.

## Architectural Decision
We establish a decoupled **Vendor Isolation Adapter Architecture** located within \`pkg/adapters/ide\`:
1. **Unified \`Adapter\` Interface**:
   - \`Name() string\`: Returns human-readable display name.
   - \`Vendor() string\`: Returns canonical vendor identifier (e.g. \`cursor\`, \`anthropic\`, \`codeium\`, \`google\`, \`cline\`, \`generic\`).
   - \`WorkspaceConfig(projectRoot string) (displayName, configPath string)\`: Resolves project-local configuration paths.
   - \`GlobalConfigs(homeDir string) map[string]string\`: Resolves host-global configuration files.
   - \`SearchDirs(homeDir, goos, appData string) []string\`: Resolves platform-specific filesystem discovery locations.
   - \`KnownFiles() map[string]string\`: Declares vendor file signatures.
   - \`IsForeign(ideName, configPath string) bool\`: Identifies foreign configuration formats to prevent destructive rewrites.
   - \`IsStdioAdapter(ideName, configPath string) bool\`: Identifies transport modes requiring stdio daemon bridging.

2. **Self-Registering Adapter Registry**:
   Each IDE/agent adapter self-registers into a global registry during package initialization:
   - \`CursorAdapter\`
   - \`ClaudeAdapter\`
   - \`ClineAdapter\`
   - \`WindsurfAdapter\`
   - \`AntigravityAdapter\`
   - \`GenericAdapter\`

3. **Core Package Decoupling**:
   \`pkg/mcp/install.go\` now delegates all filesystem discovery, foreign format inspection, and stdio transport detection directly to the \`pkg/adapters/ide\` registry, eliminating all hardcoded third-party product names and vendor-specific paths from core MCP installation routines.

## Verification & Proof
- **Static Floor Invariants**: Verified that every adapter satisfies required metadata, non-empty vendor identifiers, and strict interface adherence.
- **Operational Dynamic Proof**: Validated multi-vendor configuration resolution, workspace discovery, and stdio transport detection across mock and real filesystem layouts.
- **Negative Adversarial Boundary**: Confirmed proper rejection and skip-handling for foreign configuration paths, avoiding false positives and destructive writes to unowned tool configurations.
