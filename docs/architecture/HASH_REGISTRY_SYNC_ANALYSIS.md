# Hash Registry Sync Analysis and Prevention

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Related:** CLI Normative Path v1.0, Hash Registry Design v1.0

## Root Cause Analysis

### Incident Summary

**Date:** 2025-12-31  
**Issue:** 18 files missing from hash registry (Tier 2 warnings)  
**Files Affected:**
- 8 role files (ROL-003 through ROL-010)
- 5 account files (coder-agent, collective-team-alpha, observer-agent, senior-developer, test-agent)
- 5 audit files (AUD-10, AUD-14, AUD-17, AUD-20, AUD-23)

### Root Cause

**Primary Cause:** Files were created directly using file system operations (via `write` tool) instead of through the CLI normative path.

**What Happened:**
1. Files were created by writing YAML directly to disk
2. Hash registry was not updated (only CLI operations update the registry)
3. System check detected missing hash entries
4. Warnings accumulated until manual intervention

**Why It Happened:**
- **Tooling Gap**: No automated detection when files are created outside CLI
- **Workflow Gap**: Direct file creation is faster/easier than CLI commands
- **Detection Gap**: No pre-commit validation for hash registry sync
- **Awareness Gap**: Tooling doesn't warn about missing CLI operations

### Impact

- **Tier 2 Warnings**: 18 violations detected
- **System Health**: Degraded (warnings above threshold of 5)
- **Detection Time**: Immediate (system check detected on next run)
- **Resolution Time**: ~5 minutes (manual hash registration script)

## Prevention Mechanisms

### 1. Enhanced Pre-Commit Hook (IMMEDIATE)

**Current State:** Pre-commit hook checks for Tier 1 violations but doesn't detect unregistered files.

**Enhancement:** Add hash registry sync check to pre-commit hook.

**Implementation:**
```bash
# In tools/git-hooks/pre-commit
# Detect files created/modified outside CLI
STAGED_YAML_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.yaml$' | grep -E '^(.zqk/process/(roles|accounts|audit|backlog|criteria|requirements|tests|test_cases|policies|decisions|questions|missions|visions|roadmaps|strategic_plans|goals|milestones|workstreams|backlog_items|components|agent_architectures|agent_onboarding_preparations|kind_synonyms|scheduler_jobs|evolution_managements)/)' || true)

if [ -n "$STAGED_YAML_FILES" ] && [ -f "$REPO_ROOT/zqk" ]; then
    # Check for missing hash registry entries
    MISSING_HASHES=$("$REPO_ROOT/zqk" system check --format json 2>/dev/null | jq -r '.results[] | select(.issues[]?.category == "integrity" and .issues[]?.message | contains("No integrity hash")) | .object_id' || true)
    
    if [ -n "$MISSING_HASHES" ]; then
        echo "❌ ERROR: Files created outside CLI detected!"
        echo ""
        echo "The following files are missing from hash registry:"
        echo "$MISSING_HASHES" | while read -r obj_id; do
            echo "  - $obj_id"
        done
        echo ""
        echo "Fix: Use CLI commands to create objects:"
        echo "  zqk object create <kind> --file <file.yaml>"
        echo ""
        echo "Or register hashes manually:"
        echo "  zqk system check --auto-fix --force"
        exit 1
    fi
fi
```

### 2. CLI Helper Tool (SHORT-TERM)

**Purpose:** Make it easier to create objects via CLI, reducing temptation to create files directly.

**Implementation:** Create `scripts/create-object.sh` helper:

```bash
#!/bin/bash
# Helper script to create objects via CLI from YAML files
# Usage: ./scripts/create-object.sh <file.yaml>

FILE="$1"
if [ ! -f "$FILE" ]; then
    echo "Error: File not found: $FILE"
    exit 1
fi

# Extract kind and id from YAML
KIND=$(grep '^kind:' "$FILE" | head -1 | awk '{print $2}')
ID=$(grep '^id:' "$FILE" | head -1 | awk '{print $2}')

if [ -z "$KIND" ] || [ -z "$ID" ]; then
    echo "Error: File must contain 'kind' and 'id' fields"
    exit 1
fi

# Create object via CLI
zqk object create "$ID" --file "$FILE"
```

### 3. File System Watcher (MEDIUM-TERM)

**Purpose:** Detect when files are created/modified outside CLI and warn immediately.

**Implementation:** Background service that monitors object directories and detects:
- Files created without corresponding hash registry entry
- Files modified without hash registry update
- Files deleted without hash registry cleanup

**Action:** Log warning and suggest CLI command to fix.

### 4. CI/CD Validation (IMMEDIATE)

**Enhancement:** Add hash registry sync check to CI/CD pipeline.

