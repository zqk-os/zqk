# Comprehensive Resolver Architecture Design

**Last Verified:** 2026-08-31


**Created:** 2026-01-12  
**Status:** Design  
**Purpose:** Establish comprehensive resolver system for all system integrity scenarios

## Executive Summary

This document designs a comprehensive resolver architecture that can automatically resolve system integrity violations while maintaining safety, auditability, and user awareness. The design addresses:

1. **All integrity scenarios** that need resolution
2. **Safety implications** of automatic vs. manual resolution
3. **Tracking and analytics** for agents bypassing CLI
4. **User confirmation** requirements for different violation types
5. **Integration** with existing auto-fix infrastructure

## Problem Statement

Currently, the system has:
- ✅ Detection of integrity violations (system check)
- ✅ Basic auto-fix for hash mismatches and missing fields
- ⚠️ Partial resolver infrastructure (ViolationResolver exists but is mostly stubs)
- ❌ No comprehensive resolver system for all integrity scenarios
- ❌ No tracking of which agents/users bypass CLI and introduce violations
- ❌ No confidence-based resolution strategy

## Integrity Scenarios Requiring Resolvers

Based on system check output and documentation, the following scenarios need resolver support:

### 1. Hash Integrity Violations

#### 1.1 Hash Mismatch (CAS Files)
- **Detection:** Tier 1 (Blocking)
- **Cause:** File content changed but filename hash unchanged (CAS files)
- **Current Resolution:** `fixImmutableObjectHash` via `storage.Update()`
- **Resolver Strategy:** 
  - ✅ Already implemented (via `fixImmutableObjectHash`)
  - Requires `--force` flag (creates audit event)
  - Safe to auto-resolve if `--force` is provided

#### 1.2 Hash Mismatch (Non-CAS Files)
- **Detection:** Tier 1 (Blocking)
- **Cause:** File content changed but hash registry not updated
- **Current Resolution:** `updateFileHash` via `PerformCacheFreshnessCheck`
- **Resolver Strategy:**
  - ✅ Already implemented (via cache refresh operations)
  - Safe to auto-resolve (non-destructive, just updates registry)

#### 1.3 Missing Hash
- **Detection:** Tier 2/3 (Warning/Informational, escalates by file age)
- **Cause:** File created outside CLI
- **Current Resolution:** Auto-fix adds hash to registry
- **Resolver Strategy:**
  - ✅ Already implemented (via auto-fix)
  - Safe to auto-resolve (non-destructive)
  - **Tracking Opportunity:** Track which files lack hashes (indicates CLI bypass)

### 2. Reference Integrity Violations

#### 2.1 Missing Reference Target
- **Detection:** Tier 1 (Blocking)
- **Cause:** Referenced object doesn't exist
- **Current Resolution:** Manual (create object or remove reference)
- **Resolver Strategy:**
  - **Option A:** Remove/clear invalid reference (safe, but loses data)
  - **Option B:** Suggest creating missing object (requires user decision)
  - **Option C:** Use FieldResolver to find similar objects (smart matching)
  - **Recommendation:** Option C with user confirmation for high-confidence matches

#### 2.2 Reference Kind Mismatch
- **Detection:** Tier 2 (Warning)
- **Cause:** Reference ID suggests one kind, but object is different kind
- **Current Resolution:** Manual (use explicit kind notation or fix object)
- **Resolver Strategy:**
  - Use explicit kind notation in reference (e.g., `goal:GOAL-001`)
  - Safe to auto-resolve if we can determine correct kind from context
  - Low risk (just adds kind prefix)

#### 2.3 Stale Cache Reference
- **Detection:** Tier 2 (Warning)
- **Cause:** Cache miss, object exists but not in cache
- **Current Resolution:** `--refresh-cache`
- **Resolver Strategy:**
  - ✅ Already handled (cache refresh)
  - Safe to auto-resolve
  - **Tracking Opportunity:** Track cache misses by directory/file (indicates bulk operations outside CLI)

### 3. Lifecycle Violations

#### 3.1 Invalid Lifecycle Status
- **Detection:** Tier 1 (Blocking)
- **Cause:** Status value not valid for object kind
- **Current Resolution:** Manual (update to valid status)
- **Resolver Strategy:**
  - Load lifecycle definition
  - Suggest valid initial status (if object has no status)
  - Suggest most similar valid status (if invalid status provided)
  - **Safety:** Medium risk - changing status may have side effects
  - **Recommendation:** Suggest fix, require user confirmation

