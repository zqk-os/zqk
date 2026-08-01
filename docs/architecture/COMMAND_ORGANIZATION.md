# Command Organization and Hierarchy

## Current Structure

### Top-Level Command Groups

- `object` - Object operations (CRUD, query, and management)
- `system` - System operations (health, validation, and maintenance)
- `utility` - Utility operations (version, migration, and helpers)
- `automation` - Automation and integration operations
- `callback` - Handle scheduler job callbacks
- `docman` - Documentation management
- `internal` - Manage internal and built-in objects (admin only)
- `keystore` - Keystore operations
- `mcp` - MCP server operations
- `organizational` - Organizational structure and change impact analysis
- `reports` - Generate AI metrics reports
- `scheduler` - Manage scheduler jobs and daemon
- `semantic` - Semantic operations and maturity assessment

### System Commands (Specs Created)

check, init, status, validate, sync, whoami, reminders, metrics, generate-command-builders.

### System Commands (Specs Pending)

Health & Validation: check-baseline, check-async-baseline. Code Generation: generate-builders, generate-instance-builders, generate-lifecycle-builders, generate-profile-builders, generate-routing-builders, generate-trait-builders, generate-config-builders, generate-lifecycle-id-list. Migration & Maintenance: migrate, cleanup-duplicates, orphan-cleanup-fallback, recover-cas, repair-cas-corruption, repair-yaml. Snapshots: snapshot-scenario, snapshot-expand, snapshot-verify. Audit & Metrics: aggregate-audit, aggregate-change-journal, audit-report, audit-buffer, cache-audit. Config: update-specs, detect-spec-changes, feature-flags. Other: service (subcommands), git (subcommands).

### Utility Commands (Specs Created)

version, migrate, validate-yaml, fix-hashes, fix-registration, scenario-builder.

## Proposed Organization Improvements

1. **Short-term**: Keep current flat structure but improve help organization
2. **Medium-term**: Consider grouping code generation commands under `system generate`
3. **Long-term**: Evaluate if subcommand groups improve discoverability

## Spec File Organization

Command specs are validated by `./scripts/validate_command_specs.sh`. See [COMMAND_SPEC_COVERAGE.md](./COMMAND_SPEC_COVERAGE.md) for coverage and spec layout.
