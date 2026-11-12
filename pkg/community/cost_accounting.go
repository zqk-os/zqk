package community

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	ErrAccountNotFound      = errors.New("cost accounting: account not found")
	ErrAccountAlreadyExists = errors.New("cost accounting: account already exists")
	ErrBudgetExceeded       = errors.New("cost accounting: budget quota exceeded")
	ErrInvalidDebitAmount   = errors.New("cost accounting: debit amount must be strictly positive")
	ErrInvalidAccountID     = errors.New("cost accounting: account id cannot be empty")
)

// BudgetStatus represents the consumption health stage.
type BudgetStatus string

const (
	BudgetStatusNominal  BudgetStatus = "nominal"
	BudgetStatusWarning  BudgetStatus = "warning"
	BudgetStatusExceeded BudgetStatus = "exceeded"
)

// AccountBudget defines quota parameters for a tenant or provider.
type AccountBudget struct {
	AccountID       string       `json:"account_id"`
	MaxCredits      int64        `json:"max_credits"`
	CurrentCredits  int64        `json:"current_credits"`
	WarningPercent  int          `json:"warning_percent"`
	Status          BudgetStatus `json:"status"`
	LastTransaction time.Time    `json:"last_transaction"`
}

// CostLedger manages multi-tenant budget quotas and thread-safe credit consumption.
type CostLedger struct {
	mu       sync.RWMutex
	accounts map[string]*AccountBudget
}

// NewCostLedger initializes an empty cost ledger.
func NewCostLedger() *CostLedger {
	return &CostLedger{
		accounts: make(map[string]*AccountBudget),
	}
}

// RegisterAccount initializes a new budget account with maximum allowable credits.
func (c *CostLedger) RegisterAccount(accountID string, maxCredits int64, warningPercent int) (*AccountBudget, error) {
	if strutil.IsBlank(accountID) {
		return nil, ErrInvalidAccountID
	}
	trimmed := strings.TrimSpace(accountID)
	if maxCredits <= 0 {
		return nil, errors.New("cost accounting: max credits must be greater than zero")
	}
	if warningPercent <= 0 || warningPercent >= 100 {
		warningPercent = 80 // sensible default
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.accounts[trimmed]; exists {
		return nil, fmt.Errorf("%w: %s", ErrAccountAlreadyExists, trimmed)
	}

	acc := &AccountBudget{
		AccountID:       trimmed,
		MaxCredits:      maxCredits,
		CurrentCredits:  0,
		WarningPercent:  warningPercent,
		Status:          BudgetStatusNominal,
		LastTransaction: time.Now(),
	}
	c.accounts[trimmed] = acc
	return acc, nil
}

// Debit deducts credits from an account and evaluates threshold statuses.
func (c *CostLedger) Debit(accountID string, credits int64) (BudgetStatus, error) {
	if strutil.IsBlank(accountID) {
		return "", ErrInvalidAccountID
	}
	trimmed := strings.TrimSpace(accountID)
	if credits <= 0 {
		return "", ErrInvalidDebitAmount
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	acc, exists := c.accounts[trimmed]
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrAccountNotFound, trimmed)
	}

	if acc.CurrentCredits+credits > acc.MaxCredits {
		acc.Status = BudgetStatusExceeded
		return BudgetStatusExceeded, fmt.Errorf("%w: requested %d, available %d", ErrBudgetExceeded, credits, acc.MaxCredits-acc.CurrentCredits)
	}

	acc.CurrentCredits += credits
	acc.LastTransaction = time.Now()

	usedPct := int((acc.CurrentCredits * 100) / acc.MaxCredits)
	if usedPct >= acc.WarningPercent {
		acc.Status = BudgetStatusWarning
	} else {
		acc.Status = BudgetStatusNominal
	}

	return acc.Status, nil
}

// Replenish resets or adds available budget capacity to an account.
func (c *CostLedger) Replenish(accountID string, credits int64) error {
	if strutil.IsBlank(accountID) {
		return ErrInvalidAccountID
	}
	trimmed := strings.TrimSpace(accountID)
	if credits <= 0 {
		return ErrInvalidDebitAmount
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	acc, exists := c.accounts[trimmed]
	if !exists {
		return fmt.Errorf("%w: %s", ErrAccountNotFound, trimmed)
	}

	if acc.CurrentCredits > credits {
		acc.CurrentCredits -= credits
	} else {
		acc.CurrentCredits = 0
	}
	acc.LastTransaction = time.Now()

	usedPct := int((acc.CurrentCredits * 100) / acc.MaxCredits)
	if usedPct >= acc.WarningPercent {
		acc.Status = BudgetStatusWarning
	} else {
		acc.Status = BudgetStatusNominal
	}

	return nil
}

// GetAccount retrieves a snapshot of the account budget.
func (c *CostLedger) GetAccount(accountID string) (*AccountBudget, error) {
	if strutil.IsBlank(accountID) {
		return nil, ErrInvalidAccountID
	}
	trimmed := strings.TrimSpace(accountID)

	c.mu.RLock()
	defer c.mu.RUnlock()

	acc, exists := c.accounts[trimmed]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrAccountNotFound, trimmed)
	}

	// return a safe copy
	snapshot := *acc
	return &snapshot, nil
}

// CostGovernor provides contextual gating for execution invocations based on ledger budgets.
type CostGovernor struct {
	ledger *CostLedger
}

// NewCostGovernor constructs a new cost governor.
func NewCostGovernor(ledger *CostLedger) *CostGovernor {
	return &CostGovernor{
		ledger: ledger,
	}
}

// AuthorizeExecution verifies and reserves credits prior to executing a unit of work.
func (g *CostGovernor) AuthorizeExecution(ctx context.Context, accountID string, estimatedCredits int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := g.ledger.Debit(accountID, estimatedCredits)
	return err
}