#### 3.2 Missing Status Field
- **Detection:** Tier 2 (Warning)
- **Cause:** Required status field is missing
- **Current Resolution:** Manual (add status with initial value)
- **Resolver Strategy:**
  - Load lifecycle definition
  - Add initial status (status with `initial: true`)
  - Safe to auto-resolve (non-destructive, adds required field)

#### 3.3 Lifecycle Precondition Violations
- **Detection:** Tier 1 (Blocking)
- **Cause:** Required references missing (e.g., `priority_plan_ref`, `milestone_refs`, `goal_refs`)
- **Current Resolution:** Manual (link to appropriate objects)
- **Resolver Strategy:**
  - Use FieldResolver with contextual hints (category, tags, status, title)
  - If unique match found: auto-link (high confidence)
  - If multiple matches: suggest top candidates (medium confidence)
  - If no matches: suggest creation command (low confidence)
  - **Safety:** Medium-High risk - linking wrong objects corrupts relationships
  - **Recommendation:** Auto-resolve only for unique matches with confidence > 0.9

### 4. Instance Validation Violations

#### 4.1 Missing Required Fields
- **Detection:** Tier 1 or 2 (depending on field)
- **Cause:** Required field not present
- **Current Resolution:** Partial (SpecBasedAutoFixer handles defaults)
- **Resolver Strategy:**
  - ✅ Partially implemented (default values from spec)
  - Extend to handle all required fields
  - Safe to auto-resolve if default exists

#### 4.2 Invalid Field Values
- **Detection:** Tier 2 (Warning)
- **Cause:** Field value doesn't match type/enum/pattern
- **Current Resolution:** Manual (fix value)
- **Resolver Strategy:**
  - Type coercion (string → int, etc.)
  - Enum: use first valid value
  - Pattern: suggest correction
  - **Safety:** Medium risk - coercion may lose precision
  - **Recommendation:** Suggest fix, require user confirmation for coercions

### 5. Registration Violations

#### 5.1 Missing ID Field
- **Detection:** Tier 1 (Blocking)
- **Cause:** Object file missing required 'id' field
- **Current Resolution:** Manual (recreate object with ID)
- **Resolver Strategy:**
  - Cannot auto-resolve (ID must be unique and meaningful)
  - Must suggest manual recreation
  - **Tracking Opportunity:** Track files missing IDs (strong indicator of CLI bypass)

#### 5.2 Missing Kind Field
- **Detection:** Tier 1 (Blocking)
- **Cause:** Object file missing required 'kind' field
- **Current Resolution:** Manual (recreate object with kind)
- **Resolver Strategy:**
  - Infer kind from file location/directory
  - If inference possible: auto-add kind (high confidence)
  - If inference ambiguous: suggest (low confidence)
  - Safe to auto-resolve if inference is unambiguous

#### 5.3 ID Format Violation
- **Detection:** Tier 2 (Warning)
- **Cause:** ID doesn't match kind pattern (e.g., INVALID-001 for backlog_item)
- **Current Resolution:** Manual (recreate with correct ID)
- **Resolver Strategy:**
  - Cannot auto-resolve (would require renaming file and updating all references)
  - Must suggest manual recreation
  - **Tracking Opportunity:** Track ID format violations (indicates manual file creation)

#### 5.4 Kind Mismatch
- **Detection:** Tier 2 (Warning)
- **Cause:** Kind field doesn't match file location
- **Current Resolution:** Manual (move file or fix kind)
- **Resolver Strategy:**
  - Prefer fixing kind field (simpler)
  - Only move file if kind field is correct and location is wrong
  - Safe to auto-resolve (just updates field or moves file)

#### 5.5 YAML Parse Error
- **Detection:** Tier 1 (Blocking)
- **Cause:** Malformed YAML syntax
- **Current Resolution:** Manual (fix YAML syntax)
- **Resolver Strategy:**
  - Cannot reliably auto-fix (syntax errors vary widely)
  - Must suggest manual fix
  - **Tracking Opportunity:** Track parse errors by file (indicates manual editing)

### 6. Policy Violations

#### 6.1 Policy Compliance Issues
- **Detection:** Tier 2/3 (Warning/Informational)
- **Cause:** Object violates policy rules
- **Current Resolution:** Manual (varies by policy)
- **Resolver Strategy:**
  - Policy-specific resolution logic needed
  - Generally cannot auto-resolve (policies express business rules)
  - Suggest manual fixes based on policy

### 7. Spec Checklist Violations

