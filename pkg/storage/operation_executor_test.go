package storage

import (
	"testing"
)

func TestOperationExecutor_LifetimeCounters(t *testing.T) {
	executor := &OperationExecutor{}
	exec, fail := executor.GetOperationExecutorStats()
	if exec != 0 || fail != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", exec, fail)
	}

	executor.operationsExecutedTotal.Add(5)
	executor.operationsFailedTotal.Add(1)

	exec, fail = executor.GetOperationExecutorStats()
	if exec != 5 || fail != 1 {
		t.Errorf("expected (5, 1), got (%d, %d)", exec, fail)
	}
}
