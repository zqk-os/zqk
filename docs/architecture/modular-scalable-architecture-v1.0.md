> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Modular and Scalable Architecture v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: Storage Backend Detection v1.0, Knowledge Kernel Separation v1.0, Multi-Instance Policy Reconciliation v1.0

## Overview

This document defines a modular, scalable architecture for zqk that supports growth and contraction, avoids bloat, and enables managing zqk state in separate repositories from code repositories. The architecture ensures system health and goal alignment across repository boundaries.

## Problem Statement

**Requirements**:

1. **Modularity**: Components should be modular and allow for scale and growth changes over time (both expansion and contraction)
2. **Avoid Bloat**: System should not add confusion and complexity when unnecessary
3. **Avoid Constriction**: System should not be too narrow and constrict growth or expansion
4. **Separate State Repository**: Manage zqk state in its own repository while working on code in another repository
5. **Backend Configurations**: Common backend configurations that maintain system health and keep code projects aligned with goals

**Challenges**:

- How to enable/disable features based on needs?
- How to manage state separately from code?
- How to maintain alignment when repositories are separate?
- How to scale up without bloat?
- How to scale down without losing functionality?

## Solution: Modular Architecture with Feature Flags and Separate State Management

### Core Principles

1. **Progressive Enhancement**: Start minimal, add features as needed
2. **Feature Flags**: Enable/disable features via configuration
3. **Separate State Repository**: State can live in separate repository from code
4. **Health Checks**: System health maintained across repository boundaries
5. **Goal Alignment**: Alignment mechanisms work across repositories
6. **Backend Abstraction**: Common backend configurations for different scenarios

## Architecture

### 1. Modular Component System

**Component Categories**:

#### Core Components (Always Enabled)
- **Storage Backend**: File or graph backend (one at a time)
- **Object System**: Core object types and validation
- **CLI Interface**: Basic CLI commands
- **Configuration**: Configuration management

#### Optional Components (Feature Flags)
- **Scheduler**: Background jobs and lifecycle hooks
- **MCP Server**: MCP protocol support
- **Metrics**: Observability and metrics collection
- **Policy Engine**: Policy enforcement
- **Lifecycle System**: Lifecycle state management
- **Multi-Instance**: Multi-instance coordination
- **Techscape**: Techscape discovery and reconciliation
- **Discovery**: Project discovery wizard
- **Alignment**: Strategic alignment system

**Feature Flag Configuration**:
```yaml
# .zqk/config.yaml
features:
  # Core (always enabled)
  storage: true
  objects: true
  cli: true
  config: true
  
  # Optional (enable as needed)
  scheduler: false          # Enable for background jobs
  mcp_server: false        # Enable for MCP protocol
  metrics: false            # Enable for observability
  policy_engine: true       # Enable for policy enforcement
  lifecycle: true           # Enable for lifecycle management
  multi_instance: false     # Enable for multi-instance coordination
  techscape: false          # Enable for techscape discovery
  discovery: false          # Enable for project discovery wizard
  alignment: false          # Enable for strategic alignment
  
  # Advanced (for enterprise)
  federation: false         # Enable for instance federation
  portfolio: false          # Enable for portfolio management
  audit: false              # Enable for audit logging
```

**Component Initialization**:
```go
// Components are initialized based on feature flags
type ComponentManager struct {
    features map[string]bool
    components map[string]Component
}

func (cm *ComponentManager) Initialize(config *Config) error {
    // Core components always initialized
    cm.initializeCore()
    
    // Optional components initialized based on flags
    if config.Features.Scheduler {
        cm.initializeScheduler()
    }
    if config.Features.MCP {
        cm.initializeMCPServer()
    }
    // ... etc
}
```

### 2. Separate State Repository Support

**Configuration**: State repository separate from code repository

