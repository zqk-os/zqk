# Pre-Stage Formatting

## Problem

Git doesn't have a native "pre-stage" hook, so formatting happens at commit time. This means:
- Files with formatting issues get staged
- You see formatting changes in `git diff --staged`
- Formatting happens late in the workflow

## Solution

We provide a wrapper script that formats files **before** staging them.

## Usage

### Option 1: Use the wrapper script directly

```bash
# Format and stage specific files
./scripts/git-add-formatted.sh pkg/mcp/server.go pkg/mcp/handlers.go

# Format and stage all modified Go files
./scripts/git-add-formatted.sh
```

### Option 2: Create a Git alias (recommended)

```bash
# Create alias
git config alias.addf '!./scripts/git-add-formatted.sh'

# Use it like normal git add
git addf pkg/mcp/server.go
git addf  # Stage all modified Go files (formatted)
```

### Option 3: Editor integration (best)

Configure your editor to format on save:

**VS Code:**
```json
{
  "editor.formatOnSave": true,
  "go.formatTool": "gofmt",
  "[go]": {
    "editor.formatOnSave": true
  }
}
```

**Vim/Neovim:**
- Use `vim-go` plugin with `:GoFmt` on save
- Or use `gofumpt` for stricter formatting

## Benefits

1. **Format before staging** - Files are formatted as soon as they're added
2. **Clean diffs** - No formatting changes in staged diffs
3. **Early detection** - See formatting issues immediately
4. **Consistent workflow** - Format → Stage → Commit

## Workflow Comparison

### Without pre-stage formatting:
```bash
git add file.go          # Stage unformatted file
git commit               # Pre-commit hook formats it
# File gets reformatted, diff shows formatting changes
```

### With pre-stage formatting:
```bash
git addf file.go         # Format, then stage
# or
./scripts/git-add-formatted.sh file.go
git commit               # File already formatted, no changes
```

## Integration with Pre-Commit Hook

The pre-commit hook still runs `gofmt` as a safety net:
- If you forget to use `git addf`, pre-commit will format
- If editor didn't format on save, pre-commit will format
- Ensures all commits are formatted, even if workflow is skipped

## Best Practice

1. **Enable format-on-save in editor** (prevents issues at source)
2. **Use `git addf` or wrapper script** (formats before staging)
3. **Pre-commit hook as safety net** (catches anything missed)

This three-layer approach ensures formatting issues never accumulate.

