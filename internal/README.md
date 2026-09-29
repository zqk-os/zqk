# Internal Packages Directory

**Status**: Active  
**Last Updated**: AUTO-GENERATED - Do not edit manually

This directory contains Go packages for the ZQK project. Each package is a modular component with a defined boundary and contract.

**⚠️ This README is auto-generated. To update it, run:**
```bash
./scripts/open-core/generate-code-readme-index.sh internal
```

## Internal Packages Index

| Package | Import Path | Files | Tests | Subpackages | README | Description |
|---------|-------------|-------|-------|-------------|--------|-------------|
| [bootstrap](./bootstrap/) | `github.com/zqk-os/zqk/internal/bootstrap` | 4+6 | 6 | - | ✅ [README](./bootstrap/README.md) | Embedded bootstrap archive for zqk system init. |
| [cli](./cli/) | `github.com/zqk-os/zqk/internal/cli` | 24+14 | 14 | context, errorsuggest, flagutil | ✅ [README](./cli/README.md) | This package provides shared internal utilities, layered context resolution, and output helpers for the ZQK CLI. |
| [codegen](./codegen/) | `github.com/zqk-os/zqk/internal/codegen` | 0+1 | 1 | ast, generators, macro | ❌ - | Automated code generation, schema-to-Go binding synthesis, and spec scaffolding. |
| [distribution](./distribution/) | `github.com/zqk-os/zqk/internal/distribution` | 0+1 | 1 | - | ❌ - | Release packaging, archive bundling, and distribution asset compilation. |
| [stamping](./stamping/) | `github.com/zqk-os/zqk/internal/stamping` | 1+1 | 1 | - | ❌ - | Binary build stamping, version metadata injection, and build environment provenance. |
| [testpackageconcurrency](./testpackageconcurrency/) | `github.com/zqk-os/zqk/internal/testpackageconcurrency` | 1+1 | 1 | - | ❌ - | Concurrency test fixtures, race detection harnesses, and synchronization benchmarks. |

## Package Structure

```
internal/
├── bootstrap/          # Embedded bootstrap archive for zqk system init.
├── cli/          # This package provides shared internal utilities, layered con
│   └── context/
│   └── errorsuggest/
│   └── flagutil/
├── codegen/          # Automated code generation, schema-to-Go binding synthesis, a
│   └── ast/
│   └── generators/
│   └── macro/
├── distribution/          # Release packaging, archive bundling, and distribution asset 
├── stamping/          # Binary build stamping, version metadata injection, and build
├── testpackageconcurrency/          # Concurrency test fixtures, race detection harnesses, and syn
```

## Usage

Import packages using their canonical import path:

```go
import "github.com/zqk-os/zqk/internal/cli"
```

## Related Documentation

- [Project README](../README.md) - Project overview
- [Architecture Docs](../docs/architecture/) - System architecture
- [Verifiable Decomposition Spine](../docs/architecture/VERIFIABLE_DECOMPOSITION_SPINE.md) - Done-gates and contracts

---

*This README was auto-generated. Packages are discovered dynamically from the file system.*