**State Repository Structure**:
```
zqk-state-repo/
├── .zqk/                    # zqk configuration
│   ├── config.yaml           # Feature flags, backend config
│   └── state/                 # System state
│       ├── cache/             # Object ID cache
│       └── metrics/           # Command metrics
│
├── .zqk/process/              # Object storage
│   ├── goals/                # Goal objects
│   ├── milestones/           # Milestone objects
│   ├── backlog/              # Backlog items
│   ├── policies/             # Policy objects
│   └── ...
│
└── README.md                  # State repository documentation
```

**Code Repository Structure**:
```
code-repo/
├── src/                       # Application code
├── tests/                     # Test code
├── .zqk-link.yaml           # Link to state repository
└── README.md                  # Code repository documentation
```

**State Repository Link**:
```yaml
# code-repo/.zqk-link.yaml
state_repository:
  url: "https://github.com/org/zqk-state"
  path: "zqk-state-repo"
  branch: "main"
  
code_repository:
  url: "https://github.com/org/code"
  path: "code-repo"
  branch: "main"

alignment:
  enabled: true
  health_check_interval: 300  # 5 minutes
  goal_sync: true
  policy_sync: false
```

**Initialization with Separate State**:
```bash
# Initialize zqk in code repository with separate state
zqk system init \
  --state-repo "https://github.com/org/zqk-state" \
  --state-branch "main"

# This creates .zqk-link.yaml in code repository
# zqk commands will use state from separate repository
```

### 3. Backend Configurations

**Common Backend Configurations**:

#### Configuration 1: Minimal (Individual Developer)
```yaml
# .zqk/config.yaml
project:
  name: "my-project"
  complexity: minimal

storage:
  backend: file
  data_dir: .zqk/process

features:
  scheduler: false
  mcp_server: false
  metrics: false
  policy_engine: true
  lifecycle: true
  multi_instance: false
  techscape: false
  discovery: false
  alignment: false

state_repository:
  mode: embedded  # State in same repo as code
```

#### Configuration 2: Standard (Small Team)
```yaml
# .zqk/config.yaml
project:
  name: "team-project"
  complexity: standard

storage:
  backend: file
  data_dir: .zqk/process

features:
  scheduler: true
  mcp_server: false
  metrics: true
  policy_engine: true
  lifecycle: true
  multi_instance: false
  techscape: false
  discovery: true
  alignment: true

state_repository:
  mode: embedded  # State in same repo as code
```

#### Configuration 3: Separate State (Enterprise)
```yaml
# .zqk/config.yaml
project:
  name: "enterprise-project"
  complexity: enterprise

storage:
  backend: graph
  graph_host: "memgraph.internal"
  graph_port: 7687

features:
  scheduler: true
  mcp_server: true
  metrics: true
  policy_engine: true
  lifecycle: true
  multi_instance: true
  techscape: true
  discovery: true
  alignment: true
  federation: true
  portfolio: true
  audit: true

state_repository:
  mode: separate
  url: "https://github.enterprise.com/org/zqk-state"
  branch: "main"
  sync_interval: 300  # 5 minutes
```

#### Configuration 4: Multi-Repository (Portfolio)
```yaml
# .zqk/config.yaml
project:
  name: "portfolio-management"
  complexity: enterprise

storage:
  backend: graph
  graph_host: "memgraph.internal"
  graph_port: 7687

features:
  scheduler: true
  mcp_server: true
  metrics: true
  policy_engine: true
  lifecycle: true
  multi_instance: true
  techscape: true
  discovery: true
  alignment: true
  federation: true
  portfolio: true
  audit: true

state_repository:
  mode: separate
  url: "https://github.enterprise.com/org/zqk-state"
  branch: "main"
  
code_repositories:
  - url: "https://github.enterprise.com/org/frontend"
    branch: "main"
    role: "frontend"
  - url: "https://github.enterprise.com/org/backend"
    branch: "main"
    role: "backend"
```

### 4. System Health Across Repositories

**Health Check System**:

