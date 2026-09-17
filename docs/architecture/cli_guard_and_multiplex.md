# CLI Terminal Guard & Multi-Workspace Multiplexing

## Overview
This architecture ensures that the ZQK CLI never leaves a user's terminal in a corrupted ANSI state on exit (crash, panic, or interrupt) and securely multiplexes audit streams across multiple active workspaces (e.g., `prod`, `dev`) using isolated `neo4j.Session` contexts.

## 1. Terminal Escape-Sequence Guard (`pkg/cli/guard.go`)
Installs SIGINT/SIGTERM handlers and provides a deferrable `Reset()` method to flush TTY colors back to default (`\033[0m`) upon any exit path.

```go
package cli

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const ANSI_RESET = "\033[0m"

type TerminalGuard struct {
	mu   sync.Mutex
	once bool
}

func NewTerminalGuard() *TerminalGuard { return &TerminalGuard{} }

func (g *TerminalGuard) Install() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		g.Reset()
		os.Exit(0)
	}()
}

// Must be deferred in every Cobra command root: defer guard.Reset()
func (g *TerminalGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.once { return }
	g.once = true

	fmt.Fprintf(os.Stdout, "%s\n", ANSI_RESET)
	fmt.Fprintf(os.Stderr, "%s\n", ANSI_RESET)
	os.Stdout.Sync()
	os.Stderr.Sync()
}
```

## 2. Multi-Workspace Routing (`cmd/zqk/system/audit_multiplex.go`)
Concurrently tails graph kernels across multiple workspaces, mapping specific ANSI colors to workspace ID prefixes.

```go
package system

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/lanceman/zqk/pkg/audit"
	"github.com/lanceman/zqk/pkg/cli"
)

var workspaceColor = map[string]string{
	"default": "\033[90m", // Dim gray
	"prod":    "\033[35m", // Purple
	"staging": "\033[34m", // Blue
	"dev":     "\033[33m", // Yellow
}

func RunMultiWorkspaceAuditStream(ctx context.Context, workspaceList []string) error {
	streamCh := make(chan audit.AuditRecord, 128)
	var wg sync.WaitGroup

	for _, ws := range workspaceList {
		sessCtx := context.WithValue(ctx, cli.WorkspaceKey(ws), true)
		session := cli.GraphFromContext(sessCtx)
		if session == nil { continue }

		wg.Add(1)
		go func(w string, s neo4j.Session) {
			defer wg.Done()
			
			err := audit.FetchAndStreamAudits(ctx, s, time.Now().Add(-time.Hour), 10000, &wsWriter{target: os.Stdout, ws: w})
			if err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "[\033[31mERR\033[0m] %s: %v\n", w, err)
			}
		}(ws, session)
	}

	// Fan-in multiplexer to stdout
	multiplexRender(ctx, streamCh)
	wg.Wait()
	close(streamCh)
	return ctx.Err()
}

type wsWriter struct { target io.Writer; ws string }
func (w *wsWriter) Write(p []byte) (int, error) {
	id := workspaceColor[w.ws]
	if id == "" { id = "\033[90m" }
	_, err := fmt.Fprintf(w.target, "%s[%-12s]\033[0m %s", id, w.ws, string(p))
	if err == nil { os.Stdout.Sync() } 
	return len(p), err
}

func multiplexRender(ctx context.Context, ch <-chan audit.AuditRecord) {
	for {
		select {
		case rec := <-ch:
			renderLiveAudit(&rec) 
		case <-ctx.Done(): return
		}
	}
}
```