#### 7.1 Missing Spec Checklist Items
- **Detection:** Tier 1/2 (depending on item)
- **Cause:** Spec missing required checklist fields (field_profile_code, purpose, etc.)
- **Current Resolution:** Manual (add to spec)
- **Resolver Strategy:**
  - Cannot auto-resolve (requires domain knowledge)
  - Must suggest manual fix

## Resolver Architecture

### Design Principles

1. **Safety First:** Never auto-resolve changes that could cause data loss or corruption
2. **Confidence-Based:** Only auto-resolve when confidence > threshold (varies by scenario)
3. **Auditability:** All resolutions must be auditable (audit events, change journal)
4. **Transparency:** Users must understand what was resolved and why
5. **Tracking:** Track patterns of violations to identify systemic issues
6. **User Control:** Allow users to review/approve before resolution when appropriate

### Resolver Categories

#### Category 1: Safe Auto-Resolvable (No User Confirmation)
- Missing hash (non-CAS)
- Missing status (use initial status)
- Missing kind (infer from location)
- Cache staleness
- Type coercion (with validation)

#### Category 2: Conditional Auto-Resolvable (Confidence-Based)
- Hash mismatches (requires --force flag)
- Reference linking (only if unique match with high confidence)
- Kind mismatch (fix field, not move file)

#### Category 3: Suggest-Only (Requires User Decision)
- Missing reference targets (suggest create/link/remove)
- Invalid status (suggest valid status)
- ID format violations (suggest recreation)
- YAML parse errors (suggest fix)
- Policy violations (policy-specific suggestions)

### Resolver Interface

```go
// Resolver interface for all integrity resolvers
type IntegrityResolver interface {
    // Resolve attempts to resolve an issue
    // Returns resolution result with action taken
    Resolve(ctx *ResolutionContext, issue Issue, obj *ParsedObject) (*ResolutionResult, error)
    
    // CanAutoResolve returns whether this resolver can auto-resolve this issue
    CanAutoResolve(issue Issue, obj *ParsedObject) (bool, float64) // (canResolve, confidence)
    
    // GetResolutionStrategy returns the recommended resolution strategy
    GetResolutionStrategy(issue Issue) ResolutionStrategy
}

// ResolutionContext provides context for resolution
type ResolutionContext struct {
    ProjectRoot      string
    StorageProvider  storage.ObjectStorageProvider
    SpecLoader       *objects.SpecLoader
    LifecycleLoader  *objects.LifecycleLoader
    FieldResolver    *FieldResolver
    SecurityContext  *pkgctx.SecurityContext
    Logger           logging.Logger
    UserConfirmation bool // Whether user has confirmed resolution
    ForceMode        bool // Whether --force flag is set
}

// ResolutionResult represents the result of resolution attempt
type ResolutionResult struct {
    Resolved     bool                 // Whether resolution was successful
    Action       ResolutionAction     // What action was taken
    Confidence   float64              // Confidence level (0.0-1.0)
    Message      string               // Human-readable message
    AuditEvent   *AuditEventData      // Audit event data (if applicable)
    RequiresUser bool                 // Whether user confirmation is required
    Suggestions  []ResolutionSuggestion // Alternative resolutions if not resolved
}

// ResolutionAction describes what action was taken
type ResolutionAction string

const (
    ActionNone          ResolutionAction = "none"           // No action taken
    ActionFixed         ResolutionAction = "fixed"          // Issue was resolved
    ActionSuggested     ResolutionAction = "suggested"      // Fix was suggested
    ActionRequiresUser  ResolutionAction = "requires_user"  // User decision needed
    ActionCannotResolve ResolutionAction = "cannot_resolve" // Cannot be resolved automatically
)

// ResolutionStrategy defines how to resolve an issue
type ResolutionStrategy struct {
    Type              StrategyType     // Auto, Suggest, RequireUser
    Confidence        float64          // Confidence level
    SafetyLevel       SafetyLevel      // Safety assessment
    TrackingCategory  string           // Category for analytics tracking
    UserConfirmation  bool             // Whether user confirmation is required
}

type StrategyType string

const (
    StrategyAuto        StrategyType = "auto"         // Auto-resolve
    StrategySuggest     StrategyType = "suggest"      // Suggest fix
    StrategyRequireUser StrategyType = "require_user" // Require user decision
    StrategyCannotFix   StrategyType = "cannot_fix"   // Cannot be fixed automatically
)

type SafetyLevel string

const (
    SafetySafe      SafetyLevel = "safe"      // No risk of data loss
    SafetyLow       SafetyLevel = "low"       // Low risk
    SafetyMedium    SafetyLevel = "medium"    // Medium risk
    SafetyHigh      SafetyLevel = "high"      // High risk
    SafetyDestructive SafetyLevel = "destructive" // Destructive operation
)
```

