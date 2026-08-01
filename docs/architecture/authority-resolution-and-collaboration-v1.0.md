# Authority Resolution and Multi-User Collaboration v1.0

**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: Distributed Kernel Architecture v1.0, Multi-Instance Policy Reconciliation v1.0, Modular Scalable Architecture v1.0

## Overview

This document defines a comprehensive system for authority resolution, user context reconciliation, and conflict-free collaboration when multiple zqk CLI instances operate within an enterprise. It addresses scenarios where users have different access levels to different repositories, need to merge or separate concerns, and must handle "ah-ha" moments where configuration pivots are required.

## Problem Statement

**Challenges**:

1. **Differential Authority**: CLI users have full control over some repositories but limited/no access to others
2. **Authority Structure**: How to reconcile user context and determine authority structure
3. **Conflict Prevention**: Prevent conflicts when multiple CLIs operate in an enterprise
4. **Strategic Alignment**: Some cases desire merging concerns, others remain separate/loosely connected
5. **Configuration Pivots**: Handle "ah-ha" moments where misalignment is discovered and pivots are needed
6. **Collaboration**: Ensure CLIs can collaborate without overwriting and creating blocking conflicts

**Example Scenarios**:

- **Data Science vs Business Intelligence**: Separate prototypes/experiments from BI, but overlapping concern in technical spend
- **Infrastructure vs Product**: Different teams, different repositories, but need strategic alignment
- **Early Misalignment**: Configuration established early, later discovered misalignment requires pivot

## Solution: Authority Resolution and Collaboration Protocol

### Core Principles

1. **Authority Boundaries**: Clear boundaries define what each user can modify
2. **Conflict Detection**: Detect conflicts before they become blocking
3. **Collaborative Merging**: Merge changes collaboratively with conflict resolution
4. **Pivot Support**: Support configuration pivots without data loss
5. **Authority Escalation**: Mechanism for requesting elevated permissions
6. **Audit Trail**: Complete audit trail of all authority decisions

## Architecture

### 1. Authority Resolution System

**Command**: `zqk authority resolve`

Resolves authority for operations across repositories:

**Authority Resolution Process**:

#### Step 1: User Context Discovery
```bash
# Discover user context and permissions
zqk authority discover --user

# Output:
# User: account:alice
# Repositories:
#   - repo:data-science (full control)
#   - repo:business-intelligence (read-only)
#   - repo:infrastructure (limited: write backlog items)
```

**User Context Object**:
```yaml
id: USER-CTX-001
kind: user_context
title: "Alice's User Context"
account_id: "account:alice"
repositories:
  - repository: "repo:data-science"
    permissions:
      - read:*
      - write:*
      - delete:*
    authority_level: full_control
  
  - repository: "repo:business-intelligence"
    permissions:
      - read:*
    authority_level: read_only
  
  - repository: "repo:infrastructure"
    permissions:
      - read:*
      - write:backlog_item
      - write:milestone
    authority_level: limited
```

#### Step 2: Repository Authority Mapping
```bash
# Map authority across repositories
zqk authority map --repositories

# Output:
# Repository Authority Map:
#   repo:data-science:
#     - account:alice (full_control)
#     - account:bob (read_only)
#   repo:business-intelligence:
#     - account:charlie (full_control)
#     - account:alice (read_only)
#   repo:infrastructure:
#     - account:dave (full_control)
#     - account:alice (limited)
```

**Repository Authority Object**:
```yaml
id: REPO-AUTH-001
kind: repository_authority
title: "Data Science Repository Authority"
repository: "repo:data-science"
authorities:
  - account_id: "account:alice"
    permissions: ["read:*", "write:*", "delete:*"]
    authority_level: full_control
    granted_by: "account:admin"
    granted_at: "2025-01-15T10:00:00Z"
  
  - account_id: "account:bob"
    permissions: ["read:*"]
    authority_level: read_only
    granted_by: "account:admin"
    granted_at: "2025-01-15T10:00:00Z"
```

#### Step 3: Operation Authority Check
```bash
# Check if user has authority for operation
zqk authority check \
  --user "account:alice" \
  --operation "write:goal" \
  --repository "repo:data-science"

# Output:
# Authority: granted
# Permission: write:goal
# Repository: repo:data-science
# Authority Level: full_control
```

