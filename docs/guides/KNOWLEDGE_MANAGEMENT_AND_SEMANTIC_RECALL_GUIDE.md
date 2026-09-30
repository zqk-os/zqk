# Developer & Agent Guide: Knowledge Management, Vocabularies, Libraries, and On-Demand Semantic Recall

## 1. Executive Summary & Architectural Motivation

In complex engineering organizations and multi-agent AI swarms, the single greatest failure mode is **semantic entropy**:
- AI agents hallucinate command flags, invent non-existent APIs, and guess system behaviors when context is omitted.
- Human engineers and agents operate with divergent mental models of shared terms (e.g., what constitutes a "completed" task, a "draft" membrane, or an "authoritative" state).
- Architecture and specification documents gather dust in disconnected wikis, completely decoupled from active backlog items and code validation gates.
- Shipped documentation drifts from the code, with broken command examples and false attributions.

The **ZQK Knowledge Kernel** solves this crisis by elevating documentation, taxonomies, glossaries, and specification libraries into **first-class, cryptographically verified kernel objects**. State is not an ephemeral string in an LLM memory buffer; it is an immutable, Content-Addressed Storage (CAS) entity indexed in the system knowledge graph.

This guide provides developers, systems architects, and autonomous AI agents with the foundational protocols for:
1. **Operating the Vocabulary Pack** (`glossary_term`, `vocabulary_scheme`, `glossary_term_relation`, `import_tracking`).
2. **Managing the Library & Documentation Pack** (`doc_entry`, `library`, `technical_spec`).
3. **Automating Documentation Lifecycle & Integrity** via `zqk docman register` and `zqk docman verify`.
4. **Executing On-Demand Semantic Recall** at runtime to disambiguate terms, fetch machine hints, and execute agent instructions without polluting context windows.
5. **Enforcing Feature-to-Documentation Traceability** (`doc_entry_refs` and `POL-DOC-001`).

---

## 2. The Semantic Vocabulary System (`packs/vocabulary/`)

The Vocabulary Pack establishes a shared, deterministic ontology across human engineers, AI agents, and automated tools.

```
┌────────────────────────────────────────────────────────────────────────┐
│                   VOCABULARY SCHEME (VOC-*)                            │
│  Defines a lens, taxonomy, or navigation graph (e.g. CLI, Arch, VDS)   │
└──────────────────────────────────┬─────────────────────────────────────┘
                                   │ scopes
                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│               GLOSSARY TERM RELATION (GTR-*)                           │
│  source_term_ref ──[ predicate_ref (e.g. broader/narrower) ]──▶ target │
└──────────────────────────────────┬─────────────────────────────────────┘
                                   │ connects
                                   ▼
┌────────────────────────────────────────────────────────────────────────┐
│                    GLOSSARY TERM (GLS-*)                               │
│  • title: "CLI Command: do"                                            │
│  • definition: "Single-command workflow loop executing an intent..."   │
│  • agent_prompts: "Execute when running single-command autonomy..."    │
│  • machine_hints: {"command":"do","spec":".zqk/cli/specs/do.yaml"}     │
│  • context_scope: operational | category: cli | semantic_tags: [...]    │
└────────────────────────────────────────────────────────────────────────┘
```

### 2.1 `glossary_term` (The Unit of Semantic Truth)
A `glossary_term` (`GLS-*`) provides an unambiguous, context-scoped definition. It does not merely define words for humans; it embeds actionable machine hints and model directives.

#### Core Field Specification:
| Field | Type | Validation / Profile | Purpose |
| :--- | :--- | :--- | :--- |
| `title` | `string` | Required, unique in scope | Canonical name of the term or concept. |
| `definition` | `string` | Required | Authoritative, formal explanation of the term. |
| `context_scope` | `string` | Required (`operational`, `architectural`, `governance`, `testing`) | Operational boundary where this definition is valid. |
| `category` | `string` | Required (`cli`, `kernel`, `storage`, `compliance`, `tpm`, `agent`) | Top-level semantic classification. |
| `agent_prompts` | `string` | Required | **Direct runtime guidance for AI models** on how to interpret, recommend, or operate the concept. |
| `machine_hints` | `string` | Required (JSON string) | **Machine-readable metadata** (source spec paths, CLI command names, AST nodes) for automation. |
| `semantic_tags` | `list` | Optional | Semantic usage flags: `ontology`, `epistemology`, `inference`, `display`, `navigation`, `predicate_definition`. |
| `alias_refs` | `list` | Optional | Kernel references to synonym or alias objects. |

