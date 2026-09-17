# Audit Historical Query Architecture

## Overview
This document outlines the zero-buffering stream fetcher and terminal pager routing logic for querying historical audits via Cypher. It ensures we can stream millions of audit rows from Memgraph directly to a terminal pager (`less -R`) without blowing up the daemon's RAM.

*Note: Adapted for ZQK using `errfmt`.*

## 1. Streaming Fetcher (`pkg/audit/query_stream.go`)

```go
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/lanceman/zqk/pkg/errfmt"
)

type AuditRecord struct {
	AuditID        string `json:"audit_id"`
	IdempotencyKey string `json:"idempotency_key"`
	TaskID         string `json:"task_id"`
	ModelID        string `json:"model_id"`
	SafetyClass    string `json:"safety_class"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	Timestamp      string `json:"timestamp"`
}

func FetchAndStreamAudits(ctx context.Context, db neo4j.SessionRunner, since time.Time, limit int, w io.Writer) error {
	query := `MATCH (a:MutationAudit)
WHERE a.timestamp >= datetime($since)
RETURN a.audit_id AS audit_id,
       a.idempotency_key AS idempotency_key,
       a.task_id AS task_id,
       a.model_id AS model_id,
       a.safety_class AS safety_class,
       a.action AS action,
       a.status AS status,
       toString(a.timestamp) AS timestamp
ORDER BY a.timestamp ASC
LIMIT $limit`

	return db.ExecuteRead(ctx, func(tx neo4j.TransactionContext) (interface{}, error) {
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"since": since,
			"limit": limit,
		})
		if err != nil {
			return nil, errfmt.Errorf("cypher exec failed").Wrap(err)
		}
		defer res.Close()

		for res.Next(ctx) {
			rec := res.Record()
			out := AuditRecord{
				AuditID:        castStr(rec, "audit_id"),
				IdempotencyKey: castStr(rec, "idempotency_key"),
				TaskID:         castStr(rec, "task_id"),
				ModelID:        castStr(rec, "model_id"),
				SafetyClass:    castStr(rec, "safety_class"),
				Action:         castStr(rec, "action"),
				Status:         castStr(rec, "status"),
				Timestamp:      castStr(rec, "timestamp"),
			}

			line, err := json.Marshal(out)
			if err != nil { continue } 
			fmt.Fprintf(w, "%s\n", line)

			select {
			case <-ctx.Done(): return nil, ctx.Err()
			default:
			}
		}
		if err := res.Err(); err != nil {
			return nil, errfmt.Errorf("stream error").Wrap(err)
		}
		return nil, nil
	})
}

func castStr(rec neo4j.Record, key string) string {
	val, ok := rec.Get(key)
	if !ok || val == nil { return "" }
	if s, ok := val.(string); ok { return s }
	return fmt.Sprintf("%v", val)
}
```

## 2. CLI Pager Routing (`cmd/zqk/system/audit_query.go`)

```go
package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/lanceman/zqk/pkg/audit"
	"github.com/lanceman/zqk/pkg/errfmt"
)

func RunAuditQuery(ctx context.Context, sinceStr string, batchSize int, session neo4j.Session) error {
	since, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil { return errfmt.Errorf("invalid --since format").Wrap(err) }

	writer := os.Stdout
	var pagerCleanup func() error = func() error { return nil }

	if shouldPaginate(batchSize) && stdoutIsTerminal() {
		r, w, err := os.Pipe()
		if err != nil { return errfmt.Errorf("pipe creation failed").Wrap(err) }
		
		pagerCmd := exec.CommandContext(ctx, "less", "-R")
		pagerCmd.Stdin = r
		pagerCmd.Stdout = os.Stdout
		pagerCmd.Stderr = os.Stderr
		
		writer = w
		pagerCleanup = func() error { 
			w.Close()
			return pagerCmd.Wait() 
		}
		go pagerCmd.Run() 
	}

	err = audit.FetchAndStreamAudits(ctx, session, since, batchSize, writer)
	
	if cerr := pagerCleanup(); cerr != nil {
		return errfmt.Errorf("pager exited").Wrap(cerr)
	}
	return err
}

func shouldPaginate(limit int) bool { return limit > 50 }
func stdoutIsTerminal() bool {
	stat, err := os.Stdout.Stat()
	return err == nil && (stat.Mode()&os.ModeCharDevice) != 0
}
```
