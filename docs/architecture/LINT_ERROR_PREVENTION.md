# Lint Error Prevention Strategy

**Last Verified:** 2026-08-31


## Problem

Lint errors (especially formatting errors like `gofmt`) accumulate over time, requiring repeated manual fixes. This slows down development and creates frustration.

## Solution

We've implemented a multi-layered approach to prevent lint errors from accumulating:

### 0. Pre-Stage Formatting (Recommended)

Format files **before** staging them using the wrapper script:

```bash
# Format and stage specific files
./scripts/git-add-formatted.sh pkg/mcp/server.go

# Format and stage all modified Go files
./scripts/git-add-formatted.sh

# Or create an alias for convenience
git config alias.addf '!./scripts/git-add-formatted.sh'
git addf  # Use like normal git add, but formats first
```

**Benefits:**
- Files are formatted before staging
- Clean diffs (no formatting changes)
- Early detection of formatting issues

See [PRE_STAGE_FORMATTING.md](./PRE_STAGE_FORMATTING.md) for details.

### 1. Pre-Commit Auto-Formatting

The pre-commit hook now **automatically formats** Go files before running lint checks:

```bash
# In tools/git-hooks/pre-commit
# 1. Auto-format staged Go files with gofmt
# 2. Re-stage formatted files
# 3. Run lint checks
```

**Benefits:**
- Formatting issues are fixed automatically
- No manual intervention needed
- Consistent formatting across all commits

### 2. Manual Formatting Script

Use `scripts/format-and-lint.sh` to format and lint before committing:

```bash
# Format and lint all packages
./scripts/format-and-lint.sh

# Format and lint specific packages
./scripts/format-and-lint.sh ./pkg/mcp ./cmd/zqk
```

**When to use:**
- Before staging files for commit
- When you see formatting warnings
- As part of your development workflow

### 3. Editor Integration

Configure your editor to auto-format on save:

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

**Benefits:**
- Catch formatting issues immediately
- No need to wait for pre-commit hook

### 4. CI/CD Linting

Lint checks run in CI/CD pipelines to catch issues before merge:

- All PRs must pass linting
- Formatting is checked automatically
- Prevents accumulation of lint errors in main branch

## Best Practices

### For Developers

1. **Format before committing:**
   ```bash
   ./scripts/format-and-lint.sh
   git add -A
   git commit
   ```

2. **Use editor auto-format:**
   - Enable format-on-save in your editor
   - This prevents formatting issues from being written

3. **Run linting locally:**
   ```bash
   golangci-lint run ./...
   ```

### For AI Agents

1. **Always format before staging:**
   - Run `gofmt -w` on modified files
   - Or use `./scripts/format-and-lint.sh`

2. **Let pre-commit hook handle it:**
   - The hook will auto-format if you forget
   - But it's faster to format before staging

3. **Check linting before commit:**
   ```bash
   # Format
   gofmt -w ./pkg/mcp
   
   # Lint
   golangci-lint run ./pkg/mcp
   
   # Then stage and commit
   git add -A
   git commit
   ```

## Common Issues and Fixes

### Issue: "File is not properly formatted (gofmt)"

**Fix:**
```bash
gofmt -w <file>
git add <file>
```

**Prevention:**
- Enable format-on-save in editor
- Run `./scripts/format-and-lint.sh` before committing

### Issue: "ineffectual assignment"

**Fix:**
- Remove unused variable assignments
- Or add `//nolint:ineffassign` comment if intentional

**Prevention:**
- Review code before committing
- Use `go vet` to catch these early

### Issue: "staticcheck warnings"

**Fix:**
- Address the warning (usually a code improvement)
- Or add `//nolint:staticcheck` if intentional

**Prevention:**
- Run `golangci-lint` locally before committing
- Fix warnings as you write code

## Tools

### Formatting Tools

- `gofmt`: Standard Go formatter (built-in)
- `gofumpt`: Stricter formatter (recommended for new code)

### Linting Tools

- `golangci-lint`: Comprehensive linter (recommended)
- `go vet`: Built-in linter (fallback)

### Installation

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install gofumpt (optional, stricter formatting)
go install mvdan.cc/gofumpt@latest
```

## Summary

The key to preventing lint error accumulation is:

1. **Auto-format before linting** (pre-commit hook does this)
2. **Format early** (editor integration)
3. **Lint locally** (before committing)
4. **CI/CD enforcement** (catch issues before merge)

By following these practices, lint errors should be caught and fixed automatically, preventing accumulation.

