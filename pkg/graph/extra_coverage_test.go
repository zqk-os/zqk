// BLI-STARTER-COMMUNITY-045 / PRI-STARTER-COMMUNITY-045 coverage elevation
package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/mutation"
)

type extraRecord struct{ m map[string]any }

func (r extraRecord) AsMap() map[string]any { return r.m }

type extraResult struct {
	rec Record
	err error
}

func (r extraResult) Single() (Record, error) { return r.rec, r.err }

type extraTx struct {
	runErr error
	result Result
}

func (t extraTx) Run(context.Context, string, map[string]any) (Result, error) {
	if t.runErr != nil {
		return nil, t.runErr
	}
	return t.result, nil
}

type extraSession struct {
	readErr  error
	writeErr error
	tx       extraTx
}

func (s extraSession) ExecuteRead(ctx context.Context, work func(tx TransactionContext) (any, error)) error {
	if s.readErr != nil {
		return s.readErr
	}
	_, err := work(s.tx)
	return err
}

func (s extraSession) ExecuteWrite(ctx context.Context, work func(tx TransactionContext) (any, error)) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	_, err := work(s.tx)
	return err
}

func TestExtraGraphImplCommitAndMutationLookup(t *testing.T) {
	ctx := context.Background()
	g := &GraphImpl{Session: extraSession{
		tx: extraTx{result: extraResult{rec: extraRecord{m: map[string]any{"exists": true}}}},
	}}
	ok, err := g.HasCommittedMutation(ctx, "ik-1")
	if err != nil || !ok {
		t.Fatalf("exists = %v %v", ok, err)
	}

	g = &GraphImpl{Session: extraSession{readErr: errors.New("read")}}
	if _, err := g.HasCommittedMutation(ctx, "ik-1"); err == nil {
		t.Fatal("read err")
	}
	g = &GraphImpl{Session: extraSession{tx: extraTx{runErr: errors.New("run")}}}
	if _, err := g.HasCommittedMutation(ctx, "ik-1"); err == nil {
		t.Fatal("run err")
	}
	g = &GraphImpl{Session: extraSession{tx: extraTx{result: extraResult{err: errors.New("single")}}}}
	if _, err := g.HasCommittedMutation(ctx, "ik-1"); err == nil {
		t.Fatal("single err")
	}

	g = &GraphImpl{Session: extraSession{}}
	if err := g.CommitWithResilience(ctx, "ik-1", []mutation.Mutation{}, "task-1"); err != nil {
		t.Fatal(err)
	}
	_ = isTransientDBError(errors.New("x"))
}
