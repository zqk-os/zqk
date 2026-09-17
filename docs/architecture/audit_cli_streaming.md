# Audit CLI & WAL Streaming Architecture

## Overview
This document specifies the exact CLI implementation patterns for ZQK's audit streams. It includes the safe JSONL WAL Tailer, the ANSI-colored live terminal consumer (`zqk audit stream`), and the paginated historical query handler (`zqk audit query`).

*Note: The CLI stream uses standard ANSI escape codes mapped to ZQK's semantic safety classes.*

## 1. Safe JSONL WAL Tailer (`pkg/audit/wal_tailer.go`)
Yields `*AuditRecord` structs line-by-line. Malformed lines are caught per-line, logged to stderr using `logging.FluentEvent`, and skipped without crashing the stream buffer.

```go
package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"github.com/lanceman/zqk/pkg/logging"
)

// WALTailer safely tail-streams ZQK's audit WAL, yielding valid records on a channel.
func (a *AuditStream) Tail(ctx context.Context, walPath string) (<-chan *AuditRecord, <-chan error) {
	recCh := make(chan *AuditRecord, 128)
	errCh := make(chan error, 1)

	go func() {
		defer close(recCh)
		defer close(errCh)

		f, err := os.Open(walPath)
		if err != nil {
			errCh <- fmt.Errorf("failed to open WAL: %w", err)
			return
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		const maxLine = 1 << 20 // 1MB safety cap per line
		scanner.Buffer(make([]byte, 0, 1024), maxLine)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				if !scanner.Scan() {
					if err := scanner.Err(); err != nil {
						errCh <- fmt.Errorf("WAL scan error: %w", err)
						return
					}
					// Static polling backoff (replace with fsnotify for true tailing)
					select {
					case <-ctx.Done(): return
					case <-time.After(250 * time.Millisecond): continue
					}
				}

				line := scanner.Text()
				var rec AuditRecord
				if err := json.Unmarshal([]byte(line), &rec); err != nil {
					// SAFE GRACEFUL DEGRADATION
					logging.FluentEvent(logging.GetLogger()).Warn("Skipping malformed JSONL line", err).Log()
					continue
				}

				select {
				case recCh <- &rec:
				case <-ctx.Done(): return
				}
			}
		}
	}()

	return recCh, errCh
}
```

## 2. CLI Live Stream Consumer (`cmd/zqk/system/audit_stream.go`)
Applies exact ANSI color mapping and streams directly to stdout with zero buffering overhead.

```go
package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/audit"
)

var safetyColor = map[string]string{
	"read":         "\033[36m", // Cyan
	"write":        "\033[36m", // Cyan
	"destructive":  "\033[31m", // Red
	"hil_required": "\033[33m", // Yellow
}

func RunAuditStream(ctx context.Context) error {
	stream := audit.NewAuditStream()
	
	ctx, cancel := context.WithCancel(ctx)
	go audit.WatchWAL(ctx, ".zqk/audit/mutations.jsonl", stream)

	recCh, errCh := stream.Subscribe(ctx)
	defer cancel()

	for {
		select {
		case rec := <-recCh:
			renderLiveAudit(rec)
		case err := <-errCh:
			fmt.Fprintf(os.Stderr, "[zqk/audit] stream error: %v\n", err)
		case <-ctx.Done():
			fmt.Fprintln(os.Stdout)
			return nil
		}
	}
}

func renderLiveAudit(rec *audit.AuditRecord) {
	const reset = "\033[0m"
	const metaColor = "\033[90m" // Dim/Gray

	color, ok := safetyColor[strings.ToLower(rec.SafetyClass)]
	if !ok { color = "\033[37m" }

	ts := rec.Timestamp[:19]
	fmt.Printf("%s%s%s %s%s[%-8s]%s %-12s | %s/%-8s -> %s\n",
		metaColor, ts, reset,
		color, rec.SafetyClass, reset,
		trunc(rec.TaskID, 14), trunc(rec.IdempotencyKey, 8), rec.Status)
}

func trunc(s string, n int) string {
	if len(s) > n { return s[:n] + ".." }
	return s
}
```

## 3. Pagination Handler (`cmd/zqk/system/audit_query.go`)
Pipes historical payloads to `less -R` preserving ANSI codes automatically.

```go
package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"github.com/lanceman/zqk/pkg/audit"
)

func RunAuditQuery(ctx context.Context, since string) error {
	var buf bytes.Buffer
	writer := &io.Writer(&buf)
	
	results := fetchHistoricalAudits(since)
	for _, rec := range results {
		line, _ := json.Marshal(rec)
		fmt.Fprintf(writer, "%s\n", line)
	}

	if shouldPaginate(&buf) && stdoutIsTerminal() {
		cmd := exec.CommandContext(ctx, "less", "-R")
		cmd.Stdin = bytes.NewReader(buf.Bytes())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	fmt.Print(buf.String())
	return nil
}

func shouldPaginate(buf *bytes.Buffer) bool {
	return buf.Len() > 2048
}

func stdoutIsTerminal() bool {
	stat, err := os.Stdout.Stat()
	return err == nil && (stat.Mode() & os.ModeCharDevice) != 0
}

func fetchHistoricalAudits(since string) []*audit.AuditRecord {
	return []*audit.AuditRecord{}
}
```
