# Shutdown Hang Analysis (Process Sample)

## Summary

Shutdown hangs because the **MCP serve loop** blocks in a `select` waiting for the next stdin read to complete, while the **reader goroutine** is blocked in a syscall **read(stdin)** that is never interrupted or closed during shutdown.

## Evidence from Process Sample

- **Main thread** (`Thread_5384082`, `com.apple.main-thread`): **2494 samples** in `pthread_cond_wait` → Go runtime channel receive. This is the MCP `ServeLoop()` blocked in `select { case readResult := <-readChan: ... }`, waiting for the next message.
- **One thread** (`Thread_5384090`): **2494 samples** in `read` (syscall). This is the MCP message-reader goroutine (`mcp_message_reader`) blocked in `transport.ReadMessage(reader)` where `reader` wraps `os.Stdin`. `ReadMessage` uses `Peek(1)` / content-length reads, which block in `read()` when there is no input.
- **Other threads**: Many in `pthread_cond_wait` (idle goroutines) and a few in `kevent` (netpoller).

## Root Cause

1. **Serve loop** (`pkg/mcp/serve_coordinator.go`): Each iteration starts an async read via `ReadMessageAsync(reader)` and then blocks in:
   ```go
   select {
   case readResult := <-readChan:
   case <-timeoutChan:  // may be nil if no server context
   }
   ```
   Shutdown is only checked at the **start** of the loop and **after** a message is received. There is **no** `select` case for shutdown.

2. **Reader goroutine** (`pkg/mcp/message_processor.go`): `ReadMessageAsync` runs `transport.ReadMessage(reader)` in a goroutine. That call blocks in `reader.Peek(1)` / `readContentLengthMessage` → underlying `read(stdin)`.

3. **On shutdown**: `shutdownFlag` is set and `shutdownCtx` is cancelled, but the serve loop is stuck in the `select` waiting for `readChan`. The only way `readChan` gets a value is when the reader returns from `ReadMessage`. The reader is blocked in a syscall **read** on stdin; cancelling the context does **not** close stdin or interrupt the read, so the read never returns and shutdown never proceeds.

## Fix

Add a third case to the serve loop `select` so that when the server shutdown context is cancelled, the loop exits immediately instead of waiting for the in-flight read:

- In `pkg/mcp/serve_coordinator.go`, in `ServeLoop()`, include `sc.server.GetShutdownContext().Done()` in the `select`. When it fires, return from the loop (e.g. `return sc.handleExplicitShutdown()`).

The in-flight reader goroutine may remain blocked in `read(stdin)` until the process exits (or the parent closes stdin). Exiting the serve loop allows the rest of shutdown to run and the process to terminate; the OS will clean up the blocked reader.

## Alternative / Complementary

- **Close stdin on shutdown**: If the process can safely close or replace stdin when shutdown is requested (e.g. via a wrapper that implements `io.Reader` and closes when `shutdownCtx` is done), the reader would get EOF and exit. This is more invasive and may not be appropriate if other code uses `os.Stdin`.
- **Interruptible read**: Use a read that can be cancelled (e.g. wrapper that does `select` between reading from stdin and `shutdownCtx.Done()`); requires changing how the transport reads.

The minimal, low-risk fix is to add the shutdown context to the serve loop `select` as above.