**Authority Check Logic**:
```go
func CheckAuthority(userCtx *UserContext, operation string, repository string) (*AuthorityResult, error) {
    // 1. Check repository-specific permissions
    repoAuth := GetRepositoryAuthority(repository)
    if repoAuth.HasPermission(userCtx.AccountID, operation) {
        return &AuthorityResult{Granted: true, Source: "repository"}, nil
    }
    
    // 2. Check enterprise-wide permissions
    enterpriseAuth := GetEnterpriseAuthority()
    if enterpriseAuth.HasPermission(userCtx.AccountID, operation) {
        return &AuthorityResult{Granted: true, Source: "enterprise"}, nil
    }
    
    // 3. Check role-based permissions
    if userCtx.HasRole("admin") {
        return &AuthorityResult{Granted: true, Source: "role"}, nil
    }
    
    // 4. Deny if no match
    return &AuthorityResult{Granted: false, Reason: "insufficient_permissions"}, nil
}
```

### 2. Conflict Detection and Prevention

**Command**: `zqk conflict detect`

Detects potential conflicts before they occur:

**Conflict Detection Process**:

#### Step 1: Pre-Operation Conflict Check
```bash
# Check for conflicts before operation
zqk conflict detect \
  --operation "update" \
  --object "GOAL-001" \
  --repository "repo:data-science"

# Output:
# Conflict Check: passed
# Last Modified: 2025-12-30T10:00:00Z by account:alice
# Current Version: v1.2.3
# No conflicts detected
```

#### Step 2: Concurrent Modification Detection
```bash
# Detect concurrent modifications
zqk conflict detect --concurrent

# Output:
# Concurrent Modifications Detected:
#   - GOAL-001: Modified by account:alice and account:bob
#   - BLI-123: Modified by account:charlie and account:dave
#   - Resolution: merge_required
```

**Conflict Detection Logic**:
```go
func DetectConflicts(operation string, objectID string, repository string) (*ConflictReport, error) {
    // 1. Check for concurrent modifications
    concurrent := CheckConcurrentModifications(objectID)
    if len(concurrent) > 0 {
        return &ConflictReport{
            HasConflicts: true,
            Conflicts: concurrent,
            Resolution: "merge_required",
        }, nil
    }
    
    // 2. Check for authority conflicts
    authorityConflicts := CheckAuthorityConflicts(operation, objectID)
    if len(authorityConflicts) > 0 {
        return &ConflictReport{
            HasConflicts: true,
            Conflicts: authorityConflicts,
            Resolution: "authority_escalation_required",
        }, nil
    }
    
    // 3. Check for policy conflicts
    policyConflicts := CheckPolicyConflicts(operation, objectID)
    if len(policyConflicts) > 0 {
        return &ConflictReport{
            HasConflicts: true,
            Conflicts: policyConflicts,
            Resolution: "policy_reconciliation_required",
        }, nil
    }
    
    return &ConflictReport{HasConflicts: false}, nil
}
```

### 3. Collaborative Merging

**Command**: `zqk conflict merge`

Merges changes collaboratively:

**Merge Strategies**:

#### Strategy 1: Automatic Merge (Non-Conflicting)
```bash
# Automatic merge for non-conflicting changes
zqk conflict merge \
  --object "GOAL-001" \
  --strategy "automatic"

# Output:
# Merge: successful
# Strategy: automatic
# Conflicts: 0
# Merged Changes:
#   - account:alice: Updated description
#   - account:bob: Updated target
```

#### Strategy 2: Three-Way Merge (Conflicting)
```bash
# Three-way merge for conflicting changes
zqk conflict merge \
  --object "GOAL-001" \
  --strategy "three_way" \
  --base "v1.2.0" \
  --ours "v1.2.1" \
  --theirs "v1.2.2"

# Output:
# Merge: conflicts detected
# Strategy: three_way
# Conflicts: 2
# Resolution: manual_required
# Conflict Details:
#   - Field: description (both modified)
#   - Field: target (both modified)
```

#### Strategy 3: Authority-Based Merge
```bash
# Authority-based merge (higher authority wins)
zqk conflict merge \
  --object "GOAL-001" \
  --strategy "authority_based"

# Output:
# Merge: successful
# Strategy: authority_based
# Authority: account:alice (full_control) > account:bob (read_only)
# Resolution: account:alice's changes applied
```

**Merge Object**:
```yaml
id: MERGE-001
kind: merge_operation
title: "Merge GOAL-001"
object_id: "GOAL-001"
strategy: "three_way"
status: "conflicts_detected"
conflicts:
  - field: "description"
    our_value: "Data science goal"
    their_value: "Business intelligence goal"
    resolution: "manual_required"
  - field: "target"
    our_value: "100 experiments"
    their_value: "50 reports"
    resolution: "manual_required"
resolved_by: null
resolved_at: null
```

