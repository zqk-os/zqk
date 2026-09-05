package systemcheck

import "context"

// Checker is the systemcheck subpackage contract for hand-CAS + duplicate-ID scans.
type Checker interface {
	RunHandCASAndDupIDs(ctx context.Context, kindDir, kind string) (*HandCASSystemCheckResult, error)
}

// DefaultChecker delegates to RunSystemCheckForHandCASAndDupIDs.
type DefaultChecker struct{}

func (DefaultChecker) RunHandCASAndDupIDs(ctx context.Context, kindDir, kind string) (*HandCASSystemCheckResult, error) {
	return RunSystemCheckForHandCASAndDupIDs(ctx, kindDir, kind)
}

var _ Checker = DefaultChecker{}