### Resolver Registry

```go
// ResolverRegistry manages all integrity resolvers
type ResolverRegistry struct {
    resolvers map[string]IntegrityResolver // category -> resolver
    mu        sync.RWMutex
}

// RegisterResolver registers a resolver for a category
func (rr *ResolverRegistry) RegisterResolver(category string, resolver IntegrityResolver)

// GetResolver returns the resolver for a category
func (rr *ResolverRegistry) GetResolver(category string) (IntegrityResolver, bool)

// ResolveIssue attempts to resolve an issue using the appropriate resolver
func (rr *ResolverRegistry) ResolveIssue(ctx *ResolutionContext, issue Issue, obj *ParsedObject) (*ResolutionResult, error)
```

### Resolver Implementations

#### 1. HashIntegrityResolver
- Handles: Hash mismatches, missing hashes
- Safety: Safe (just updates registries/files)
- Auto-resolve: Yes (with --force for mismatches)
- Tracking: Track which files had hash issues

#### 2. ReferenceIntegrityResolver
- Handles: Missing references, kind mismatches, stale cache
- Safety: Medium (linking wrong objects is harmful)
- Auto-resolve: Conditional (only unique matches with high confidence)
- Tracking: Track reference violations by type and target kind

#### 3. LifecycleResolver
- Handles: Invalid status, missing status, precondition violations
- Safety: Medium-High (status changes have side effects)
- Auto-resolve: Conditional (missing status = yes, invalid status = suggest)
- Tracking: Track lifecycle violations by kind and status

#### 4. InstanceValidationResolver
- Handles: Missing required fields, invalid values, type mismatches
- Safety: Low-Medium (depends on fix type)
- Auto-resolve: Conditional (defaults = yes, coercions = suggest)
- Tracking: Track validation violations by field and rule

#### 5. RegistrationResolver
- Handles: Missing ID/kind, ID format violations, kind mismatches, YAML errors
- Safety: Varies (kind inference = safe, ID format = cannot fix)
- Auto-resolve: Conditional (kind inference = yes, others = suggest)
- Tracking: Track registration violations (strong CLI bypass indicator)

#### 6. PolicyResolver
- Handles: Policy compliance violations
- Safety: High (policies express business rules)
- Auto-resolve: No (suggest only)
- Tracking: Track policy violations by policy ID

## Tracking and Analytics

### Violation Tracking Metadata

Each violation should be tracked with metadata to identify patterns:

```go
type ViolationMetadata struct {
    // Detection context
    DetectedAt       time.Time
    DetectedBy       string              // "system_check", "async_validator", etc.
    
    // Object context
    ObjectID         string
    ObjectKind       string
    FilePath         string
    FileMTime        time.Time           // When file was last modified
    
    // Violation context
    Category         string              // "integrity", "reference", etc.
    Tier             int                 // 1, 2, 3, 4
    Message          string
    AutoFixable      bool
    
    // Resolution context (if resolved)
    ResolvedAt       *time.Time
    ResolvedBy       string              // "auto_fix", "user_action", etc.
    ResolutionAction ResolutionAction
    ResolutionConfidence float64
    
    // Agent/User tracking (if available)
    LastModifiedBy   string              // From object metadata (updated_by, created_by)
    LastModifiedAt   time.Time           // From object metadata
    FileCreatedBy    string              // From filesystem (if available)
    
    // Pattern detection
    IsCLIBypass      bool                // Inferred: file modified but no hash update
    BypassIndicator  string              // "missing_hash", "hash_mismatch", "missing_id", etc.
}
```

### CLI Bypass Detection

Track indicators that suggest CLI was bypassed:

1. **Missing Hash:** File exists but no hash in registry
   - Strong indicator: File created outside CLI
   - Tracking: Count by directory, file age, object kind

2. **Hash Mismatch:** File content changed but hash not updated
   - Strong indicator: File edited outside CLI
   - Tracking: Count by object kind, time since last CLI operation

3. **Missing ID/Kind:** File exists but missing required fields
   - Strong indicator: Manual file creation
   - Tracking: Count by directory, object kind

4. **YAML Parse Errors:** Malformed YAML syntax
   - Indicator: Manual editing with syntax errors
   - Tracking: Count by error type, object kind

5. **ID Format Violations:** ID doesn't match pattern
   - Indicator: Manual file creation with incorrect ID
   - Tracking: Count by expected vs. actual pattern