### 4. Configuration Pivot Support

**Command**: `zqk pivot`

Handles configuration pivots when misalignment is discovered:

**Pivot Process**:

#### Step 1: Pivot Discovery
```bash
# Discover misalignment requiring pivot
zqk pivot discover

# Output:
# Misalignment Discovered:
#   - Configuration: data-science and business-intelligence are separate
#   - Issue: Overlapping concern in technical spend
#   - Recommendation: Merge into shared technical-spend goal
#   - Impact: High (affects 5 goals, 12 backlog items)
```

#### Step 2: Pivot Planning
```bash
# Plan configuration pivot
zqk pivot plan \
  --from "separate" \
  --to "merged" \
  --concern "technical-spend"

# Output:
# Pivot Plan:
#   - Merge Goals: GOAL-DS-001, GOAL-BI-001 → GOAL-TECH-001
#   - Reconcile Policies: POL-DS-001, POL-BI-001 → POL-TECH-001
#   - Update References: 12 backlog items, 5 milestones
#   - Estimated Impact: Medium
#   - Rollback Plan: Available
```

**Pivot Object**:
```yaml
id: PIVOT-001
kind: pivot_operation
title: "Merge Data Science and Business Intelligence"
pivot_type: "merge_concerns"
from_state:
  repositories: ["repo:data-science", "repo:business-intelligence"]
  goals: ["GOAL-DS-001", "GOAL-BI-001"]
  policies: ["POL-DS-001", "POL-BI-001"]
  status: "separate"
to_state:
  repositories: ["repo:technical-spend"]
  goals: ["GOAL-TECH-001"]
  policies: ["POL-TECH-001"]
  status: "merged"
impact:
  goals_affected: 2
  backlog_items_affected: 12
  milestones_affected: 5
  policies_affected: 2
rollback_plan:
  enabled: true
  checkpoint: "pivot-checkpoint-001"
status: "planned"
```

#### Step 3: Pivot Execution
```bash
# Execute configuration pivot
zqk pivot execute --pivot PIVOT-001 --confirm

# Output:
# Pivot: executing
# Checkpoint: created (pivot-checkpoint-001)
# Merging Goals: GOAL-DS-001, GOAL-BI-001 → GOAL-TECH-001
# Reconciling Policies: POL-DS-001, POL-BI-001 → POL-TECH-001
# Updating References: 12 backlog items, 5 milestones
# Pivot: complete
```

#### Step 4: Pivot Rollback (If Needed)
```bash
# Rollback configuration pivot
zqk pivot rollback --pivot PIVOT-001 --checkpoint pivot-checkpoint-001

# Output:
# Pivot: rolling back
# Checkpoint: pivot-checkpoint-001
# Restoring Goals: GOAL-DS-001, GOAL-BI-001
# Restoring Policies: POL-DS-001, POL-BI-001
# Restoring References: 12 backlog items, 5 milestones
# Rollback: complete
```

### 5. Authority Escalation

**Command**: `zqk authority escalate`

Requests elevated permissions:

**Escalation Process**:

#### Step 1: Escalation Request
```bash
# Request authority escalation
zqk authority escalate \
  --operation "write:goal" \
  --repository "repo:business-intelligence" \
  --reason "Need to merge goals for technical spend alignment"

# Output:
# Escalation Request: created
# Request ID: ESC-001
# Operation: write:goal
# Repository: repo:business-intelligence
# Reason: Need to merge goals for technical spend alignment
# Status: pending_approval
```

**Escalation Request Object**:
```yaml
id: ESC-001
kind: escalation_request
title: "Authority Escalation for Goal Merge"
requested_by: "account:alice"
operation: "write:goal"
repository: "repo:business-intelligence"
reason: "Need to merge goals for technical spend alignment"
current_authority: "read_only"
requested_authority: "write:goal"
status: "pending_approval"
approved_by: null
approved_at: null
```

#### Step 2: Escalation Approval
```bash
# Approve authority escalation (by admin)
zqk authority approve --escalation ESC-001 --approver account:admin

# Output:
# Escalation: approved
# Request ID: ESC-001
# Approved By: account:admin
# Approved At: 2025-12-30T10:00:00Z
# Authority: granted (temporary, expires 2025-12-31T10:00:00Z)
```