**Command**: `zqk system health`

Checks system health across repository boundaries:

```bash
# Check health of state repository
zqk system health --state-repo

# Check health of code repository
zqk system health --code-repo

# Check health of alignment between repositories
zqk system health --alignment

# Check health of all linked repositories
zqk system health --all
```

**Health Check Components**:

1. **State Repository Health**:
   - Object integrity (hashes, references)
   - Cache consistency
   - Storage backend connectivity
   - Policy compliance

2. **Code Repository Health**:
   - Code structure
   - Build system health
   - Test coverage
   - Documentation

3. **Alignment Health**:
   - Goal alignment (code work traces to goals)
   - Policy compliance (code follows policies)
   - Reference integrity (code references valid objects)
   - Sync status (state and code are in sync)

**Health Check Configuration**:
```yaml
# .zqk/config.yaml
health:
  enabled: true
  check_interval: 300  # 5 minutes
  checks:
    state_repository: true
    code_repository: true
    alignment: true
    sync_status: true
  
  thresholds:
    alignment_score: 0.80  # 80% alignment required
    policy_compliance: 0.90  # 90% compliance required
    reference_integrity: 1.0  # 100% integrity required
```

### 5. Goal Alignment Across Repositories

**Alignment System**:

**Command**: `zqk system align --cross-repo` (PRUNED)

Ensures code work aligns with goals in state repository:

```bash
# Check alignment between state and code repositories
zqk system align --cross-repo (PRUNED)

# Show alignment gaps
zqk system align --cross-repo --gaps (PRUNED)

# Sync alignment (update state based on code)
zqk system align --cross-repo --sync (PRUNED)
```

**Alignment Mechanisms**:

1. **Code-to-Goal Mapping**:
   - Code commits reference goals
   - PR descriptions link to goals
   - Issue tracking links to goals

2. **Goal-to-Code Mapping**:
   - Goals reference code repositories
   - Goals reference specific commits/PRs
   - Goals track code changes

3. **Bidirectional Sync**:
   - Code changes update goal progress
   - Goal changes trigger code work
   - Alignment validated on both sides

**Alignment Configuration**:
```yaml
# .zqk-link.yaml
alignment:
  enabled: true
  sync_mode: bidirectional  # unidirectional, bidirectional
  
  code_to_goal:
    enabled: true
    patterns:
      - pattern: "\\[GOAL-\\d+\\]"
        type: commit_message
      - pattern: "goal:GOAL-\\d+"
        type: pr_description
      - pattern: "closes #\\d+"
        type: issue_tracking
  
  goal_to_code:
    enabled: true
    track_commits: true
    track_prs: true
    track_issues: true
```

### 6. Progressive Enhancement Model

**Growth Path**:

#### Stage 1: Minimal (Individual Developer)
```yaml
features:
  scheduler: false
  mcp_server: false
  metrics: false
  policy_engine: true
  lifecycle: true
```
**Use Case**: Simple project management, basic policies

#### Stage 2: Standard (Small Team)
```yaml
features:
  scheduler: true
  mcp_server: false
  metrics: true
  policy_engine: true
  lifecycle: true
  discovery: true
  alignment: true
```
**Use Case**: Team collaboration, goal tracking, policy enforcement

#### Stage 3: Advanced (Enterprise)
```yaml
features:
  scheduler: true
  mcp_server: true
  metrics: true
  policy_engine: true
  lifecycle: true
  multi_instance: true
  techscape: true
  discovery: true
  alignment: true
  federation: true
  portfolio: true
  audit: true
```
**Use Case**: Multi-team coordination, portfolio management, audit compliance

**Contraction Path**:

**Disable Features**:
```bash
# Disable unused features
zqk config set features.scheduler false
zqk config set features.mcp_server false
zqk config set features.metrics false

# System automatically removes unused components
```