**Implementation:**
```yaml
# In CI/CD pipeline
- name: Check Hash Registry Sync
  run: |
    ./zqk system check all --format json > check.json
    TIER2_COUNT=$(jq '.summary.warnings' check.json)
    if [ "$TIER2_COUNT" -gt 0 ]; then
      echo "❌ Tier 2 violations detected: $TIER2_COUNT"
      echo "Files may have been created outside CLI"
      exit 1
    fi
```

### 5. Developer Documentation (IMMEDIATE)

**Enhancement:** Add clear warnings in developer documentation about CLI-first approach.

**Implementation:** Update onboarding docs with:
- Clear examples of correct vs. incorrect workflows
- Warning about direct file creation
- Quick reference for CLI commands

### 6. Automated Hash Registration (SHORT-TERM)

**Purpose:** Auto-register hashes for files created outside CLI when safe.

**Implementation:** Enhance `system check --auto-fix` to:
- Detect files missing from hash registry
- Calculate and register hashes automatically
- Create audit event for the registration
- Warn about future use of CLI

**Safety:** Only auto-fix when:
- File is valid YAML
- File matches object spec
- No hash mismatch (file wasn't modified after creation)

### 7. CLI Command Enhancements (SHORT-TERM)

**Enhancement:** Add commands to make CLI operations easier:

```bash
# Create object from file (already exists, but enhance UX)
zqk object create --file <file.yaml>

# Batch create from directory
zqk object create --dir <directory>

# Validate file before creating
zqk object validate --file <file.yaml>
```

## Detection Mechanisms

### Current Detection

1. **System Check Command**: Detects missing hashes on demand
2. **Pre-Commit Hook**: Checks for Tier 1 violations (but not Tier 2)
3. **CI/CD**: Runs system check (but may not fail on Tier 2)

### Enhanced Detection

1. **Pre-Commit Hook**: Detect unregistered files before commit
2. **File System Watcher**: Real-time detection of direct file operations
3. **CI/CD**: Fail on Tier 2 violations (configurable threshold)
4. **Periodic Jobs**: Scheduled system checks with alerts

## Resolution Workflow

### When Files Are Created Outside CLI

1. **Detection**: System check or pre-commit hook detects missing hashes
2. **Notification**: Warning/error message with fix instructions
3. **Resolution Options**:
   - **Option A (Recommended)**: Recreate via CLI
     ```bash
     # Delete file
     rm <file.yaml>
     # Recreate via CLI
     zqk object create <id> --file <file.yaml>
     ```
   - **Option B (Quick Fix)**: Register hash manually
     ```bash
     zqk system check <object-id> --auto-fix --force
     ```
4. **Verification**: Run system check to confirm resolution

## Metrics and Monitoring

### Track

- Number of files created outside CLI per day
- Time to detection (from creation to system check)
- Time to resolution (from detection to fix)
- Frequency of direct file operations by user/tool

### Alerts

- >5 files created outside CLI in one day
- >10 Tier 2 violations (indicates systematic issue)
- Files created outside CLI in production branches

## Policy Updates

### POL-CODE-002 Enhancement

Add explicit prohibition on direct file creation:

```yaml
prohibited_operations:
  - Creating object files directly (must use CLI)
  - Renaming object files with mv/cp (must use CLI)
  - Copying object files (must use CLI)
  
enforcement:
  - Pre-commit hook blocks commits with unregistered files
  - CI/CD fails on Tier 2 violations
  - File system watcher warns on detection
```

### New Policy: POL-CODE-008

**Title:** Hash Registry Synchronization Requirements

**Requirements:**
- All object files must have registered hashes
- Hash registry must be updated atomically with file operations
- Direct file operations are prohibited
- System check must pass before commit

## Implementation Priority

### Immediate (This Week)
1. ✅ Enhanced pre-commit hook (detect unregistered files)
2. ✅ CI/CD validation (fail on Tier 2 violations)
3. ✅ Developer documentation updates

### Short-Term (This Month)
4. CLI helper tools (easier object creation)
5. Automated hash registration (auto-fix safe cases)
6. CLI command enhancements (batch operations)

### Medium-Term (Next Quarter)
7. File system watcher (real-time detection)
8. Periodic monitoring jobs (scheduled checks)
9. Enhanced metrics and alerting

## Success Criteria

- **Zero Tier 2 violations** from unregistered files
- **Pre-commit hook** blocks all direct file creations
- **CI/CD** fails on hash registry sync issues
- **Developer awareness** of CLI-first approach
- **Automated resolution** for safe cases

## Related Documentation

- [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)
- [Hash Registry Design v1.0](./hash-registry-design-v1.0.md)
- [SYNC_PREVENTION_RECOMMENDATIONS.md](../policies/SYNC_PREVENTION_RECOMMENDATIONS.md)
- [POL-CODE-002](../policies/POL-CODE-002.yaml): YAML File Integrity
- [POL-CODE-004](../policies/POL-CODE-004.yaml): System Check Violations

---

*This analysis is based on the incident of 2025-12-31 where 18 files were created outside the CLI normative path.*

