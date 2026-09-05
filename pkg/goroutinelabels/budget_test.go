package goroutinelabels

import (
	"sync"
	"testing"
)

func TestBudget_Reserve_Release(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})

	release, err := b.Reserve(5)
	if err != nil {
		t.Fatalf("Reserve(5): %v", err)
	}
	if b.Reserved() != 5 {
		t.Errorf("Reserved() = %d, want 5", b.Reserved())
	}
	if b.Available() != 5 {
		t.Errorf("Available() = %d, want 5", b.Available())
	}

	release()
	if b.Reserved() != 0 {
		t.Errorf("after release: Reserved() = %d, want 0", b.Reserved())
	}
	if b.Available() != 10 {
		t.Errorf("after release: Available() = %d, want 10", b.Available())
	}
}

func TestBudget_Reserve_Exceeded(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})

	_, err := b.Reserve(11)
	if err == nil {
		t.Fatal("Reserve(11) expected error")
	}
	if b.Reserved() != 0 {
		t.Errorf("Reserved() = %d, want 0 after failed reserve", b.Reserved())
	}
}

func TestBudget_Reserve_ExactThenExceed(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})

	release, err := b.Reserve(10)
	if err != nil {
		t.Fatalf("Reserve(10): %v", err)
	}
	if b.Available() != 0 {
		t.Errorf("Available() = %d, want 0", b.Available())
	}

	_, err = b.Reserve(1)
	if err == nil {
		t.Fatal("Reserve(1) with 0 available expected error")
	}

	release()
	if b.Available() != 10 {
		t.Errorf("after release: Available() = %d, want 10", b.Available())
	}
}

func TestBudget_Unbounded(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 0})

	release, err := b.Reserve(10000)
	if err != nil {
		t.Fatalf("unbounded Reserve(10000): %v", err)
	}
	if b.Reserved() != 10000 {
		t.Errorf("Reserved() = %d, want 10000", b.Reserved())
	}
	release()
	if b.Reserved() != 0 {
		t.Errorf("after release: Reserved() = %d, want 0", b.Reserved())
	}
}

func TestBudget_Release_Idempotent(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 10})

	release, _ := b.Reserve(3)
	release()
	release() // second release is no-op
	release()
	if b.Reserved() != 0 {
		t.Errorf("after multiple release: Reserved() = %d, want 0", b.Reserved())
	}
	if b.Available() != 10 {
		t.Errorf("Available() = %d, want 10", b.Available())
	}
}

func TestBudget_Reserve_ZeroOrNegative(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 5})

	release, err := b.Reserve(0)
	if err != nil {
		t.Fatalf("Reserve(0): %v", err)
	}
	release()
	if b.Reserved() != 0 {
		t.Errorf("Reserved() = %d, want 0", b.Reserved())
	}

	release, err = b.Reserve(-1)
	if err != nil {
		t.Fatalf("Reserve(-1): %v", err)
	}
	release()
	if b.Reserved() != 0 {
		t.Errorf("Reserved() = %d, want 0", b.Reserved())
	}
}

func TestBudget_MaxTotal(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 42})
	if b.MaxTotal() != 42 {
		t.Errorf("MaxTotal() = %d, want 42", b.MaxTotal())
	}
	b0 := NewBudget(BudgetConfig{MaxTotal: 0})
	if b0.MaxTotal() != 0 {
		t.Errorf("unbounded MaxTotal() = %d, want 0", b0.MaxTotal())
	}
}

func TestBudget_ConcurrentReserveRelease(t *testing.T) {
	t.Parallel()
	b := NewBudget(BudgetConfig{MaxTotal: 100})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		NewGoroutine("budget_test", "concurrent reserve release").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				release, err := b.Reserve(1)
				if err != nil {
					t.Errorf("Reserve(1): %v", err)
					return
				}
				release()
			}
		})
	}
	wg.Wait()
	if b.Reserved() != 0 {
		t.Errorf("after concurrent reserve/release: Reserved() = %d, want 0", b.Reserved())
	}
}

func TestSetDefaultBudget_DefaultBudget(t *testing.T) {
	// Avoid affecting other tests by not setting a default, or restore after
	prev := DefaultBudget()
	defer SetDefaultBudget(prev)

	SetDefaultBudget(nil)
	if DefaultBudget() != nil {
		t.Error("DefaultBudget() should be nil after SetDefaultBudget(nil)")
	}

	b := NewBudget(BudgetConfig{MaxTotal: 50})
	SetDefaultBudget(b)
	if DefaultBudget() != b {
		t.Error("DefaultBudget() should return set budget")
	}
}
