# Ambient Engine Architecture & Feature Gating

**Last Verified:** 2026-08-31


**Date:** 2026-06-02
**Context:** The `Ambient Engine` introduces proactive filesystem observation and predictive AST caching to the ZQK daemon. Because this requires constant background processing, it introduces a risk of CPU/Memory bloat.

## The Architectural Mandate
Per executive mandate: **"Do not lower standards for performance. Services must be cleanly separable via feature gates. If it cannot be cleanly separated, it must provide sustainable value at a cheap cost natively."**

## 1. Feature Gating & Capability Registration
The Ambient Engine will *not* run by default for all users. It is an opt-in `Capability`.

*   **Config Flag**: Controlled via `ZQK_ENABLE_AMBIENT_WATCHER=1` or a corresponding `zqk config` setting.
*   **The Interface**: The daemon must define an `ambient.Service` interface. If the feature gate is disabled, a `noopService` is injected into the scheduler.
*   **Zero-Cost Abstraction**: The core `Scheduler` should not know or care if the Ambient Engine is running. It merely exposes a generic event bus (or inbox) that the Ambient Engine *can* hook into if enabled.

## 2. Clean Separation (The Boundary)
The `pkg/ambient` package must be strictly isolated.
*   **Inputs**: It receives raw filesystem events (`fsnotify`).
*   **Outputs**: It produces predictive `.csnap` context envelopes or emits `ContextEvent` objects to the daemon's internal message bus.
*   **No Circular Dependencies**: `pkg/ambient` may depend on `pkg/storage` (to cache ASTs) and `pkg/hive/capability`, but the core scheduler must never import `pkg/ambient` directly except at the main binary injection point.

## 3. High-Performance Design (The Watcher)
Filesystem watchers can easily overwhelm a system (e.g., node_modules changes, heavy git checkouts). 
To maintain our <10MB RAM footprint:
1.  **Strict Ignoral**: The watcher must instantly drop events related to `.git`, `.zqk`, `.zqk-state`, and any directories listed in `.gitignore`.
2.  **Debouncing**: If a developer mashes `CMD+S` 10 times in 2 seconds, we must only trigger *one* evaluation cycle.
3.  **Cheap Evaluation**: Before waking up an expensive LLM or heavy AST parser, the engine must perform a cheap regex/path check (e.g., "Is this a Go file? Did the interface block change?").

## Implementation Strategy
1. Create `pkg/ambient/config.go` (Feature Gates).
2. Create `pkg/ambient/watcher.go` (The debounced `fsnotify` wrapper).
3. Inject the `ambient.Service` into `cmd/zqk/main.go` only if the feature is enabled.
