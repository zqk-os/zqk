> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Role Discovery and Progressive Configuration v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: Authority Resolution v1.0, Modular Scalable Architecture v1.0, Distributed Kernel Architecture v1.0

## Overview

This document defines a comprehensive system for discovering roles and privileges, registering CLI instances with PKI, enabling rapid evolution from simple to enterprise configurations, and proactively detecting alignment opportunities to prompt humans.

## Problem Statement

**Challenges**:

1. **Role Discovery**: How to determine what roles and privileges an enterprise or individual needs
2. **CLI Instance Registration**: Each CLI instance needs unique ID registered with PKI system
3. **Rapid Growth**: Startups need to adapt from simple to complex enterprise configurations quickly
4. **Alignment Opportunities**: System should proactively detect and prompt humans about alignment opportunities

**Example Scenarios**:

- **Individual Developer**: Starts with minimal roles, grows to team, needs role expansion
- **Startup**: Begins simple, scales rapidly, needs enterprise features quickly
- **Enterprise**: Complex role structure, needs discovery and optimization
- **Alignment Discovery**: System detects overlapping concerns, prompts for alignment

## Solution: Role Discovery and Progressive Configuration

### Core Principles

1. **Discover Before Prescribe**: Analyze needs before assigning roles
2. **Progressive Enhancement**: Start minimal, add complexity as needed
3. **PKI Integration**: All CLI instances registered with PKI
4. **Rapid Evolution**: Quick migration from simple to enterprise
5. **Proactive Detection**: System detects and prompts alignment opportunities
6. **Human Guidance**: Clear prompts guide humans through decisions

## Architecture

### 1. Role and Privilege Discovery

**Command**: `zqk role discover`

Discovers roles and privileges based on context:

**Discovery Process**:

#### Step 1: Context Analysis
```bash
# Analyze context to determine roles needed
zqk role discover --analyze

# Output:
# Context Analysis:
#   - Project Type: startup
#   - Team Size: 5
#   - Repositories: 3
#   - Current Roles: 2 (admin, developer)
#   - Recommended Roles: 4 (admin, developer, viewer, automation)
```

**Context Analysis Factors**:
- **Project Type**: Individual, startup, enterprise
- **Team Size**: Number of users
- **Repository Count**: Number of repositories
- **Current Usage**: What operations are being performed
- **Growth Trajectory**: Expected growth rate

#### Step 2: Role Recommendation
```bash
# Get role recommendations
zqk role recommend --context startup

# Output:
# Recommended Roles:
#   1. admin (full control)
#      - Permissions: read:*, write:*, delete:*
#      - Users: 1-2 (founders/CTO)
#      - Justification: Need full control for initial setup
#   
#   2. developer (standard development)
#      - Permissions: read:*, write:backlog_item, write:milestone
#      - Users: 3-10 (engineering team)
#      - Justification: Standard development workflow
#   
#   3. viewer (read-only access)
#      - Permissions: read:*
#      - Users: stakeholders, external teams
#      - Justification: Visibility without modification
#   
#   4. automation (CI/CD, bots)
#      - Permissions: read:*, write:backlog_item (status only)
#      - Users: CI/CD systems, automation bots
#      - Justification: Automated workflows
```

**Role Recommendation Object**:
```yaml
id: ROLE-REC-001
kind: role_recommendation
title: "Startup Role Recommendations"
context_type: startup
team_size: 5
repositories: 3
recommended_roles:
  - role_id: "admin"
    permissions: ["read:*", "write:*", "delete:*"]
    user_count: 1
    justification: "Full control for initial setup"
    priority: high
  
  - role_id: "developer"
    permissions: ["read:*", "write:backlog_item", "write:milestone"]
    user_count: 4
    justification: "Standard development workflow"
    priority: high
  
  - role_id: "viewer"
    permissions: ["read:*"]
    user_count: 0
    justification: "Visibility for stakeholders"
    priority: medium
  
  - role_id: "automation"
    permissions: ["read:*", "write:backlog_item"]
    user_count: 0
    justification: "CI/CD and automation"
    priority: low
```

