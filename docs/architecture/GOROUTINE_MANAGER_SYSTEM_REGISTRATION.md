# Goroutine Manager System Registration Guide

**Last Verified:** 2026-08-31


**Date:** 2026-01-13  
**Status:** Active  
**Purpose:** Guide for registering goroutine manager artifacts with the system

## Overview

All artifacts have been created and are ready for system registration. This document provides the commands and patterns to register them with the established system.

## Artifact Inventory

### Documentation (10 files)
All in `docs/process/architecture/`:
1. `runtime-goroutine-manager-v1.0.md`
2. `GOROUTINE_MANAGER_CAPABILITIES.md`
3. `GOROUTINE_MANAGER_IMPLEMENTATION_SUMMARY.md`
4. `goroutine-entry-points-analysis.md`
5. `goroutine-manager-integration-artifacts.md`
6. `goroutine-manager-change-verification.yaml`
7. `goroutine-manager-verification-checklist.md`
8. `GOROUTINE_MANAGER_VERIFICATION_REPORT.md`
9. `mcp-goroutine-lifecycle-v1.0.md`
10. `mcp-goroutine-leak-prevention-checklist.md`

### Code (5 files)
All in `pkg/runtime/`:
1. `goroutine_manager.go`
2. `goroutine_manager_test.go`
3. `leak_detection_test.go`
4. `helpers.go`
5. `example_mcp_integration.go`

## Registration Steps

### Step 1: Register Documentation

**Command:**
```bash
zqk automation docman-sync
```

**What it does:**
- Scans `docs/` directory for markdown files
- Extracts metadata (title, summary, status, group, category)
- Creates `doc_entry` objects for new files
- Skips files that already have entries

**Expected Result:**
- 10 new `doc_entry` objects created (DOC-XXX)
- Group: `architecture`
- Category: Determined from path

**Verification:**
```bash
zqk object list doc_entry --filter 'path=*goroutine*' --format table
```

### Step 2: Create Architecture Decision Record

**Command:**
```bash
zqk object create decision --file adr_goroutine_manager.yaml
```

**File:** payload for `zqk object create decision --file …` (CAS lands in `docs/process/decisions/`, not `docs/process/decision/`). TRACK: `DEC-CAS-GIT-PACKAGE-MUST-NOT-ORPHAN-001`.

**Content:**
```yaml
kind: decision
schema_version: "2.0.0"
id: ADR-002
title: Architecture Decision: Goroutine Lifecycle Management System
status: accepted
created_at: "2026-01-13T23:00:00Z"
created_by: account:system
updated_at: "2026-01-13T23:00:00Z"
updated_by: account:system
context: |
  After shutting down Cursor, many mcp-simple processes continue running and consuming 100% CPU,
  indicating a critical goroutine leak. The system needs OS-level process/thread tracking.
rationale: |
  Implement a GoroutineManager system integrated with the coordination framework to provide
  OS-level process/thread tracking and lifecycle management.
impact: |
  Positive: Fixes critical leak, enables observability, provides graceful shutdown.
  Negative: Requires migration of existing goroutines.
revisit: "2027-01-13T00:00:00Z"
```

**Verification:**
```bash
zqk object get ADR-002
```

### Step 3: Link Relationships

After registration, link `doc_entry` objects to ADR-002:

**Update doc_entry objects:**
```bash
# For each doc_entry related to goroutine manager, add decision_refs
zqk object update doc_entry DOC-XXX --data 'decision_refs: [ADR-002]'
```

## Established Patterns

### Pattern 1: Documentation Registration

**Established Pattern:**
- Use `zqk automation docman-sync` for all markdown files
- Automatically extracts metadata
- Creates `doc_entry` objects
- Group/category determined from path

**Our Usage:**
- ✅ All documentation files follow this pattern
- ✅ Will be registered on next `docman-sync` run

### Pattern 2: Architecture Decisions

**Established Pattern:**
- Use `decision` objects with ADR-XXX format
- Status: `accepted` for implemented decisions
- Link to related documentation via `decision_refs` in doc_entry

**Our Usage:**
- ⏳ Create ADR-002 decision object
- ⏳ Link doc_entry objects to ADR-002

### Pattern 3: Verification Artifacts

**Established Pattern:**
- Verification data stored as YAML files
- Linked to documentation via doc_entry
- Tracked via git

**Our Usage:**
- ✅ `goroutine-manager-change-verification.yaml` created
- ✅ Linked to documentation files

## Traceability

### Documentation → System Objects
```
Markdown Files
    ↓ (docman-sync)
doc_entry objects
    ↓ (decision_refs)
decision (ADR-002)
```

### Requirements → Components
```
Requirements (R-GOROUTINE-001 through 005)
    ↓ (documented in)
Architecture Docs
    ↓ (implemented in)
Code (pkg/runtime/)
```

### Components → Documentation
```
Code Files
    ↓ (documented in)
Architecture Docs
    ↓ (registered as)
doc_entry objects
```

## Maintenance

### Adding New Documentation

1. Create markdown file in `docs/process/architecture/`
2. Run `zqk automation docman-sync`
3. Verify registration: `zqk object list doc_entry --filter 'path=*new-file*'`

### Updating Documentation

1. Edit markdown file
2. `doc_entry` object automatically reflects changes (path-based)
3. Update relationships if needed: `zqk object update doc_entry DOC-XXX`

### Creating New ADRs

1. Create YAML file following ADR template
2. Run `zqk object create decision --file adr-file.yaml`
3. Link to related doc_entry objects

## Verification Commands

```bash
# List all goroutine manager documentation
zqk object list doc_entry --filter 'path=*goroutine*'

# Get ADR
zqk object get ADR-002

# Check code files
zqk system check pkg/runtime

# Verify build
go build ./pkg/runtime/...
```

## Status

- ✅ **Documentation:** Ready for registration (10 files)
- ✅ **Code:** Tracked via git (5 files)
- ⏳ **ADR-002:** Pending creation (YAML syntax needs adjustment)
- ⏳ **Relationships:** Pending linking

## Next Actions

1. **Run docman-sync** to register documentation
2. **Create ADR-002** using interactive create or fixed YAML
3. **Link relationships** between doc_entry and ADR-002
4. **Verify** all artifacts are discoverable
