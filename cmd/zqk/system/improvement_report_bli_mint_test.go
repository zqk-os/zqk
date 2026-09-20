package system

import (
	"context"
	"testing"
)

func TestEnsureWorkItemBLI_doesNotCreate(t *testing.T) {
	t.Parallel()
	// Nil storage would panic on List/Create. Skip must return first.
	ensureWorkItemBLI(context.Background(), nil, ImprovementWorkItem{
		Title:  "Address high-failure or timeout-prone commands",
		Metric: "metrics_issues",
	}, nil)
}