### 6. Collaboration Protocol

**Command**: `zqk collaborate`

Manages collaboration between CLI instances:

**Collaboration Features**:

#### Feature 1: Collaboration Session
```bash
# Start collaboration session
zqk collaborate start \
  --session "merge-technical-spend" \
  --participants "account:alice,account:charlie"

# Output:
# Collaboration Session: started
# Session ID: COLLAB-001
# Participants: account:alice, account:charlie
# Repositories: repo:data-science, repo:business-intelligence
# Status: active
```

**Collaboration Session Object**:
```yaml
id: COLLAB-001
kind: collaboration_session
title: "Merge Technical Spend Goals"
participants:
  - account_id: "account:alice"
    repository: "repo:data-science"
    authority: "full_control"
  - account_id: "account:charlie"
    repository: "repo:business-intelligence"
    authority: "full_control"
repositories: ["repo:data-science", "repo:business-intelligence"]
operations:
  - operation: "merge_goals"
    objects: ["GOAL-DS-001", "GOAL-BI-001"]
    target: "GOAL-TECH-001"
status: "active"
started_at: "2025-12-30T10:00:00Z"
```

#### Feature 2: Lock Management
```bash
# Lock object for exclusive modification
zqk collaborate lock \
  --object "GOAL-001" \
  --session COLLAB-001 \
  --user "account:alice"

# Output:
# Lock: acquired
# Object: GOAL-001
# Locked By: account:alice
# Session: COLLAB-001
# Expires: 2025-12-30T11:00:00Z (1 hour)
```

**Lock Object**:
```yaml
id: LOCK-001
kind: object_lock
title: "Lock on GOAL-001"
object_id: "GOAL-001"
locked_by: "account:alice"
session_id: "COLLAB-001"
acquired_at: "2025-12-30T10:00:00Z"
expires_at: "2025-12-30T11:00:00Z"
status: "active"
```

#### Feature 3: Change Notification
```bash
# Notify collaborators of changes
zqk collaborate notify \
  --session COLLAB-001 \
  --change "GOAL-001 updated by account:alice"

# Output:
# Notification: sent
# Session: COLLAB-001
# Recipients: account:charlie
# Change: GOAL-001 updated by account:alice
```

### 7. Repository Relationship Management

**Command**: `zqk repository relate`

Manages relationships between repositories:

**Relationship Types**:

1. **Separate**: No relationship, independent operation
2. **Loosely Connected**: Shared goals/policies, independent execution
3. **Merged**: Fully merged, shared state
4. **Pivoting**: Transitioning between states

**Relationship Management**:
```bash
# Define repository relationship
zqk repository relate \
  --repo1 "repo:data-science" \
  --repo2 "repo:business-intelligence" \
  --relationship "loosely_connected" \
  --shared_concerns "technical-spend"

# Output:
# Relationship: created
# Repository 1: repo:data-science
# Repository 2: repo:business-intelligence
# Relationship: loosely_connected
# Shared Concerns: technical-spend
```

**Repository Relationship Object**:
```yaml
id: REPO-REL-001
kind: repository_relationship
title: "Data Science - Business Intelligence Relationship"
repository_1: "repo:data-science"
repository_2: "repo:business-intelligence"
relationship_type: "loosely_connected"
shared_concerns:
  - concern: "technical-spend"
    goals: ["GOAL-DS-001", "GOAL-BI-001"]
    policies: ["POL-DS-001", "POL-BI-001"]
    alignment: "partial"
status: "active"
```

### 8. Conflict Resolution Strategies

**Resolution Strategies**:

#### Strategy 1: Last Write Wins (Default)
- Simple, fast
- May lose changes
- Use for low-stakes conflicts

#### Strategy 2: Three-Way Merge
- Preserves all changes
- Requires manual resolution for conflicts
- Use for high-stakes conflicts

#### Strategy 3: Authority-Based
- Higher authority wins
- Clear resolution
- Use for authority conflicts

#### Strategy 4: Collaborative Merge
- All participants review
- Consensus-based resolution
- Use for strategic decisions

**Conflict Resolution Configuration**:
```yaml
# .zqk/config.yaml
conflict_resolution:
  default_strategy: "three_way"
  strategies:
    low_stakes: "last_write_wins"
    high_stakes: "three_way"
    authority_conflicts: "authority_based"
    strategic_decisions: "collaborative_merge"
  
  auto_resolve:
    enabled: true
    threshold: "low_stakes"  # Auto-resolve low-stakes conflicts
```

