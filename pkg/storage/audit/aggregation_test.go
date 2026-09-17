package audit

import (
	"testing"
	"time"
)

func TestDefaultRules(t *testing.T) {
	t.Parallel()
	rules := DefaultRules()
	if len(rules) != 2 {
		t.Fatalf("len=%d, expected 2 (only declared noise kinds cache_*)", len(rules))
	}
	if rules[0].Window != time.Hour || rules[0].Threshold != 10 {
		t.Fatalf("first rule: %+v", rules[0])
	}
	for _, rule := range rules {
		for _, et := range rule.EventTypes {
			if et == EventTypeSchedulerJobStarted || et == EventTypeSchedulerJobCompleted {
				t.Fatalf("scheduler lifecycle events must not be in default aggregation rules: %s", et)
			}
		}
	}
}