### Agent/User Statistics

Track which agents/users are introducing violations:

```go
type AgentViolationStats struct {
    AgentID              string    // From object metadata (created_by, updated_by)
    TotalViolations      int
    ViolationsByCategory map[string]int
    ViolationsByTier     map[int]int
    CLIByPassCount       int       // Count of violations indicating CLI bypass
    LastViolationAt      time.Time
    MostCommonViolation  string
    BypassPatterns       []BypassPattern // Patterns indicating CLI bypass
}

type UserViolationStats struct {
    UserID               string
    TotalViolations      int
    ViolationsByAgent    map[string]int // Agent -> violation count
    CLIByPassCount       int
    LastViolationAt      time.Time
    Agents               []string  // Agents associated with this user
}

type BypassPattern struct {
    PatternType    string    // "missing_hash", "hash_mismatch", "missing_id", "yaml_error"
    Count          int
    FirstSeen      time.Time
    LastSeen       time.Time
    ObjectKinds    []string  // Kinds affected
    Directories    []string  // Directories affected
}
```

### CLI Bypass Detection Strategy

**Problem:** Files created/modified outside the CLI don't have `created_by`/`updated_by` metadata, making agent attribution difficult.

**Solution:** Multi-layered inference strategy:

1. **Primary Attribution (High Confidence):**
   - Use `created_by`/`updated_by` from object metadata when available
   - Use SecurityContext from command execution context
   - Use git user info for commits (if available)

2. **Secondary Attribution (Medium Confidence):**
   - Correlate file mtime with audit events (find audit events around same time)
   - Use filesystem owner (if available and meaningful)
   - Use directory patterns (some directories may be agent-specific)

3. **Tertiary Attribution (Low Confidence):**
   - Pattern matching (file naming patterns, content patterns)
   - Temporal clustering (violations occurring in time windows)
   - Statistical inference (violation frequency patterns)

4. **Unknown Attribution (No Confidence):**
   - Mark as "unknown" or "system" when attribution cannot be determined
   - Still track patterns but don't attribute to specific agent/user
   - Use for aggregate statistics only

**Tracking Metadata Enhancement:**

```go
type ViolationMetadata struct {
    // ... existing fields ...
    
    // Agent/User Attribution (with confidence)
    AttributedAgent       string    // Agent ID (if determinable)
    AttributedUser        string    // User ID (if determinable)
    AttributionConfidence float64   // 0.0-1.0
    AttributionMethod     string    // "metadata", "audit_event", "filesystem", "pattern", "unknown"
    
    // CLI Bypass Indicators (for analytics)
    HasMetadataFields     bool      // Object has created_by/updated_by
    HasAuditEvent         bool      // Found audit event around mtime
    FileSystemOwner       string    // Filesystem owner (if available)
    TimeSinceLastCLIOp    time.Duration // Time since last known CLI operation on this file
    
    // Pattern Detection
    ViolationPattern      string    // Pattern category (e.g., "bulk_edit", "manual_creation")
    IsCLIBypass           bool      // Inferred: file modified but no hash update
    BypassIndicator       string    // "missing_hash", "hash_mismatch", "missing_id", etc.
}
```

### Tracking Storage

Store violation tracking data:

1. **Audit Events:** Create audit events for violations (existing)
   - Include metadata: violation type, category, tier
   - Include context: file path, object ID, detection time
   - Include agent/user: from object metadata

2. **Violation Registry:** New tracking system
   - Store violation metadata in `.zqk/violations/`
   - Track patterns over time
   - Enable analytics queries

3. **Metrics:** Aggregate violation stats
   - Count violations by category, tier, agent, user
   - Track CLI bypass indicators
   - Generate reports for system health

## Resolution Safety Matrix

| Scenario | Auto-Resolve | Confidence Threshold | User Confirmation | Safety Level |
|----------|--------------|---------------------|-------------------|--------------|
| Missing hash (non-CAS) | Yes | N/A | No | Safe |
| Missing hash (CAS) | Yes (via Update) | N/A | No (with --force) | Safe |
| Hash mismatch (non-CAS) | Yes | N/A | No | Safe |
| Hash mismatch (CAS) | Yes (via Update) | N/A | No (with --force) | Safe |
| Missing status | Yes | N/A | No | Safe |
| Invalid status | Suggest | N/A | Yes | Medium |
| Missing kind (inferrable) | Yes | 0.95 | No | Safe |
| Missing ID | No | N/A | N/A | Cannot fix |
| ID format violation | No | N/A | N/A | Cannot fix |
| Missing reference (unique match) | Yes | 0.9 | No | Medium |
| Missing reference (multiple matches) | Suggest | 0.7-0.9 | Yes | Medium |
| Missing reference (no matches) | Suggest | < 0.7 | Yes | Low |
| Reference kind mismatch | Yes | N/A | No | Low |
| Missing required field (has default) | Yes | N/A | No | Safe |
| Missing required field (no default) | Suggest | N/A | Yes | Medium |
| Type mismatch (coercible) | Suggest | N/A | Yes | Medium |
| YAML parse error | No | N/A | N/A | Cannot fix |
| Policy violation | Suggest | N/A | Yes | High |

