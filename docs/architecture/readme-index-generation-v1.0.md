# README Index Auto-Generation System v1.0

**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Active

All object directory README indexes are **auto-generated** to prevent stale documentation and ensure index tables always match the actual object files.

## Overview

The system automatically generates README index tables for all object directories:
- **Requirements** (`docs/architecture/requirements/`)
- **Criteria** (`docs/architecture/criteria/`)
- **Test Cases** (`docs/architecture/tests/`)
- **Milestones** (`docs/architecture/milestones/`)
- **Goals** (`docs/architecture/goals/`)
- **Decisions** (`docs/architecture/decisions/`)
- **Workstreams** (`docs/architecture/workstreams/`)
- **Backlog Items** (`docs/architecture/backlog/`)
- **Priority Plans** (`docs/architecture/priority_plans/`)

## Quick Start

### Generate All READMEs

```bash
./scripts/generate-readme-index.sh
```

### Generate Specific Type

```bash
./scripts/generate-readme-index.sh criteria
./scripts/generate-readme-index.sh test_case
./scripts/generate-readme-index.sh milestone
```

### Verify All READMEs

```bash
./scripts/verify-all-readme-indexes.sh
```

## Architecture

### Components

1. **`scripts/generate-readme-index.go`** - Generic Go generator
   - Configurable for different object types
   - Extracts ID, title, status, category/priority, description
   - Generates markdown tables with proper formatting

2. **`scripts/generate-readme-index.sh`** - Wrapper script
   - Can generate all or specific object types
   - User-friendly output

3. **`scripts/verify-all-readme-indexes.sh`** - Verification script
   - Checks all READMEs are up-to-date
   - Used in CI to prevent stale documentation
   - Ignores timestamp differences

4. **`tools/git-hooks/pre-commit-docs`** - Pre-commit hook
   - Automatically checks README freshness when object files are staged
   - Prevents committing stale documentation

## Object Type Configuration

Each object type has a configuration defining:
- **ID Pattern**: Regex pattern for matching files (e.g., `CRIT-\d+\.yaml`)
- **Title Field**: YAML field name for title
- **Status Field**: YAML field name for status
- **Description Field**: YAML field name for description
- **Table Headers**: Columns to include in the table
- **Sort Order**: How to sort objects

### Supported Object Types

| Type | ID Pattern | Table Columns |
|------|------------|---------------|
| `requirement` | `REQ-\d+\.yaml`, `REQU-\d+\.yaml` | ID, Title, Status, Description |
| `criteria` | `CRIT-\d+\.yaml` | ID, Title, Status, Category, Description |
| `test_case` | `TEST-\d+\.yaml` | ID, Title, Status, Category, Description |
| `milestone` | `MIL-\d+\.yaml` | ID, Title, Status, Priority, Description |
| `goal` | `GOAL-\d+\.yaml` | ID, Title, Status, Priority, Description |
| `decision` | `DEC-\d+\.yaml` | ID, Title, Status, Description |
| `workstream` | `WS-\d+\.yaml` | ID, Title, Status, Priority, Description |
| `backlog_item` | `BLI-\d+\.yaml`, `BACK-\d+\.yaml` | ID, Title, Status, Priority, Category, Description |
| `priority_plan` | `PRI-\d+\.yaml`, `PRIO-.*\.yaml` | ID, Title, Status, Priority, Description |

## Integration

### Pre-Commit Hook

The pre-commit hook automatically checks README freshness:

```bash
# When you stage object files
git add docs/architecture/criteria/CRIT-8191.yaml
git commit -m "Add new criteria"
# Hook runs: ✅ All README indexes are up-to-date
```

### CI Integration

Add to your CI pipeline:

```yaml
# Example GitHub Actions
- name: Verify README Indexes
  run: ./scripts/verify-all-readme-indexes.sh
```

This ensures:
- Documentation stays in sync with code
- PRs with stale documentation fail CI
- No manual documentation maintenance required

## Benefits

1. **Prevents Stale Documentation**: READMEs always reflect actual object files
2. **Reduces Manual Work**: No need to manually update tables
3. **Eliminates Errors**: No typos or missing entries
4. **CI Enforceable**: Can verify in CI that documentation is current
5. **Consistent Format**: All indexes follow the same pattern

## Manual Edits

**⚠️ Do not manually edit the READMEs!**

All READMEs are marked "AUTO-GENERATED - Do not edit manually". Any manual edits will be overwritten the next time the generator runs.

If you need to change the README format or content:
1. Edit `scripts/generate-readme-index.go`
2. Run the generator to update READMEs
3. Commit both the script change and generated READMEs

## Troubleshooting

### README is out of date

```bash
# Regenerate it
./scripts/generate-readme-index.sh <object-type>
```

### Verification fails in CI

1. Check if object files were added/modified
2. Run generator locally: `./scripts/generate-readme-index.sh`
3. Commit the updated README

### Generator errors

- Ensure Go is installed: `go version`
- Check object YAML files are valid
- Verify file paths are correct

## Adding New Object Types

To add support for a new object type:

1. Add configuration to `objectConfigs` in `generate-readme-index.go`:

```go
"new_type": {
    Kind:         "new_type",
    IDPattern:     regexp.MustCompile(`^NEW-\d+\.yaml$`),
    TitleField:    "title",
    StatusField:   "status",
    DescField:     "context",
    SectionTitle:  "New Type Index",
    TableHeaders:  []string{"ID", "Title", "Status", "Description"},
    SortByID:      true,
},
```

2. Add to `OBJECT_TYPES` array in `generate-readme-index.sh`
3. Add directory mapping in `verify-all-readme-indexes.sh`
4. Update pre-commit hook if needed

## Related Documentation

- [Scripts README](../../../scripts/README.md) - Script documentation
- [Requirements README Generation](../requirements/README-GENERATION.md) - Detailed requirements example

