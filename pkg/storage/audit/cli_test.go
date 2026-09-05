package audit

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestHasCLIMarker(t *testing.T) {
	if HasCLIMarker(context.Background()) {
		t.Fatal("background should not be CLI")
	}
	ctx := WithCLIOperation(context.Background())
	if !HasCLIMarker(ctx) {
		t.Fatal("WithCLIOperation should set CLI marker")
	}
	if !IsCLIOperation(ctx, nil) {
		t.Fatal("CLI marker should make IsCLIOperation true")
	}
}

func TestIsCLIOperationBypassPolicy(t *testing.T) {
	sec := &pkgctx.SecurityContext{Permissions: []string{"bypass_policy"}}
	if !IsCLIOperation(context.Background(), sec) {
		t.Fatal("bypass_policy should be treated as CLI")
	}
}

func TestHasDeferEvents(t *testing.T) {
	if HasDeferEvents(nil) || HasDeferEvents(context.Background()) {
		t.Fatal("empty ctx should not defer")
	}
	ctx := WithDeferEvents(context.Background())
	if !HasDeferEvents(ctx) {
		t.Fatal("WithDeferEvents should set marker")
	}
}

func TestBeginEventCreation(t *testing.T) {
	if IsCreatingEvent() {
		t.Fatal("depth should start at 0")
	}
	done := BeginEventCreation()
	if !IsCreatingEvent() {
		t.Fatal("depth should be > 0")
	}
	done()
	if IsCreatingEvent() {
		t.Fatal("depth should return to 0")
	}
}