#### Step 3: Privilege Analysis
```bash
# Analyze what privileges are actually needed
zqk role analyze-privileges --usage-history

# Output:
# Privilege Analysis:
#   - Most Used: read:*, write:backlog_item (95% of operations)
#   - Rarely Used: delete:*, write:goal (5% of operations)
#   - Recommended: Grant read:* and write:backlog_item to developers
#   - Escalation: Use escalation for delete:* and write:goal
```

**Privilege Analysis Object**:
```yaml
id: PRIV-ANAL-001
kind: privilege_analysis
title: "Privilege Usage Analysis"
analysis_period: "2025-01-01 to 2025-12-30"
findings:
  - privilege: "read:*"
    usage_count: 1000
    usage_percentage: 50
    recommendation: "grant_to_all"
  
  - privilege: "write:backlog_item"
    usage_count: 800
    usage_percentage: 40
    recommendation: "grant_to_developers"
  
  - privilege: "delete:*"
    usage_count: 10
    usage_percentage: 0.5
    recommendation: "escalation_only"
  
  - privilege: "write:goal"
    usage_count: 5
    usage_percentage: 0.25
    recommendation: "admin_only"
```

### 2. CLI Instance Registration with PKI

**Command**: `zqk instance register`

Registers CLI instance with PKI system:

**Registration Process**:

#### Step 1: Instance Identity Generation
```bash
# Generate unique instance ID
zqk instance register --generate-id

# Output:
# Instance ID: CLI-INST-001-2025-12-30-abc123def456
# Generated: 2025-12-30T10:00:00Z
# Fingerprint: SHA256:abc123def456...
```

**Instance Identity Object**:
```yaml
id: CLI-INST-001-2025-12-30-abc123def456
kind: cli_instance
title: "Alice's Development CLI"
instance_id: "CLI-INST-001-2025-12-30-abc123def456"
hostname: "alice-laptop.local"
user: "account:alice"
registered_at: "2025-12-30T10:00:00Z"
fingerprint: "SHA256:abc123def456..."
certificate:
  subject: "cli-instance:CLI-INST-001-2025-12-30-abc123def456"
  issuer: "ca:zqk-instance"
  permissions: ["read:*", "write:backlog_item", "write:milestone"]
  validity:
    not_before: "2025-12-30T10:00:00Z"
    not_after: "2026-12-30T10:00:00Z"
```

#### Step 2: PKI Certificate Request
```bash
# Request PKI certificate for instance
zqk instance register --request-certificate \
  --instance-id "CLI-INST-001-2025-12-30-abc123def456" \
  --permissions "read:*,write:backlog_item,write:milestone"

# Output:
# Certificate Request: created
# Request ID: CERT-REQ-001
# Status: pending_approval
# CA: ca:zqk-instance
```

**Certificate Request Object**:
```yaml
id: CERT-REQ-001
kind: certificate_request
title: "Certificate Request for CLI-INST-001"
instance_id: "CLI-INST-001-2025-12-30-abc123def456"
requested_permissions:
  - read:*
  - write:backlog_item
  - write:milestone
status: "pending_approval"
requested_by: "account:alice"
requested_at: "2025-12-30T10:00:00Z"
approved_by: null
approved_at: null
certificate_issued: false
```

#### Step 3: Certificate Issuance
```bash
# Approve and issue certificate (by CA admin)
zqk instance approve --certificate-request CERT-REQ-001 --approver account:admin

# Output:
# Certificate: issued
# Request ID: CERT-REQ-001
# Certificate ID: CERT-001
# Issued By: account:admin
# Issued At: 2025-12-30T10:05:00Z
# Validity: 2025-12-30 to 2026-12-30
```

**Certificate Object**:
```yaml
id: CERT-001
kind: certificate
title: "Certificate for CLI-INST-001"
instance_id: "CLI-INST-001-2025-12-30-abc123def456"
subject: "cli-instance:CLI-INST-001-2025-12-30-abc123def456"
issuer: "ca:zqk-instance"
permissions:
  - read:*
  - write:backlog_item
  - write:milestone
validity:
  not_before: "2025-12-30T10:05:00Z"
  not_after: "2026-12-30T10:05:00Z"
public_key: "-----BEGIN PUBLIC KEY-----\n..."
signature: "-----BEGIN SIGNATURE-----\n..."
```

