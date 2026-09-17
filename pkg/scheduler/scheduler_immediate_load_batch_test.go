package scheduler

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestGetSchedulerImmediateLoadBatchSize(t *testing.T) {
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize().Name(), "")
	if n := getSchedulerImmediateLoadBatchSize(); n != 100 {
		t.Fatalf("default: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize().Name(), "  \t  ")
	if n := getSchedulerImmediateLoadBatchSize(); n != 100 {
		t.Fatalf("whitespace: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize().Name(), "notint")
	if n := getSchedulerImmediateLoadBatchSize(); n != 0 {
		t.Fatalf("invalid: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize().Name(), "-1")
	if n := getSchedulerImmediateLoadBatchSize(); n != 0 {
		t.Fatalf("negative: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize().Name(), "10")
	if n := getSchedulerImmediateLoadBatchSize(); n != 10 {
		t.Fatalf("valid: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize().Name(), "999999999")
	if n := getSchedulerImmediateLoadBatchSize(); n != maxSchedulerImmediateLoadBatchSize {
		t.Fatalf("clamp: got %d want %d", n, maxSchedulerImmediateLoadBatchSize)
	}
}

func TestGetSchedulerImmediateLoadBatchPause(t *testing.T) {
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause().Name(), "")
	if d := getSchedulerImmediateLoadBatchPause(); d != 0 {
		t.Fatalf("default: got %v", d)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause().Name(), "nope")
	if d := getSchedulerImmediateLoadBatchPause(); d != 0 {
		t.Fatalf("invalid: got %v", d)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause().Name(), "100ms")
	if d := getSchedulerImmediateLoadBatchPause(); d != 100*time.Millisecond {
		t.Fatalf("parse: got %v", d)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause().Name(), "200h")
	if d := getSchedulerImmediateLoadBatchPause(); d != maxSchedulerImmediateLoadBatchPause {
		t.Fatalf("clamp: got %v want %v", d, maxSchedulerImmediateLoadBatchPause)
	}
}