## Integration Points

### 1. System Check Integration

```go
// In check_object_helpers.go or check_impl.go

func performAllChecks(checkCtx *CheckObjectContext, objMap map[string]interface{}) CheckResult {
    result := CheckResult{
        ObjectID:   checkCtx.Obj.ID,
        ObjectKind: checkCtx.Kind,
        FilePath:   checkCtx.FilePath,
        Issues:     []Issue{},
    }
    
    // ... existing checks ...
    
    // After collecting all issues:
    // If --auto-resolve flag is set, attempt resolution
    if shouldAutoResolve(checkCtx.Cmd) {
        resolverRegistry := GetResolverRegistry()
        resolutionCtx := buildResolutionContext(checkCtx)
        
        resolvedIssues := []Issue{}
        for _, issue := range result.Issues {
            resolution, err := resolverRegistry.ResolveIssue(resolutionCtx, issue, checkCtx.Obj)
            if err != nil {
                // Log error, keep issue
                resolvedIssues = append(resolvedIssues, issue)
                continue
            }
            
            if resolution.Resolved && resolution.Action == ActionFixed {
                // Issue was resolved, don't include in results
                // Add to AutoFixed list
                result.AutoFixed = append(result.AutoFixed, issue.Message)
                
                // Create tracking record
                trackViolationResolution(checkCtx, issue, resolution)
            } else if resolution.RequiresUser {
                // Keep issue, but add resolution suggestions
                issue.ResolutionSuggestions = resolution.Suggestions
                resolvedIssues = append(resolvedIssues, issue)
            } else {
                // Cannot resolve or low confidence, keep issue
                resolvedIssues = append(resolvedIssues, issue)
            }
        }
        
        result.Issues = resolvedIssues
    }
    
    return result
}
```

### 2. Cache Refresh Integration

Already implemented - `PerformCacheFreshnessCheck` handles:
- Hash registry updates for modified files
- CAS hash mismatch fixes (via storage.Update)

### 3. Auto-Fix Integration

Extend existing auto-fix to use resolvers:

```go
// In auto_fix_helpers.go

func autoFixIssues(fixCtx *AutoFixContext, issues []Issue) []string {
    resolverRegistry := GetResolverRegistry()
    resolutionCtx := buildResolutionContextFromFixContext(fixCtx)
    
    autoFixed := []string{}
    
    for _, issue := range issues {
        if !issue.AutoFixable {
            continue
        }
        
        resolution, err := resolverRegistry.ResolveIssue(resolutionCtx, issue, fixCtx.Obj)
        if err != nil {
            fixCtx.Logger.Warn("Failed to resolve issue", logging.Error(err))
            continue
        }
        
        if resolution.Resolved && resolution.Action == ActionFixed {
            autoFixed = append(autoFixed, resolution.Message)
            
            // Create audit event if resolution created one
            if resolution.AuditEvent != nil {
                createResolutionAuditEvent(fixCtx, issue, resolution)
            }
            
            // Track violation resolution
            trackViolationResolution(fixCtx, issue, resolution)
        }
    }
    
    return autoFixed
}
```

## Implementation Phases

### Phase 1: Foundation (Current State Analysis)
- [x] Document all integrity scenarios
- [x] Analyze existing resolver infrastructure
- [x] Identify gaps
- [ ] Design resolver interface
- [ ] Design tracking system

### Phase 2: Core Resolvers (High-Value, Low-Risk)
- [ ] Implement HashIntegrityResolver (extend existing)
- [ ] Implement RegistrationResolver (kind inference)
- [ ] Implement InstanceValidationResolver (extend SpecBasedAutoFixer)
- [ ] Create ResolverRegistry
- [ ] Integrate with system check

### Phase 3: Smart Resolvers (Medium-Value, Medium-Risk)
- [ ] Implement ReferenceIntegrityResolver (with FieldResolver)
- [ ] Implement LifecycleResolver (status fixes)
- [ ] Add confidence thresholds
- [ ] Add user confirmation for medium-risk resolutions