#### Step 4: Instance Registration
```bash
# Register instance with PKI system
zqk instance register --certificate CERT-001

# Output:
# Instance: registered
# Instance ID: CLI-INST-001-2025-12-30-abc123def456
# Certificate: CERT-001
# Status: active
# Registered At: 2025-12-30T10:05:00Z
```

### 3. Progressive Configuration Evolution

**Command**: `zqk config evolve`

Evolves configuration from simple to enterprise:

**Evolution Process**:

#### Step 1: Current State Assessment
```bash
# Assess current configuration state
zqk config evolve --assess

# Output:
# Current Configuration:
#   - Type: minimal
#   - Features: storage, objects, cli, config
#   - Backend: file
#   - State: embedded
#   - Roles: 2 (admin, developer)
#   - Repositories: 1
#   
# Growth Indicators:
#   - Team Size: 5 → 15 (200% growth)
#   - Repositories: 1 → 5 (400% growth)
#   - Operations: 100/day → 1000/day (900% growth)
#   
# Recommendation: Evolve to standard configuration
```

**Configuration Assessment Object**:
```yaml
id: CONFIG-ASSESS-001
kind: configuration_assessment
title: "Configuration Evolution Assessment"
current_configuration:
  type: minimal
  features: ["storage", "objects", "cli", "config"]
  backend: file
  state: embedded
  roles: 2
  repositories: 1

growth_indicators:
  - metric: "team_size"
    current: 5
    previous: 2
    growth_rate: 150
    trend: "increasing"
  
  - metric: "repositories"
    current: 5
    previous: 1
    growth_rate: 400
    trend: "increasing"
  
  - metric: "operations_per_day"
    current: 1000
    previous: 100
    growth_rate: 900
    trend: "increasing"

recommendation:
  target_configuration: standard
  priority: high
  estimated_impact: medium
  migration_complexity: low
```

#### Step 2: Evolution Planning
```bash
# Plan configuration evolution
zqk config evolve --plan --target standard

# Output:
# Evolution Plan:
#   - Current: minimal
#   - Target: standard
#   - Steps:
#     1. Enable scheduler (background jobs)
#     2. Enable metrics (observability)
#     3. Enable discovery (project discovery)
#     4. Enable alignment (strategic alignment)
#   - Estimated Time: 15 minutes
#   - Rollback: Available
```

**Evolution Plan Object**:
```yaml
id: EVOLVE-PLAN-001
kind: evolution_plan
title: "Evolution from Minimal to Standard"
from_configuration: minimal
to_configuration: standard
steps:
  - step: 1
    action: "enable_feature"
    feature: "scheduler"
    impact: "low"
    estimated_time: "5 minutes"
  
  - step: 2
    action: "enable_feature"
    feature: "metrics"
    impact: "low"
    estimated_time: "3 minutes"
  
  - step: 3
    action: "enable_feature"
    feature: "discovery"
    impact: "medium"
    estimated_time: "5 minutes"
  
  - step: 4
    action: "enable_feature"
    feature: "alignment"
    impact: "medium"
    estimated_time: "2 minutes"

total_estimated_time: "15 minutes"
rollback_available: true
checkpoint_created: true
```

#### Step 3: Evolution Execution
```bash
# Execute configuration evolution
zqk config evolve --execute --plan EVOLVE-PLAN-001 --confirm

# Output:
# Evolution: executing
# Checkpoint: created (evolve-checkpoint-001)
# Step 1/4: Enabling scheduler... ✓
# Step 2/4: Enabling metrics... ✓
# Step 3/4: Enabling discovery... ✓
# Step 4/4: Enabling alignment... ✓
# Evolution: complete
# Configuration: standard
# Time Taken: 12 minutes
```

