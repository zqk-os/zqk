# Subordinate Namespaces Implementation

**Last Verified:** 2026-08-31


**Date**: 2026-01-05  
**Status**: Implemented

## Overview

Extended the existing `namespace_id` system to support **subordinate namespaces** for modular organization. Each namespace module (cli, metrics, storage, etc.) now has its own subordinate namespace ID.

## Implementation

### Profile Schema Updates

All profiles now include `namespace_id`:

**CLI Profiles** (`pkg/cli/profiles/`):
```yaml
schema_version: "1.0.0"
namespace_id: "zqk:kernel:cli"
namespace: "cli"  # Legacy field for backward compatibility
name: "base"
```

**Metrics Profiles** (`.zqk/metrics/profiles/`):
```yaml
schema_version: "1.0.0"
namespace_id: "zqk:kernel:metrics"
namespace: "metrics"  # Legacy field for backward compatibility
name: "base_sampler"
```

### Loader Updates

Both loaders now validate `namespace_id`:

**CLI Loader** (`pkg/cli/loader.go`):
- Validates `namespace_id == "zqk:kernel:cli"`
- Sets default if not provided (backward compatibility)
- Validates legacy `namespace` field

**Metrics Loader** (`pkg/metrics/loader.go`):
- Validates `namespace_id == "zqk:kernel:metrics"`
- Sets default if not provided (backward compatibility)
- Validates legacy `namespace` field

### Namespace ID Format

The existing pattern already supports unlimited depth:
```regex
^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$
```

**Examples**:
- `zqk:kernel` - Base kernel namespace
- `zqk:kernel:cli` - CLI subordinate namespace
- `zqk:kernel:metrics` - Metrics subordinate namespace
- `zqk:kernel:storage` - Storage subordinate namespace
- `domain:organizational:hr` - HR subdomain
- `domain:organizational:hr:recruiting` - Recruiting under HR

## Benefits

1. **Module Scoping**: Each module has a clear namespace ID
2. **Namespace Discovery**: Can discover all resources for a module
3. **Isolation**: Each module's resources are clearly scoped
4. **Consistency**: Uses existing namespace_id system
5. **Backward Compatible**: Legacy `namespace` field still supported

## Future Enhancements

1. **Namespace Registry Extensions**: Add methods to discover subordinate namespaces
2. **Config Organization**: Organize config files by subordinate namespace
3. **Spec Organization**: Organize object specs by subordinate namespace
4. **Query Support**: Query profiles/configs by subordinate namespace

