# CLI context patterns (tests and handlers)

Human reference for how we wire **`github.com/lanceman/zqk/internal/cli`** contexts so we do not regress into brittle literals, wrong imports, or import cycles.

This is **documentation only**; bulk mechanical rewrites may use **`scripts/refactor-wrap-cli-context-tests.py`** and **`scripts/README.md`** (CLI context quick reference table).

## Preferred helpers (`internal/cli`)

| Situation | Use |
|-----------|-----|
| Minimal fixture: project root only, table format | **`cli.ContextForProjectRoot(projectRoot)`** |
| Project root + profile (common in tests) | **`cli.ContextForProjectAndProfile(projectRoot, profile)`** — same as **`ContextForProjectRoot(...).WithProfile(...)`** |
| Non-default inner format or profile on a minimal context | Chain **`.WithProfile("...").WithFormat("table")`** (or equivalent) on **`ContextForProjectRoot`** |
| **`ContextManager.LoadContext`** produced a full inner `*context.Context` and you only need table output on the wrapper | **`cli.ContextFromInner(minimalCtx)`** — avoids repeating **`&cli.Context{ Context: minimalCtx, Format: cli.FormatTable }`** |

## Imports: two layers

- **`internal/cli`**: `*cli.Context`, **`GetContextFromCommand`**, **`SetContext`**, **`ResolveProjectRoot`**, **`ContextForProjectRoot`**, **`ContextForProjectAndProfile`**, **`ContextFromInner`**.
- **`internal/cli/context`**: lower-level **`Context`**, **`ResolveProjectRootFromSettings`**, **`LoadBrandSettings`**, and other workspace helpers **without** importing **`internal/cli`**.

**`pkg/storage` must not import `internal/cli`** (cycles). Use **`internal/cli/context`** only where root resolution is needed without the CLI wrapper.

## Production wrapper methods (`withInnerContext`)

New methods on **`*cli.Context`** that delegate to the embedded **`*context.Context`** must use **`withInnerContext`** (see **`internal/cli/context.go`**) so the nil guard is not duplicated. **`scripts/check-cli-inner-guard.sh`** enforces a single **`(c == nil || c.Context == nil)`** site. Full pattern: **[WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md](./WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md)** — glossary **`GLS-EXAMPLE`**.

## Anti-patterns to avoid

- Hand-building **`&cli.Context{ Context: &clicontext.Context{ProjectRoot: ..., Profile: ...}, Format: cli.FormatTable }`** for cases covered by **`ContextForProjectAndProfile`** or **`ContextForProjectRoot`**. Duplicates defaults and drifts when defaults change.
- Per-test **`wrapCLIContextForTest`**-style helpers that only wrap **`clicontext.Context`** literals — prefer the **`cli.ContextFor*`** helpers so call sites stay uniform.
- Typing **`cliCtx`** as the **inner** pointer when the API expects **`*cli.Context`** (loses **`Format`** and wrapper behavior). Prefer **`cli.GetContext(cmd)`** and variables typed **`*cli.Context`**.

## When explicit literals are still OK

- Inner **`clicontext.Context`** with **fields not expressible** by **`ContextForProjectRoot` / WithProfile / Derive** (unusual tests). Keep **`internal/cli/context`** import and document why in a short comment if non-obvious.
- Production paths that assemble context through **`GetContextFromCommand`** or **`LoadContext`** — use **`ContextFromInner`** for the minimal-wrapper fallback where applicable.

## Verification after mechanical edits

- **`go test <package> -run '^$'`** compiles tests without running them.
- Targeted **`go test -timeout ... -run ...`** for affected tests when behavior is non-trivial.