### Phase 4: Tracking and Analytics
- [ ] Implement violation tracking metadata
- [ ] Implement CLI bypass detection
- [ ] Implement agent/user statistics
- [ ] Create violation registry storage
- [ ] Create analytics/reporting commands

### Phase 5: Advanced Features
- [ ] PolicyResolver (policy-specific logic)
- [ ] Resolution strategies for edge cases
- [ ] Batch resolution optimization
- [ ] Resolution preview/dry-run mode

## Safety Considerations

### When NOT to Auto-Resolve

1. **Data Loss Risk:** Never auto-resolve if resolution could cause data loss
2. **Ambiguity:** Never auto-resolve if multiple valid resolutions exist without clear best choice
3. **Side Effects:** Never auto-resolve if resolution has significant side effects (e.g., status changes)
4. **Business Rules:** Never auto-resolve policy violations (requires domain knowledge)
5. **User Intent:** Never auto-resolve if user intent cannot be inferred

### When to Require User Confirmation

1. **Medium Confidence:** 0.7 < confidence < 0.9
2. **Multiple Options:** Multiple valid resolution paths
3. **Side Effects:** Resolution changes object state in ways that might not be intended
4. **Relationships:** Resolution creates/modifies relationships between objects

### When to Auto-Resolve

1. **High Confidence:** confidence >= 0.9 and safety level = Safe/Low
2. **Non-Destructive:** Resolution is clearly non-destructive (e.g., adding missing hash)
3. **Unambiguous:** Only one valid resolution path exists
4. **User Consent:** User has provided consent (--force flag, --auto-resolve flag)

## Tracking and Reporting

### Violation Tracking Commands

```bash
# View violation statistics
zqk system violations stats

# View violations by agent/user
zqk system violations by-agent
zqk system violations by-user

# View CLI bypass indicators
zqk system violations cli-bypass

# View violation trends over time
zqk system violations trends --period 7d
```

### Analytics Output

- Violation counts by category, tier, agent, user
- CLI bypass patterns (which directories/kinds have most bypasses)
- Resolution success rates (auto-resolved vs. manual)
- Agent/user compliance metrics

## Next Steps

1. **Review and approve design** - Get stakeholder feedback
2. **Implement Phase 1** - Foundation and tracking design
3. **Implement Phase 2** - Core resolvers
4. **Test and validate** - Ensure safety and correctness
5. **Implement Phase 3** - Smart resolvers
6. **Implement Phase 4** - Tracking system
7. **Document and train** - User documentation and examples

## Implications of Inferring Fixes

### Critical Considerations

**1. Data Integrity Risk**
- **Problem:** Auto-resolving violations by inference can introduce errors if inference is wrong
- **Mitigation:**
  - Only auto-resolve when confidence > threshold (varies by scenario)
  - Require user confirmation for medium-risk resolutions
  - Create audit events for all resolutions (for rollback/review)
  - Re-validate after resolution to catch inference errors

**2. User Intent Violation**
- **Problem:** Inferring a fix may not match user intent (e.g., linking wrong reference)
- **Mitigation:**
  - Prefer suggesting fixes over auto-resolving when intent is ambiguous
  - Use FieldResolver with high confidence thresholds (>0.9) for auto-resolution
  - Provide clear explanations of what was inferred and why
  - Allow users to review suggested fixes before applying

**3. Cascade Effects**
- **Problem:** Resolving one violation may create new violations or affect other objects
- **Mitigation:**
  - Re-validate entire object after each resolution
  - Track resolution dependencies (resolution A enables resolution B)
  - Process resolutions in dependency order
  - Stop if new blocking violations are introduced

**4. Auditability and Accountability**
- **Problem:** Inferring fixes reduces traceability of who/what made decisions
- **Mitigation:**
  - Create detailed audit events for all resolutions (what was inferred, confidence level, why)
  - Track attribution metadata (which resolver, which logic path, which hints used)
  - Store inference reasoning in audit event metadata
  - Allow users to query "why was this fixed this way?"

**5. Agent Bypass Masking**
- **Problem:** Auto-resolving violations masks the fact that agents are bypassing the CLI
- **Mitigation:**
  - **Always track violations BEFORE resolution** (even if auto-resolved)
  - Create violation tracking records before resolution attempts
  - Track resolution statistics separately from violation statistics
  - Generate reports showing: violations detected → violations auto-resolved → violations remaining
  - Flag patterns: agent with high violation rate + high auto-resolve rate = agent needs training/fixing

