# Signal and Context Pattern

**Last Verified:** 2026-08-31


## When to use

Use **context cancellation** as the way to represent "interrupt requested" (e.g. user pressed Ctrl+C). Do not add one-off signal handlers that call `os.Exit` or custom cleanup unless there is no way for the long-running work to observe a context.

## Standard pattern: `signal.NotifyContext`

Go 1.16+ provides `signal.NotifyContext(parent, os.Interrupt, ...)` which returns a context that is cancelled when one of the given signals is received. This is the standard way to bridge OS signals to context cancellation.

```go
parent := context.Background()
if cmd != nil && cmd.Context() != nil {
    parent = cmd.Context()
}
runCtx, stopSignal := signal.NotifyContext(parent, os.Interrupt)
defer stopSignal()

// Pass runCtx to long-running work. When user hits Ctrl+C, runCtx is cancelled.
// Workers check runCtx.Err() or select on runCtx.Done() and return, so defers run (e.g. CPU profile flush).
```

- **Where**: Create `runCtx` at the boundary that starts the long-running operation (e.g. in `runCheckAsyncWithFollow`).
- **Pass** `runCtx` into the async path so cache build, warm phase, and discovery can check it and return early.
- **Wait** on both completion and `runCtx.Done()` in the same select so the caller returns "context canceled" on interrupt instead of only on timeout or success.

## Why not a separate signal handler?

A handler that does `signal.Notify(sigCh, os.Interrupt)` and then `stopCPUProfile(); os.Exit(130)` is disconnected from the rest of the flow:

- It invents a second path for "interrupt" instead of reusing context cancellation.
- `os.Exit` skips defers in other goroutines, so only the handler’s side effects run.
- Long-running work (e.g. warm phase) never sees the interrupt and can’t exit cleanly.

By using `signal.NotifyContext` and passing that context into the check (and into warm/cache), interrupt is just another cancellation: the check returns, defers run (including `stopCPUProfile()`), and the follower’s select returns `runCtx.Err()`.

## Avoid: `signal.Notify` + channel + `WithCancel` + goroutine

Hand-rolling **`signal.Notify`** on a buffered channel, **`context.WithCancel`**, and a **goroutine** that does `sig := <-sigCh` then **`cancel()`** mostly duplicates **`signal.NotifyContext`**: you still end up with a context that cancels on the same signals, but with extra parts to get wrong (channel depth, **`signal.Stop`**, goroutine lifetime, parent context choice).

- **Prefer** `runCtx, stopSignal := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM); defer stopSignal()` and pass **`runCtx`** to anything that should stop on interrupt (same pattern as **`cmd/zqk/system/check_follower.go`**).
- **Exception:** Only keep a custom channel/goroutine path if you need **signal-specific** branches (e.g. different behavior per signal) and **`NotifyContext`** is not enough—document why.

## Existing usage in this codebase

- **CLI timeout hook** (`pkg/cli/timeout_hook_*.go`): Signal is one branch in a `select` with command result and timeout; interrupt returns exit code 130 without `os.Exit`.
- **Scheduler** (`cmd/zqk/scheduler/scheduler_core.go`): Uses `cmd.Context()`; a goroutine waits on `ctx.Done()` and then runs cleanup and cancels the inner context so the scheduler stops.
- **System check async follow** (`cmd/zqk/system/check_follower.go`): Uses `signal.NotifyContext`; passes `runCtx` to the check goroutine and selects on `runCtx.Done()` so interrupt is handled via context.
- **zqk-svc serve skeleton** (`cmdv2/zqk-svc/serve.go`): Uses `signal.NotifyContext` for the long-lived wait until shutdown.

## References

- [Go: signal.NotifyContext](https://pkg.go.dev/os/signal#NotifyContext)
- [Respond to Ctrl+C with context (pace.dev)](https://pace.dev/blog/2020/02/17/repond-to-ctrl-c-interrupt-signals-gracefully-with-context-in-golang-by-mat-ryer.html)
