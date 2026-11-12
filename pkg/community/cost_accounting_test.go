package community

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCostAccounting_StaticFloorInvariant(t *testing.T) {
	ledger := NewCostLedger()

	acc, err := ledger.RegisterAccount("tenant-alpha", 1000, 80)
	if err != nil {
		t.Fatalf("expected successful account registration, got: %v", err)
	}

	if acc.AccountID != "tenant-alpha" {
		t.Errorf("expected account ID tenant-alpha, got: %s", acc.AccountID)
	}
	if acc.MaxCredits != 1000 {
		t.Errorf("expected max credits 1000, got: %d", acc.MaxCredits)
	}
	if acc.Status != BudgetStatusNominal {
		t.Errorf("expected status nominal, got: %s", acc.Status)
	}
}

func TestCostAccounting_OperationalDynamicProof(t *testing.T) {
	ledger := NewCostLedger()
	accountID := "tenant-beta"

	_, err := ledger.RegisterAccount(accountID, 100, 75)
	if err != nil {
		t.Fatalf("failed to register account: %v", err)
	}

	// Step 1: Nominal consumption
	status, err := ledger.Debit(accountID, 50)
	if err != nil {
		t.Fatalf("expected debit to succeed, got: %v", err)
	}
	if status != BudgetStatusNominal {
		t.Errorf("expected status nominal, got: %s", status)
	}

	// Step 2: Warning threshold trigger (80% used)
	status, err = ledger.Debit(accountID, 30)
	if err != nil {
		t.Fatalf("expected debit to succeed, got: %v", err)
	}
	if status != BudgetStatusWarning {
		t.Errorf("expected status warning, got: %s", status)
	}

	// Step 3: Replenish budget capacity
	err = ledger.Replenish(accountID, 40)
	if err != nil {
		t.Fatalf("expected replenish to succeed, got: %v", err)
	}
	snap, err := ledger.GetAccount(accountID)
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}
	if snap.CurrentCredits != 40 {
		t.Errorf("expected 40 current credits, got: %d", snap.CurrentCredits)
	}
	if snap.Status != BudgetStatusNominal {
		t.Errorf("expected status nominal after replenish, got: %s", snap.Status)
	}

	// Step 4: Serial batch under governor
	governor := NewCostGovernor(ledger)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := governor.AuthorizeExecution(ctx, accountID, 5); err != nil {
			t.Fatalf("failed to authorize execution on iteration %d: %v", i, err)
		}
	}

	finalSnap, err := ledger.GetAccount(accountID)
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}
	if finalSnap.CurrentCredits != 65 {
		t.Errorf("expected 65 credits used, got: %d", finalSnap.CurrentCredits)
	}
}

func TestCostAccounting_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	ledger := NewCostLedger()

	// Empty account registration
	if _, err := ledger.RegisterAccount("", 100, 80); !errors.Is(err, ErrInvalidAccountID) {
		t.Errorf("expected ErrInvalidAccountID, got: %v", err)
	}

	// Negative max credits
	if _, err := ledger.RegisterAccount("tenant-bad", -10, 80); err == nil {
		t.Errorf("expected error for negative max credits, got nil")
	}

	// Duplicate registration
	_, err := ledger.RegisterAccount("tenant-dup", 100, 80)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if _, err := ledger.RegisterAccount("tenant-dup", 100, 80); !errors.Is(err, ErrAccountAlreadyExists) {
		t.Errorf("expected ErrAccountAlreadyExists, got: %v", err)
	}

	// Non-existent account debit
	if _, err := ledger.Debit("unknown", 10); !errors.Is(err, ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}

	// Non-positive debit amount
	if _, err := ledger.Debit("tenant-dup", 0); !errors.Is(err, ErrInvalidDebitAmount) {
		t.Errorf("expected ErrInvalidDebitAmount, got: %v", err)
	}
	if _, err := ledger.Debit("tenant-dup", -5); !errors.Is(err, ErrInvalidDebitAmount) {
		t.Errorf("expected ErrInvalidDebitAmount, got: %v", err)
	}

	// Exceed budget quota
	_, err = ledger.Debit("tenant-dup", 150)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("expected ErrBudgetExceeded, got: %v", err)
	}
}

func TestCostAccounting_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_COST_ACCOUNTING_AND_BUDGET_ENFORCEMENT_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0054 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0054 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0054 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
