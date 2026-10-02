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
| [codegen](./codegen/) | `github.com/zqk-os/zqk/internal/codegen` | 0+1 | 1 | ast, generators, macro | ❌ - | Automated code generation, schema-to-Go binding synthesis, and spec scaffolding. |
| [distribution](./distribution/) | `github.com/zqk-os/zqk/internal/distribution` | 0+1 | 1 | - | ❌ - | Release packaging, archive bundling, and distribution asset compilation. |
| [stamping](./stamping/) | `github.com/zqk-os/zqk/internal/stamping` | 1+1 | 1 | - | ❌ - | Binary build stamping, version metadata injection, and build environment provenance. |

## Package Structure

```
internal/
├── codegen/          # Automated code generation, schema-to-Go binding synthesis, a
│   └── ast/
│   └── generators/
│   └── macro/
├── distribution/          # Release packaging, archive bundling, and distribution asset 
├── stamping/          # Binary build stamping, version metadata injection, and build
```

## Usage

Import packages using their canonical import path:

```go
import "github.com/zqk-os/zqk/internal/codegen"
```

## Related Documentation

- [Project README](../README.md) - Project overview
- [Architecture Docs](../docs/architecture/) - System architecture
- [Verifiable Decomposition Spine](../docs/architecture/VERIFIABLE_DECOMPOSITION_SPINE.md) - Done-gates and contracts

---

*This README was auto-generated. Packages are discovered dynamically from the file system.*