**Feature Impact Analysis**:
```bash
# Check what will be affected by disabling a feature
zqk config analyze --disable scheduler

# Output:
# - Scheduler jobs will stop running
# - Lifecycle hooks will not trigger
# - Background jobs will not execute
# - No data loss (jobs can be re-enabled)
```

### 7. Component Lifecycle Management

**Component States**:

1. **Enabled**: Component is active and initialized
2. **Disabled**: Component is not initialized (saves resources)
3. **Deprecated**: Component is marked for removal (warnings shown)
4. **Removed**: Component is no longer available

**Component Dependencies**:
```yaml
# Component dependency graph
components:
  scheduler:
    depends_on: [lifecycle, storage]
    optional_depends_on: [metrics]
  
  mcp_server:
    depends_on: [storage, objects]
    optional_depends_on: [metrics]
  
  alignment:
    depends_on: [storage, objects, policy_engine]
    optional_depends_on: [techscape, multi_instance]
```

**Automatic Dependency Resolution**:
```bash
# Enable alignment (automatically enables dependencies)
zqk config set features.alignment true

# System automatically enables:
# - storage (core, already enabled)
# - objects (core, already enabled)
# - policy_engine (required dependency)
```

### 8. Backend Configuration Templates

**Template System**:

**Command**: `zqk system init --template <template>`

Available templates:

1. **minimal**: Individual developer, embedded state
2. **standard**: Small team, embedded state
3. **enterprise**: Enterprise, separate state, graph backend
4. **portfolio**: Multi-repository, separate state, graph backend
5. **custom**: Custom configuration

**Template Examples**:

#### Minimal Template
```bash
zqk system init --template minimal
```

Creates:
- File backend
- Minimal feature set
- Embedded state repository
- Basic configuration

#### Enterprise Template
```bash
zqk system init --template enterprise \
  --state-repo "https://github.enterprise.com/org/zqk-state" \
  --graph-host "memgraph.internal" \
  --graph-port 7687
```

Creates:
- Graph backend
- Full feature set
- Separate state repository
- Enterprise configuration

### 9. State Repository Management

**State Repository Operations**:

```bash
# Link code repository to state repository
zqk state link \
  --state-repo "https://github.com/org/zqk-state" \
  --state-branch "main"

# Sync state from state repository
zqk state sync --from-remote

# Sync state to state repository
zqk state sync --to-remote

# Check state repository status
zqk state status

# Clone state repository locally
zqk state clone --state-repo "https://github.com/org/zqk-state"
```

**State Repository Health**:

```bash
# Check state repository health
zqk state health

# Output:
# State Repository: https://github.com/org/zqk-state
# Status: healthy
# Last Sync: 2025-12-30T10:00:00Z
# Objects: 150
# Integrity: 100%
# Cache: up-to-date
```

### 10. Code Repository Alignment

**Code Repository Operations**:

```bash
# Check code repository alignment with state
zqk code align --check

# Show alignment gaps
zqk code align --gaps

# Sync code repository with state
zqk code sync --from-state

# Update state based on code changes
zqk code sync --to-state
```

**Code Repository Health**:

```bash
# Check code repository health
zqk code health

# Output:
# Code Repository: https://github.com/org/code
# Status: healthy
# Alignment Score: 85%
# Policy Compliance: 90%
# Reference Integrity: 100%
# Last Sync: 2025-12-30T10:00:00Z
```

## Implementation

### Phase 1: Feature Flag System

```go
// pkg/config/features.go
type FeatureFlags struct {
    Scheduler    bool
    MCPServer    bool
    Metrics      bool
    PolicyEngine bool
    Lifecycle    bool
    MultiInstance bool
    Techscape    bool
    Discovery    bool
    Alignment    bool
    Federation   bool
    Portfolio    bool
    Audit        bool
}

func LoadFeatureFlags(config *Config) (*FeatureFlags, error) {
    // Load from .zqk/config.yaml
    // Default to minimal set
}
```