#### Real-World Example (from Kernel CAS):
```yaml
schema_version: 2.0.0
id: GLS-1789901316841743000-e34e7e8b
kind: glossary_term
title: "CLI Command: agent"
category: cli
context_scope: operational
status: active
definition: "CLI command `agent` defined in `.zqk/cli/specs/agent_command.yaml`. Orchestration, task routing, and swarm tooling for kernel agents."
agent_prompts: "Use when explaining or suggesting this CLI command and its purpose."
machine_hints: '{"command_name":"agent","source_kind":"command_spec","spec_path":".zqk/cli/specs/agent_command.yaml"}'
namespace_id: zqk:kernel
```

### 2.2 `vocabulary_scheme` (Lens Networks & Taxonomies)
A `vocabulary_scheme` (`VOC-*`) defines a bounded namespace or taxonomy graph. Multiple schemes can coexist across the same glossary terms without collision:
- **`inference`**: Formal logical entailment and dependency analysis.
- **`display`**: UX and TUI Mission Control groupings.
- **`navigation`**: Documentation hierarchy and cross-linking.
- **`extension`**: External domain ontologies (e.g. SKOS, Dublin Core, ISO standards).

### 2.3 `glossary_term_relation` (Typed Semantic Edges)
Relationships between terms are explicit graph edges (`GTR-*`):
- `source_term_ref`: Originating `GLS-*` term.
- `target_term_ref`: Target `GLS-*` term.
- `predicate_ref`: Pointer to a `GLS-*` term defining the predicate semantics (e.g. `broader`, `narrower`, `related`, `governs`, `implements`).
- `scheme_ref`: Scoping `VOC-*` scheme.

---

## 3. The Library & Documentation System (`packs/library/`)

Documentation in ZQK is not an unmanaged collection of markdown files. It is an indexed, cryptographically verified subsystem managed via `docman`.

### 3.1 `doc_entry` (Cryptographically Leashed Documentation)
Every canonical markdown document is registered as a `doc_entry` (`DOC-*`) object within the kernel:

```yaml
schema_version: 2.0.0
id: DOC-001
kind: doc_entry
title: "Cellular Membrane Mode B Configuration"
target_file: "docs/architecture/CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.md"
category: "architecture"
group: "core"
status: "verified"
content_hash: "sha256:47f511e263c5dfa9372b8cb08e059cc470108a09c4880223287b44f82eaa23df"
size_bytes: 10914
last_verified_at: "2026-09-30T01:19:47Z"
```

#### Verification & Drift Detection:
- **`content_hash`**: The exact SHA-256 fingerprint of the file on disk.
- **Drift Protection**: If any agent or process alters a document without running through `docman`, `zqk docman verify` immediately flags cryptographic drift, preventing compromised documentation from entering release candidates.

### 3.2 `library` & `technical_spec` (Composable Specification Libraries)
The `library` object kind defines architectural pattern catalogs, composable components, and connector patterns. Built with the **Cloneable Configuration Pattern**, libraries enable systems to inherit, compose, and clone shared architecture DNA across multi-repo meshes.

---

## 4. Documentation Management via CLI (`zqk docman`)

ZQK provides dedicated CLI tools to discover, register, and verify documentation integrity.

### 4.1 Automated Documentation Discovery (`zqk docman register`)
When new guides, manuals, or tutorials are authored, `zqk docman register` scans the filesystem, extracts headers, infers categories, computes SHA-256 hashes, and registers `doc_entry` objects in Content-Addressed Storage:

```bash
# 1. Preview documentation to be registered (Dry Run)
zqk docman register --dry-run

# 2. Register all markdown documentation across docs/
zqk docman register

# 3. Register only shipped documentation subtrees
zqk docman register --shipped-only

# 4. Scan specific subtrees
zqk docman register --subtrees docs/guides,docs/manual

# 5. Update existing doc_entries to sync metadata changes
zqk docman register --update-existing
```

### 4.2 Cryptographic Verification & Drift Auditing (`zqk docman verify`)
The `verify` command ensures that zero undocumented mutations or corrupted files exist:

```bash
# Verify all registered doc_entry objects against files on disk
zqk docman verify

# Verify shipped documentation only
zqk docman verify --shipped-only
```

If drift is detected, `zqk docman verify` outputs exact byte-level and hash diffs:
```
Scanned 12 doc_entry object(s):
  - Passed: 11
  - Drifted: 1
Violations:
  [error] DOC-001 (docs/architecture/MEMBRANE.md): cryptographic drift detected:
          expected SHA-256 02f6c76a..., got 47f511e2...
```

