package storage

import (
	"context"
	"errors"
	"reflect"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// readProjectionStub embeds [NoopObjectStorage] but overrides Read for ReadProjected tests.
type readProjectionStub struct {
	NoopObjectStorage
	readObj map[string]any
	readErr error
}

func (s readProjectionStub) Read(context.Context, *pkgctx.SecurityContext, string) (map[string]any, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.readObj, nil
}

func TestReadProjected_nilFields_returnsSameReference(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		objects.FieldKeyID: "O-1",
		"k":                "full",
	}
	stub := readProjectionStub{readObj: obj}

	got, err := ReadProjected(ctx, secCtx, stub, "O-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.ValueOf(got).Pointer() != reflect.ValueOf(obj).Pointer() {
		t.Fatal("nil fields must return Read result without copying")
	}

	got2, err := ReadProjected(ctx, secCtx, stub, "O-1", []string{})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.ValueOf(got2).Pointer() != reflect.ValueOf(obj).Pointer() {
		t.Fatal("empty fields slice must return Read result without copying")
	}
}

func TestReadProjected_fields_projectsTopLevel(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		objects.FieldKeyID:      "O-1",
		objects.FieldKeyKind:    "task",
		objects.FieldKeyPayload: 42,
	}
	stub := readProjectionStub{readObj: obj}

	got, err := ReadProjected(ctx, secCtx, stub, "O-1", []string{objects.FieldKeyID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[objects.FieldKeyID] != "O-1" {
		t.Fatalf("got %#v", got)
	}
	if _, ok := got[objects.FieldKeyKind]; ok {
		t.Fatal("expected projection to drop kind")
	}
}

func TestReadProjected_readError(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	want := errors.New("boom")
	stub := readProjectionStub{readErr: want}

	_, err := ReadProjected(ctx, secCtx, stub, "O-1", []string{objects.FieldKeyID})
	if !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}
