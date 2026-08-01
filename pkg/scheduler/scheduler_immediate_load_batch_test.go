package scheduler

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestGetSchedulerImmediateLoadBatchSize(t *testing.T) {
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "")
	if n := getSchedulerImmediateLoadBatchSize(); n != 100 {
		t.Fatalf("default: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "  \t  ")
	if n := getSchedulerImmediateLoadBatchSize(); n != 100 {
		t.Fatalf("whitespace: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "notint")
	if n := getSchedulerImmediateLoadBatchSize(); n != 0 {
		t.Fatalf("invalid: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "-1")
	if n := getSchedulerImmediateLoadBatchSize(); n != 0 {
		t.Fatalf("negative: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "10")
	if n := getSchedulerImmediateLoadBatchSize(); n != 10 {
		t.Fatalf("valid: got %d", n)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "999999999")
	if n := getSchedulerImmediateLoadBatchSize(); n != maxSchedulerImmediateLoadBatchSize {
		t.Fatalf("clamp: got %d want %d", n, maxSchedulerImmediateLoadBatchSize)
	}
}

func TestGetSchedulerImmediateLoadBatchPause(t *testing.T) {
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause(), "")
	if d := getSchedulerImmediateLoadBatchPause(); d != 0 {
		t.Fatalf("default: got %v", d)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause(), "nope")
	if d := getSchedulerImmediateLoadBatchPause(); d != 0 {
		t.Fatalf("invalid: got %v", d)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause(), "100ms")
	if d := getSchedulerImmediateLoadBatchPause(); d != 100*time.Millisecond {
		t.Fatalf("parse: got %v", d)
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause(), "200h")
	if d := getSchedulerImmediateLoadBatchPause(); d != maxSchedulerImmediateLoadBatchPause {
		t.Fatalf("clamp: got %v want %v", d, maxSchedulerImmediateLoadBatchPause)
	}
}