#### Step 4: Rapid Enterprise Evolution
```bash
# Rapid evolution to enterprise (for startups)
zqk config evolve --rapid --target enterprise

# Output:
# Rapid Evolution: starting
# Current: standard
# Target: enterprise
# 
# Enterprise Features:
#   - Graph backend (requires MemGraph)
#   - Multi-instance coordination
#   - Techscape discovery
#   - Federation
#   - Portfolio management
#   - Audit logging
# 
# Prerequisites:
#   - MemGraph installed and running
#   - Enterprise PKI configured
#   - Multiple repositories available
# 
# Estimated Time: 30 minutes
# Continue? (y/n)
```

### 4. Alignment Opportunity Detection

**Command**: `zqk align (PRUNED)ment detect-opportunities`

Proactively detects and prompts alignment opportunities:

**Detection Process**:

#### Step 1: Opportunity Scanning
```bash
# Scan for alignment opportunities
zqk align (PRUNED)ment detect-opportunities --scan

# Output:
# Alignment Opportunities Detected:
#   1. Overlapping Concerns (High Priority)
#      - Repositories: repo:data-science, repo:business-intelligence
#      - Concern: technical-spend
#      - Impact: 2 goals, 12 backlog items
#      - Recommendation: Merge into shared technical-spend goal
#      - Estimated Benefit: 30% cost reduction, better visibility
#   
#   2. Policy Redundancy (Medium Priority)
#      - Policies: POL-DS-001, POL-BI-001
#      - Issue: Both enforce similar code review requirements
#      - Recommendation: Merge into enterprise policy
#      - Estimated Benefit: Reduced maintenance, consistency
#   
#   3. Goal Misalignment (Low Priority)
#      - Goals: GOAL-DS-001, GOAL-BI-001
#      - Issue: Similar objectives but separate tracking
#      - Recommendation: Align goals or merge
#      - Estimated Benefit: Better strategic clarity
```

**Alignment Opportunity Object**:
```yaml
id: ALIGN-OPP-001
kind: alignment_opportunity
title: "Overlapping Technical Spend Concern"
opportunity_type: overlapping_concern
priority: high
repositories:
  - repo:data-science
  - repo:business-intelligence
concern: technical-spend
impact:
  goals_affected: 2
  backlog_items_affected: 12
  policies_affected: 2
recommendation:
  action: merge_goals
  target: GOAL-TECH-001
  estimated_benefit: "30% cost reduction, better visibility"
  estimated_effort: "2 hours"
detected_at: "2025-12-30T10:00:00Z"
status: pending_review
```

#### Step 2: Opportunity Prompting
```bash
# Prompt human about alignment opportunities
zqk align (PRUNED)ment prompt --opportunity ALIGN-OPP-001

# Output:
# ⚠️  Alignment Opportunity Detected
# 
# Type: Overlapping Concerns
# Priority: High
# 
# Repositories: repo:data-science, repo:business-intelligence
# Concern: technical-spend
# 
# Current State:
#   - Data Science: GOAL-DS-001 (tracking $50K spend)
#   - Business Intelligence: GOAL-BI-001 (tracking $30K spend)
#   - Total: $80K (but tracked separately)
# 
# Recommended Action:
#   - Merge into shared GOAL-TECH-001
#   - Benefits: 30% cost reduction, better visibility, unified tracking
#   - Effort: 2 hours
# 
# Would you like to:
#   1. Review details (zqk align (PRUNED)ment show ALIGN-OPP-001)
#   2. Create merge plan (zqk align (PRUNED)ment plan --opportunity ALIGN-OPP-001)
#   3. Dismiss (zqk align (PRUNED)ment dismiss ALIGN-OPP-001)
#   4. Schedule for later (zqk align (PRUNED)ment schedule ALIGN-OPP-001 --date 2026-01-15)
```

#### Step 3: Opportunity Notification
```bash
# Set up notifications for alignment opportunities
zqk align (PRUNED)ment notify --enable --priority high

# Output:
# Notifications: enabled
# Priority: high
# Channels: terminal, email
# Frequency: daily
# 
# You will be notified when:
#   - High-priority alignment opportunities are detected
#   - Overlapping concerns are discovered
#   - Policy redundancies are found
#   - Goal misalignments are identified
```

**Notification Configuration**:
```yaml
# .zqk/config.yaml
alignment:
  notifications:
    enabled: true
    priority: high
    channels:
      - terminal
      - email
    frequency: daily
  
  detection:
    enabled: true
    scan_interval: 3600  # 1 hour
    auto_prompt: true
    min_priority: medium
```

