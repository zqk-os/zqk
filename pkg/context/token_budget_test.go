package context_test

import (
	"testing"

	zqkctx "github.com/zqk-os/zqk/pkg/context"
)

func TestNewTokenBudget_RejectsInvalidLimits(t *testing.T) {
	t.Parallel()
	if _, err := zqkctx.NewTokenBudget(0, 100); err == nil {
		t.Fatal("expected error for soft=0")
	}
	if _, err := zqkctx.NewTokenBudget(200, 100); err == nil {
		t.Fatal("expected error for soft>hard")
	}
}

func TestTokenBudget_SoftHardEvictionSignals(t *testing.T) {
	t.Parallel()
	b, err := zqkctx.NewTokenBudget(100, 150)
	if err != nil {
		t.Fatal(err)
	}
	if b.ShouldEvict() || b.AtHardLimit() {
		t.Fatal("empty budget should not signal eviction")
	}
	if got := b.Add(99); got != 99 {
		t.Fatalf("Add=99 got %d", got)
	}
	if b.ShouldEvict() {
		t.Fatal("below soft should not evict")
	}
	if got := b.Add(1); got != 100 {
		t.Fatalf("Add to soft got %d", got)
	}
	if !b.ShouldEvict() || !b.AtSoftLimit() {
		t.Fatal("at soft should ShouldEvict")
	}
	b.SetCurrent(150)
	if !b.AtHardLimit() {
		t.Fatal("expected hard limit")
	}
	b.Add(-200)
	if b.Current() != 0 {
		t.Fatalf("clamp to 0: got %d", b.Current())
	}
}

func TestDefaultTokenBudget_Limits(t *testing.T) {
	t.Parallel()
	b := zqkctx.DefaultTokenBudget()
	if b.SoftLimit() != zqkctx.DefaultTokenSoftLimit || b.HardLimit() != zqkctx.DefaultTokenHardLimit {
		t.Fatalf("defaults soft=%d hard=%d", b.SoftLimit(), b.HardLimit())
	}
}