### Phase 2: Separate State Repository

```go
// pkg/state/repository.go
type StateRepository struct {
    URL    string
    Branch string
    Path   string
}

func (sr *StateRepository) Link(codeRepo string) error {
    // Create .zqk-link.yaml in code repository
}

func (sr *StateRepository) Sync(direction SyncDirection) error {
    // Sync state between repositories
}
```

### Phase 3: Health Check System

```go
// pkg/health/checker.go
type HealthChecker struct {
    StateRepo *StateRepository
    CodeRepo  *CodeRepository
}

func (hc *HealthChecker) CheckAll() (*HealthReport, error) {
    // Check state repository health
    // Check code repository health
    // Check alignment health
}
```

### Phase 4: Alignment System

```go
// pkg/alignment/cross_repo.go
type CrossRepoAlignment struct {
    StateRepo *StateRepository
    CodeRepo  *CodeRepository
}

func (cra *CrossRepoAlignment) Check() (*AlignmentReport, error) {
    // Check goal alignment
    // Check policy compliance
    // Check reference integrity
}
```

## Use Cases

### Use Case 1: Individual Developer (Minimal)

**Setup**:
```bash
zqk system init --template minimal
```

**Configuration**:
- File backend
- Minimal features
- Embedded state

**Growth Path**:
- Add scheduler when needed
- Add metrics when needed
- Add discovery when needed

### Use Case 2: Small Team (Standard)

**Setup**:
```bash
zqk system init --template standard
```

**Configuration**:
- File backend
- Standard features
- Embedded state

**Growth Path**:
- Add MCP server for AI agents
- Add multi-instance for coordination
- Add techscape for discovery

### Use Case 3: Enterprise (Separate State)

**Setup**:
```bash
zqk system init --template enterprise \
  --state-repo "https://github.enterprise.com/org/zqk-state" \
  --graph-host "memgraph.internal"
```

**Configuration**:
- Graph backend
- Full features
- Separate state repository

**Operations**:
- State managed in separate repository
- Code repositories link to state
- Health checks across repositories
- Alignment maintained automatically

### Use Case 4: Portfolio (Multi-Repository)

**Setup**:
```bash
zqk system init --template portfolio \
  --state-repo "https://github.enterprise.com/org/zqk-state" \
  --code-repos "frontend,backend,mobile"
```

**Configuration**:
- Graph backend
- Full features
- Separate state repository
- Multiple code repositories

**Operations**:
- Single state repository for all code repos
- Portfolio-level visibility
- Cross-repository alignment
- Unified health checks

## Benefits

1. **Modularity**: Components can be enabled/disabled as needed
2. **Scalability**: System grows/shrinks based on needs
3. **Flexibility**: Supports embedded and separate state repositories
4. **Health**: System health maintained across repositories
5. **Alignment**: Goals and policies aligned across repositories
6. **Simplicity**: Minimal configuration for simple use cases
7. **Power**: Full features for complex use cases

## Future Enhancements

1. **Dynamic Feature Loading**: Load features at runtime
2. **Feature Marketplace**: Community-contributed features
3. **Auto-Scaling**: Automatically enable features based on usage
4. **Feature Analytics**: Track feature usage and impact
5. **Migration Tools**: Migrate between configurations
6. **Configuration Validation**: Validate configuration before applying

## Related Documentation

- [Storage Backend Detection](./storage-backend-detection-v1.0.md)
- [Knowledge Kernel Separation](./knowledge-kernel-separation-v1.0.md)
- [Multi-Instance Policy Reconciliation](./multi-instance-policy-reconciliation-v1.0.md)
- [Project Discovery and Strategic Alignment](./project-discovery-and-strategic-alignment-v1.0.md)

---

*This architecture ensures zqk remains modular, scalable, and flexible, supporting both simple individual projects and complex enterprise portfolios while maintaining system health and goal alignment across repository boundaries.*