### 5. Role Template System

**Role Templates**:

#### Template 1: Individual Developer
```yaml
# Role template for individual developer
roles:
  - role_id: "owner"
    permissions: ["read:*", "write:*", "delete:*"]
    description: "Full control for solo developer"
```

#### Template 2: Startup (5-20 people)
```yaml
# Role template for startup
roles:
  - role_id: "admin"
    permissions: ["read:*", "write:*", "delete:*"]
    user_count: 1-2
    description: "Founders/CTO"
  
  - role_id: "developer"
    permissions: ["read:*", "write:backlog_item", "write:milestone"]
    user_count: 3-15
    description: "Engineering team"
  
  - role_id: "viewer"
    permissions: ["read:*"]
    user_count: 0-5
    description: "Stakeholders"
```

#### Template 3: Enterprise (100+ people)
```yaml
# Role template for enterprise
roles:
  - role_id: "enterprise_admin"
    permissions: ["read:*", "write:*", "delete:*"]
    user_count: 1-5
    description: "Enterprise administrators"
  
  - role_id: "team_admin"
    permissions: ["read:*", "write:*", "delete:*"]
    scope: team_specific
    user_count: 5-20
    description: "Team administrators"
  
  - role_id: "developer"
    permissions: ["read:*", "write:backlog_item", "write:milestone"]
    user_count: 50-200
    description: "Developers"
  
  - role_id: "viewer"
    permissions: ["read:*"]
    user_count: 20-100
    description: "Stakeholders"
  
  - role_id: "automation"
    permissions: ["read:*", "write:backlog_item"]
    user_count: 10-50
    description: "CI/CD and automation"
```

**Template Application**:
```bash
# Apply role template
zqk role apply-template --template startup

# Output:
# Template: applied
# Roles Created: 4
#   - admin (1 user)
#   - developer (4 users)
#   - viewer (0 users)
#   - automation (0 users)
```

### 6. Progressive Configuration Templates

**Configuration Evolution Paths**:

#### Path 1: Individual → Startup
```bash
# Evolve from individual to startup
zqk config evolve --from individual --to startup

# Changes:
#   - Add scheduler (background jobs)
#   - Add metrics (observability)
#   - Add discovery (project discovery)
#   - Add alignment (strategic alignment)
#   - Add roles (team roles)
```

#### Path 2: Startup → Enterprise
```bash
# Evolve from startup to enterprise
zqk config evolve --from startup --to enterprise

# Changes:
#   - Switch to graph backend
#   - Enable multi-instance coordination
#   - Enable techscape discovery
#   - Enable federation
#   - Enable portfolio management
#   - Enable audit logging
```

#### Path 3: Rapid Enterprise (Startup Growth)
```bash
# Rapid evolution for fast-growing startup
zqk config evolve --rapid --target enterprise --prerequisites-check

# Output:
# Prerequisites Check:
#   ✓ MemGraph installed
#   ✓ Enterprise PKI configured
#   ✓ Multiple repositories available
#   ✗ Multiple teams (recommended but not required)
# 
# Rapid Evolution: ready
# Estimated Time: 30 minutes
# Continue? (y/n)
```

## Implementation

### Phase 1: Role Discovery

```go
// pkg/role/discovery.go
type RoleDiscoverer struct {
    storage ObjectStorageProvider
    analyzer *UsageAnalyzer
}

func (rd *RoleDiscoverer) Discover(context *ProjectContext) (*RoleRecommendation, error) {
    // 1. Analyze project context
    contextAnalysis := rd.AnalyzeContext(context)
    
    // 2. Analyze usage patterns
    usageAnalysis := rd.analyzer.AnalyzeUsage()
    
    // 3. Generate recommendations
    recommendations := rd.GenerateRecommendations(contextAnalysis, usageAnalysis)
    
    return recommendations, nil
}
```

### Phase 2: CLI Instance Registration

