# CLI Domain Subsystem & Ontology Registration Guide

## Overview

The ZQK Domain Subsystem (`cmd/zqk/domain`) provides discovery and registration capabilities for extensible domain ontologies within the Zen Quantum Kernel. While the core kernel manages universal lifecycle primitives (such as goals, requirements, criteria, test cases, and backlog items), real-world systems require domain-specific bounded contexts, entity definitions, and vocabulary namespaces.

The domain subsystem decouples vertical application domains from the core kernel engine by managing `domain_registry` objects in the Content Addressable Storage (CAS). This ensures that custom domain models, specifications, and namespace boundaries remain queryable, verifiable, and versioned within the knowledge graph.

### Core Capabilities

1. **Domain Discovery (`zqk domain discover`)**:
   - Queries and enumerates all active domain registry objects (`domain_registry`).
   - Surfaces domain identifier mappings, titles, and attached bounded contexts.
   - Provides clear onboarding recommendations when no registries are present.

2. **Domain Registration (`zqk domain register`)**:
   - Validates domain identifiers, target namespaces, and specification files.
   - Automatically initializes sequential domain registry containers (`DOMAIN-REG-001`, `DOMAIN-REG-002`, etc.) when empty.
   - Appends new domain definitions safely into existing registry containers without data loss or race conditions.
   - Provides a zero-mutation `--dry-run` preview mode for validation before state mutation.

---

## Architectural Taxonomy & Component Layout

```
cmd/zqk/domain/
├── domain.go                               # Root 'domain' command definition & storage listing helpers
├── discover.go                             # 'domain discover' command implementation & formatter
├── register.go                             # 'domain register' command, flag bindings, & sequential ID generator
├── domain_coverage_elevation_test.go       # High-coverage unit tests, error injection & flag matrix
├── domain_integration_test.go              # Storage integration tests & multi-entry lifecycle tests
└── domain_test_layout_helpers_test.go      # Test fixture layout builders & stdout capture utilities
```

### Command Hierarchy

```
zqk
└── domain                                  # Domain ontology discovery and registration
    ├── discover                            # Discover registered domain ontologies
    └── register                            # Register a domain ontology with namespace and spec
```

---

## 1. Discovering Registered Domain Ontologies (`zqk domain discover`)

The `discover` command scans the CAS for `domain_registry` objects and prints an inventory of registered domains and their associated titles.

### Command Usage

```bash
zqk domain discover
```

### Output Formats

#### Empty State
When no domain registries exist in the active project:
```
No domain registries found.
Use 'zqk object create domain_registry --file <yaml>' to register a domain.
```

#### Populated State
When one or more domain registries are stored:
```
Discovered 2 domain registr(y/ies):
  - DOMAIN-REG-001 (Primary Enterprise Domain Registry)
  - DOMAIN-REG-002 (Telemetry & Analytics Ontology)
```

---

## 2. Registering Domain Ontologies (`zqk domain register`)

The `register` command creates or updates a domain registration entry. It supports defining custom namespace patterns and linking external YAML specification files.

### Command Syntax

```bash
zqk domain register --domain-id <id> [flags]
```

### Supported Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--domain-id` | string | **Yes** | `""` | Unique identifier for the domain being registered. |
| `--namespace` | string | No | `""` | Target namespace pattern for domain objects (e.g., `ecommerce:orders:*`). |
| `--spec-file` | string | No | `""` | Relative or absolute path to the YAML specification file describing the domain ontology. |
| `--dry-run` | bool | No | `false` | Preview registration changes without mutating kernel storage or creating registry objects. |

### Operational Examples

#### Basic Registration
Register a minimal domain with only an identifier:
```bash
zqk domain register --domain-id billing
```
*Output:*
```
Domain registered: billing
Registry: DOMAIN-REG-001
```

#### Full Registration with Namespace and Specification
Register a domain with an explicit namespace constraint and a specification schema:
```bash
zqk domain register \
  --domain-id logistics \
  --namespace "logistics:shipping:*" \
  --spec-file "specs/logistics_ontology.yaml"
```
*Output:*
```
Domain registered: logistics
Registry: DOMAIN-REG-001
```

#### Dry-Run Preview
Validate domain parameters without modifying persistent storage:
```bash
zqk domain register \
  --domain-id telemetry \
  --namespace "telemetry:events:*" \
  --spec-file "specs/telemetry.yaml" \
  --dry-run
```
*Output:*
```
Dry run: would register domain "telemetry" (namespace="telemetry:events:*", spec-file="specs/telemetry.yaml")
```

---

## 3. Knowledge Kernel Object Model: `domain_registry`

Domain registrations are persisted as structured YAML objects under `.zqk/process/domain_registry/` conforming to the ZQK CAS schema:

```yaml
id: DOMAIN-REG-001
kind: domain_registry
title: Domain registry
status: active
schema_version: 2.0.0
domains:
  - id: billing
  - id: logistics
    namespace: logistics:shipping:*
    spec_file: specs/logistics_ontology.yaml
```

### Lifecycle & Persistence Characteristics

1. **Sequential Identification (`DOMAIN-REG-%03d`)**:
   - The registry identifier uses monotonic sequential formatting regex `^DOMAIN-REG-(\d+)$`.
   - When the first domain is registered, `DOMAIN-REG-001` is allocated.
2. **Atomic In-Memory & Storage Append**:
   - Subsequent calls to `zqk domain register` inspect existing registries and append domain entries directly into the `domains` array.
   - Mutations trigger memory cache invalidation via `pkgctx.WithCacheUpdate`, ensuring immediate consistency across concurrent CLI commands.
3. **Fail-Closed Input Validation**:
   - Omitting `--domain-id` immediately aborts with `--domain-id is required` before any storage interaction or transaction initialization.

---

## 4. Verification and Done-Gate Matrix

| Criterion ID | Category | Description | Verification Method |
|--------------|----------|-------------|---------------------|
| `CRIT-1792074161524619000-ceb88a37` | Acceptance | `cmd/zqk/domain` achieves $\ge 80.0\%$ statement coverage and validates CLI command hierarchy | `go test -short -cover ./cmd/zqk/domain` |
| `CRIT-1792074161524620000-8701f300` | Functional | Boundary condition validation, error paths (storage list/create/update failures), and dry-run preview | `cmd/zqk/domain/domain_coverage_elevation_test.go` |
| `CRIT-1792074161524621000-5f77f89c` | Compliance | Comprehensive documentation guide authored and committed | `docs/guides/CLI_DOMAIN_SUBSYSTEM_AND_ONTOLOGY_REGISTRATION_GUIDE.md` |