## Implementation

### Phase 1: Authority Resolution

```go
// pkg/authority/resolver.go
type AuthorityResolver struct {
    userContexts map[string]*UserContext
    repoAuthorities map[string]*RepositoryAuthority
    enterpriseAuth *EnterpriseAuthority
}

func (ar *AuthorityResolver) Resolve(userID string, operation string, repository string) (*AuthorityResult, error) {
    // 1. Get user context
    userCtx := ar.GetUserContext(userID)
    
    // 2. Check repository authority
    repoAuth := ar.GetRepositoryAuthority(repository)
    if repoAuth.HasPermission(userID, operation) {
        return &AuthorityResult{Granted: true, Source: "repository"}, nil
    }
    
    // 3. Check enterprise authority
    if ar.enterpriseAuth.HasPermission(userID, operation) {
        return &AuthorityResult{Granted: true, Source: "enterprise"}, nil
    }
    
    // 4. Deny
    return &AuthorityResult{Granted: false}, nil
}
```

### Phase 2: Conflict Detection

```go
// pkg/conflict/detector.go
type ConflictDetector struct {
    storage ObjectStorageProvider
    authority *AuthorityResolver
}

func (cd *ConflictDetector) Detect(operation string, objectID string, repository string) (*ConflictReport, error) {
    // 1. Check concurrent modifications
    concurrent := cd.CheckConcurrentModifications(objectID)
    
    // 2. Check authority conflicts
    authorityConflicts := cd.CheckAuthorityConflicts(operation, objectID)
    
    // 3. Check policy conflicts
    policyConflicts := cd.CheckPolicyConflicts(operation, objectID)
    
    return &ConflictReport{
        HasConflicts: len(concurrent) > 0 || len(authorityConflicts) > 0 || len(policyConflicts) > 0,
        Conflicts: append(append(concurrent, authorityConflicts...), policyConflicts...),
    }, nil
}
```

### Phase 3: Pivot Support

```go
// pkg/pivot/manager.go
type PivotManager struct {
    storage ObjectStorageProvider
    authority *AuthorityResolver
}

func (pm *PivotManager) PlanPivot(fromState, toState *PivotState) (*PivotPlan, error) {
    // 1. Analyze impact
    impact := pm.AnalyzeImpact(fromState, toState)
    
    // 2. Create rollback plan
    rollback := pm.CreateRollbackPlan(fromState)
    
    // 3. Generate pivot plan
    return &PivotPlan{
        FromState: fromState,
        ToState: toState,
        Impact: impact,
        RollbackPlan: rollback,
    }, nil
}
```

## Use Cases

### Use Case 1: Differential Authority

**Scenario**: Alice has full control over data-science repo, read-only on business-intelligence repo

**Solution**:
- Authority resolution checks repository-specific permissions
- Alice can modify data-science goals, but not business-intelligence goals
- Escalation request if Alice needs to modify business-intelligence goals

### Use Case 2: Configuration Pivot

**Scenario**: Early misalignment discovered, need to merge data-science and business-intelligence for technical spend

**Solution**:
- Pivot discovery identifies misalignment
- Pivot planning creates merge plan
- Pivot execution merges goals/policies with rollback support

### Use Case 3: Collaborative Merging

**Scenario**: Multiple CLIs modifying same objects

**Solution**:
- Conflict detection identifies concurrent modifications
- Collaborative merge session coordinates changes
- Lock management prevents overwrites
- Change notifications keep participants informed

## Benefits

1. **Authority Clarity**: Clear authority boundaries prevent unauthorized changes
2. **Conflict Prevention**: Detect conflicts before they become blocking
3. **Collaborative Merging**: Merge changes without data loss
4. **Pivot Support**: Handle configuration pivots safely
5. **Audit Trail**: Complete audit trail of all authority decisions
6. **Flexibility**: Support separate, loosely connected, and merged states

## Related Documentation

- [Distributed Kernel Architecture](./distributed-kernel-architecture-v1.0.md)
- [Multi-Instance Policy Reconciliation](./multi-instance-policy-reconciliation-v1.0.md)
- [Modular Scalable Architecture](./modular-scalable-architecture-v1.0.md)

---

*This system ensures multiple zqk CLI instances can collaborate effectively without overwriting and creating blocking conflicts, while supporting authority resolution, conflict detection, and configuration pivots.*