---

## 5. On-Demand Semantic Recall for AI Agents & Humans

AI agents should never rely on stale training weights or speculate about system behavior. Instead, agents execute **On-Demand Semantic Recall** directly against the Knowledge Kernel.

### 5.1 Protocol: Term Disambiguation & Agent Instruction Retrieval
When an agent encounters a command, subsystem, or architectural pattern:

```bash
# 1. Look up the glossary term by title or keyword
zqk object list glossary_term --filter "title:CLI Command: do" --format yaml

# 2. Query all glossary terms in a specific operational scope
zqk object list glossary_term --filter "context_scope=operational" --fields title,definition,agent_prompts

# 3. Retrieve machine hints for tooling integration
zqk object list glossary_term --filter "category=cli" --fields title,machine_hints
```

#### Why This Eliminates Hallucinations:
- **`definition`** provides the factual semantic contract.
- **`agent_prompts`** tells the model *how to act* when handling this entity.
- **`machine_hints`** gives the exact file paths and schemas to inspect.

### 5.2 Topologically Scoped Prompt Generation (`zqk system prompt-builder`)
For automated agent swarm dispatch, the kernel provides `zqk system prompt-builder`. This command traverses the active DAG, extracts relevant glossary terms, policies, and requirements, and compiles a topologically sorted, token-optimized context prompt for LLM consumption.

```bash
# Build an agent context prompt scoped to a priority plan
zqk system prompt-builder --plan PRI-PUBLIC-LAUNCH-100 --format agent-prompt
```

### 5.3 Automated Glossary Harvesting (`zqk system sync-glossary-from-specs`)
As new specs, lifecycles, and CLI commands are introduced into the codebase, `zqk-admin system sync-glossary-from-specs` inspects all YAML schemas in `.zqk/specs/` and proposes or creates corresponding `glossary_term` objects:

```bash
# Report missing glossary candidates inferred from specs
zqk-admin system sync-glossary-from-specs --format table

# Ingest and mint missing glossary terms into CAS
zqk-admin system sync-glossary-from-specs --apply --dry-run=false
```

---

## 6. Policy Enforcement: Mandatory Documentation Traceability (`POL-DOC-001`)

Under ZQK governance, documentation written after feature delivery is considered technical debt. Feature work must generate documentation concurrently.

### 6.1 The Policy Rule: `POL-DOC-001`
- **Rule**: Every `requirement` and `criteria` must include a default criterion mandating documentation completion and must link to a valid `doc_entry` reference (`doc_entry_refs`).
- **Enforcement Pipeline**: Built directly into `zqk workflow gen-trace-pipeline` and validated fail-closed during pre-commit and `zqk system check`.

```bash
# Generate the complete ontological trace pipeline with mandatory documentation criteria
zqk workflow gen-trace-pipeline REQ-FEATURE-001
```

Generated criteria include:
```yaml
id: CRT-DOC-VERIFY
kind: criteria
category: compliance
title: "Documentation & Knowledge Base Entry: Public Core Launch"
description: "Complete feature documentation, user guides, or architectural specifications authored and linked via doc_entry reference (DOC-*)."
doc_entry_refs:
  - DOC-015
```

---

## 7. Practical Recipes & Cheatsheet

### Recipe 1: Querying Knowledge for a CLI Command
```bash
# Query how the kernel defines the 'mutate' command and its model guidance
zqk object list glossary_term --filter "title:CLI Command: mutate" \
  --fields id,title,definition,agent_prompts,machine_hints --format yaml
```

### Recipe 2: Minting a New Domain Glossary Term
```bash
zqk object create glossary_term \
  --title "Epistemic Check-Valve" \
  --fields category:kernel,context_scope:architectural,definition:"A non-bypassable preflight validation gate preventing unverified CAS mutations from entering authoritative planes.",agent_prompts:"Verify check-valve status before attempting CAS promotion.",machine_hints:'{"component":"membrane","layer":0}'
```

### Recipe 3: Registering & Verifying Shipped Documentation
```bash
# Discover and register newly written docs
zqk docman register

# Validate cryptographic integrity across all docs
zqk docman verify
```

### Recipe 4: Graph Query across Terms & Schemes (ZPARQL)
```bash
# Traverse terms related to 'storage'
zqk query "MATCH (g:glossary_term)-[:related]->(target:glossary_term) WHERE g.category = 'storage' RETURN g.title, target.title"
```