```go
// pkg/instance/registration.go
type InstanceRegistrar struct {
    pki PKISystem
    storage ObjectStorageProvider
}

func (ir *InstanceRegistrar) Register(instanceInfo *InstanceInfo) (*Instance, error) {
    // 1. Generate instance ID
    instanceID := ir.GenerateInstanceID()
    
    // 2. Request certificate
    certRequest := ir.RequestCertificate(instanceID, instanceInfo.Permissions)
    
    // 3. Issue certificate (if auto-approve enabled)
    certificate := ir.IssueCertificate(certRequest)
    
    // 4. Register instance
    instance := ir.CreateInstance(instanceID, certificate)
    
    return instance, nil
}
```

### Phase 3: Configuration Evolution

```go
// pkg/config/evolution.go
type ConfigurationEvolver struct {
    storage ObjectStorageProvider
    featureManager *FeatureManager
}

func (ce *ConfigurationEvolver) Evolve(from, to string) (*EvolutionPlan, error) {
    // 1. Assess current state
    assessment := ce.AssessCurrentState()
    
    // 2. Create evolution plan
    plan := ce.CreateEvolutionPlan(from, to, assessment)
    
    // 3. Execute evolution
    if err := ce.ExecuteEvolution(plan); err != nil {
        return nil, err
    }
    
    return plan, nil
}
```

### Phase 4: Alignment Opportunity Detection

```go
// pkg/alignment/opportunity_detector.go
type AlignmentOpportunityDetector struct {
    storage ObjectStorageProvider
    analyzer *AlignmentAnalyzer
}

func (aod *AlignmentOpportunityDetector) Detect() ([]*AlignmentOpportunity, error) {
    // 1. Scan for overlapping concerns
    overlapping := aod.ScanOverlappingConcerns()
    
    // 2. Scan for policy redundancies
    redundancies := aod.ScanPolicyRedundancies()
    
    // 3. Scan for goal misalignments
    misalignments := aod.ScanGoalMisalignments()
    
    // 4. Prioritize opportunities
    opportunities := aod.Prioritize(overlapping, redundancies, misalignments)
    
    return opportunities, nil
}
```

## Use Cases

### Use Case 1: Individual Developer

**Initial Setup**:
```bash
zqk system init --template minimal
zqk role discover --analyze
# Output: Recommended: owner role (full control)
zqk role apply-template --template individual
```

**Growth to Team**:
```bash
zqk config evolve --assess
# Output: Recommendation: Evolve to startup configuration
zqk config evolve --execute --target startup
zqk role apply-template --template startup
```

### Use Case 2: Startup Rapid Growth

**Initial Setup**:
```bash
zqk system init --template startup
zqk instance register --generate-id
zqk role apply-template --template startup
```

**Rapid Enterprise Evolution**:
```bash
zqk config evolve --rapid --target enterprise
# Output: Prerequisites check, evolution plan, execution
```

### Use Case 3: Alignment Opportunity

**Detection**:
```bash
zqk align (PRUNED)ment detect-opportunities --scan
# Output: Overlapping concern detected
```

**Human Prompt**:
```bash
zqk align (PRUNED)ment prompt --opportunity ALIGN-OPP-001
# Output: Interactive prompt with options
```

**Action**:
```bash
zqk align (PRUNED)ment plan --opportunity ALIGN-OPP-001
zqk align (PRUNED)ment execute --plan ALIGN-PLAN-001
```

## Benefits

1. **Role Discovery**: Automatically discover needed roles and privileges
2. **PKI Integration**: All CLI instances registered with PKI
3. **Rapid Evolution**: Quick migration from simple to enterprise
4. **Proactive Detection**: System detects and prompts alignment opportunities
5. **Human Guidance**: Clear prompts guide decisions
6. **Template System**: Pre-configured templates for common scenarios

## Related Documentation

- [Authority Resolution and Collaboration](./authority-resolution-and-collaboration-v1.0.md)
- [Modular Scalable Architecture](./modular-scalable-architecture-v1.0.md)
- [Distributed Kernel Architecture](./distributed-kernel-architecture-v1.0.md)

---

*This system ensures roles and privileges are discovered appropriately, CLI instances are registered with PKI, configurations evolve rapidly from simple to enterprise, and alignment opportunities are proactively detected and prompted to humans.*