**6. False Confidence**
- **Problem:** High confidence scores may be misleading if inference logic has bugs
- **Mitigation:**
  - Start with conservative thresholds (0.95+ for auto-resolve)
  - Gradually lower thresholds based on success rates
  - Monitor resolution success (re-validation after resolution)
  - Track resolution failure rates by scenario and confidence level

### Recommended Approach: Track-First, Resolve-Second

**Principle:** Always track violations before attempting resolution, even if auto-resolving.

**Workflow:**
1. **Detection Phase:**
   - Detect violation
   - Create violation tracking record (with attribution metadata)
   - Store in violation registry

2. **Analysis Phase:**
   - Determine if violation can be auto-resolved
   - Calculate confidence score
   - Determine resolution strategy (auto, suggest, require-user)

3. **Resolution Phase:**
   - If auto-resolvable with high confidence: resolve immediately
   - If auto-resolvable with medium confidence: suggest and require confirmation
   - If not auto-resolvable: suggest manual fix
   - Create resolution tracking record (linked to violation record)
   - Create audit event for resolution

4. **Verification Phase:**
   - Re-validate object after resolution
   - If new violations introduced: rollback resolution (if possible) or mark as failed
   - Update resolution tracking record with success/failure

**Benefits:**
- Violations are tracked even if auto-resolved
- Agent/user statistics remain accurate
- Resolution decisions are auditable
- Resolution failures can be analyzed and improved

## Open Questions

1. **Tracking Storage:** Where should violation tracking data be stored?
   - Option A: Separate `.zqk/violations/` directory (structured YAML/JSON files)
   - Option B: Extend audit events with violation metadata
   - Option C: New metrics system (separate from audit events)
   - **Recommendation:** Option A (separate tracking registry) + Option B (audit events for resolutions)
   - **Rationale:** Violation tracking is different from audit events (violations are detected state, audit events are actions). Separate tracking allows analytics without polluting audit event stream.

2. **Agent Identification:** How to reliably identify which agent/user made changes?
   - **Current:** Use `created_by`/`updated_by` from object metadata
   - **Gap:** Files created outside CLI don't have this metadata
   - **Recommendation:** Multi-layered inference strategy (see "CLI Bypass Detection Strategy" above)
   - **Implementation:** Track attribution confidence levels, use correlation with audit events, filesystem metadata

3. **Resolution Rollback:** Should we support rolling back auto-resolutions?
   - **Recommendation:** Limited rollback support
   - **Rationale:** Full rollback is complex (cascade effects, state changes). Instead:
     - Create audit events for all resolutions (auditable)
     - Re-validate after resolution (catch errors early)
     - If resolution introduces new violations, mark resolution as failed and suggest manual fix
     - Store "before" state in audit event metadata for manual review/rollback if needed

4. **Batch Resolution:** How to handle resolution of thousands of violations?
   - **Recommendation:** Process in batches with progress tracking
   - **Implementation:**
     - Group violations by type/strategy for efficient batch processing
     - Process in batches of 100-1000 (configurable)
     - Show progress indicator
     - Allow interruption (Ctrl+C)
     - Resume capability (track which violations were processed)
     - Transaction boundaries: resolve violations for one object atomically

5. **Confidence Thresholds:** Should thresholds be configurable?
   - **Recommendation:** Yes, via config file with safe defaults
   - **Default Thresholds:**
     - Auto-resolve: 0.95 (very high confidence only)
     - Suggest with auto-apply: 0.85-0.95 (medium-high confidence, require user confirmation)
     - Suggest only: <0.85 (low-medium confidence, user must review)
   - **Configuration File:** `.zqk/resolver-config.yaml`
   - **Per-Scenario Overrides:** Allow different thresholds for different violation types

6. **Inference Logic Maintenance:** How to ensure inference logic remains accurate as system evolves?
   - **Recommendation:** Comprehensive testing and monitoring
   - **Implementation:**
     - Unit tests for each resolver with known scenarios
     - Integration tests with real violation patterns
     - Track resolution success rates (re-validation after resolution)
     - Alert on resolution failure rate increases
     - Regular review of resolution audit events for pattern analysis

7. **Performance Impact:** How to ensure resolution system doesn't slow down system check?
   - **Recommendation:** Async resolution with progress tracking
   - **Implementation:**
     - Detection phase: synchronous (fast, just identify violations)
     - Resolution phase: optional, user-triggered (via --auto-resolve flag)
     - Resolution can run in background with progress updates
     - Cache resolution results (don't re-resolve same violation if object unchanged)