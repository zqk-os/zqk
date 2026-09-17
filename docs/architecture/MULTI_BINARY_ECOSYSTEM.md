# Multi-binary ecosystem

**Last Verified:** 2026-08-31


This document lists **Go binaries** in the repo, how they relate, and what changes for day-to-day use.

## Binaries you actually run

| Binary / output name | Source (`go build …`) | Role |
|---------------------|------------------------|------|
| **`zqk`** | `./cmd/zqk` | Primary CLI: objects, system, scheduler, internal, etc. |
| **`zqk-admin`** | `./cmd/zqk-admin` | **Same command tree and bootstrap as `zqk`** (`cmd/zqk/app`). Different executable name only (e.g. runbooks, PATH aliases). |
| **`zqk-mcp`** | `./cmd/zqk/mcp-simple` | Minimal MCP server binary (not the full interactive CLI). |
| **`pattern-cli`** | `./cmd/pattern-cli` | Standalone demo of the async command pattern; not the product CLI. |

`make build-zqk-staging` builds **`zqk`** and **`zqk-admin`** with the same `-ldflags` into `.zqk/tmp/zqk-build` and `.zqk/tmp/zqk-build-admin`. `make copy-zqk-from-staging` copies those to `./zqk`, `bin/zqk`, `bin/zqk-stable`, and **`bin/zqk-admin`** (admin build is **not** a copy of the `zqk` artifact; it is built from `./cmd/zqk-admin`).

## Experimental / opt-in (cmdv2)

Built only when you run the matching **Makefile** target (not part of the default `zqk` workflow):

| Target | Source | Notes |
|--------|--------|--------|
| `bin/zqk-v2` | `./cmdv2/zqk` | Experimental core CLI |
| `bin/zqkdev` | `./cmdv2/zqkdev` | Experimental dev/tooling |
| `bin/zqk-scenario` | `./cmdv2/zqk-scenario` | Experimental scenario/bundle |
| `bin/zqk-svc` | `./cmdv2/zqk-svc` | Experimental services |

Treat these as **separate products** until promoted into the main `cmd/zqk` line.

## How many binaries?

- **Production path:** effectively **three** distinct programs people care about: **`zqk`**, **`zqk-admin`** (duplicate runtime), **`zqk-mcp`**.
- **Plus** optional **cmdv2** variants if you opt in.
- **Plus** **`pattern-cli`** for the async-pattern demo only.

So: **one** full CLI implementation (`cmd/zqk/app`); **two** entrypoints that ship it (`zqk`, `zqk-admin`); **one** slim MCP binary; everything else is optional or demo.

## Typical workflow — what changes?

**Almost nothing** if you keep using `zqk`:

- Subcommands, flags, and behavior are **identical** between `zqk` and `zqk-admin`.
- The only intentional difference is **branding from `os.Args[0]`**: `--help` / `--version` show the executable basename (e.g. `Executable: zqk` vs `Executable: zqk-admin`).

Use **`zqk-admin`** when:

- A script or doc standardizes on an “admin” binary name on `PATH`.
- You want a visual distinction in process lists (`ps`) without changing any syntax.

You do **not** need `zqk-admin` for ordinary `object …` CRUD; both binaries share that surface.

Elevated access (`object … --internal`) requires an Enterprise license **or** the `zqk-admin` binary (transitional carrier). The legacy `internal …` tree is **admin-binary-only**, deprecated in favor of `object … --internal` (DEC-1785930071988960000-364a5796).

## Command execution model (unchanged)

- Both `zqk` and `zqk-admin` call **`app.Execute()`** in `cmd/zqk/app` (root command, timeout hook, dispatch, CAS flush, etc.).
- **Syntax does not change** between the two: `zqk object list` and `zqk-admin object list` are the same.

## Related docs

- Operator-facing command surfaces: `docs/enforcement/AGENT_GUIDELINES.md` (CLI command surfaces table).
- Makefile targets: `Makefile` `help` and `build-zqk-staging` / `copy-zqk-from-staging`.
