# Internal Packages Directory

**Status**: Active  
**Last Updated**: AUTO-GENERATED - Do not edit manually

This directory contains Go packages for the ZQK project. Each package is a reusable component with a specific purpose.

**⚠️ This README is auto-generated. To update it, run:**
```bash
./scripts/generate-code-readme-index.sh internal
```

## Internal Packages Index

| Package | Import Path | Files | Tests | Subpackages | README | Description |
|---------|-------------|-------|-------|-------------|--------|-------------|
| [bootstrap](./bootstrap/) | `github.com/lanceman/zqk/internal/bootstrap` | 4+4 | 4 | - | ✅ [README](./bootstrap/README.md) | - **Build:** The build process creates `archive... |
| [cli](./cli/) | `github.com/lanceman/zqk/internal/cli` | 24+13 | 13 | context, errorsuggest, flag... | ✅ [README](./cli/README.md) | The CLI uses a layered context system with prec... |
| [testpackageconcurrency](./testpackageconcurrency/) | `github.com/lanceman/zqk/internal/testpackageconcurrency` | 1+1 | 1 | - | ❌ - | - |

## Package Structure

```
internal/
├── bootstrap/          # - **Build:** The build process creates `archive/bootstrap.tar.gz` and `archive/manifest.txt` via ...
├── cli/          # The CLI uses a layered context system with precedence:
│   └── context/
│   └── errorsuggest/
    └── flagutil/
├── testpackageconcurrency/          # 
```

## Usage

Import packages using their import path:

```go
import "github.com/lanceman/zqk/internal/bootstrap"
```

## Related Documentation

- [Project README](../README.md) - Project overview
- [Architecture Docs](../docs/process/architecture/) - System architecture

---

*This README was auto-generated. Packages are discovered dynamically from the file system.*
