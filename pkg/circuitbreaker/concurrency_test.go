package circuitbreaker

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

func TestExtractPackagePathFromRunWrapperCommand(t *testing.T) {
	tests := []struct {
		command string
		args    []string
		want    string
	}{
		{"/bin/sh", []string{"-c", "go test ./pkg/storage -run '^TestX$' -v -count=1 -p 10 > /tmp/out.log 2>&1"}, "pkg/storage"},
		{"/bin/sh", []string{"-c", "go test ./cmd/zqk/utility -run '^TestY$' -v -count=1 -p 10"}, "cmd/zqk/utility"},
		{"/bin/sh", []string{"-c", "echo hello"}, ""},
		{"/bin/sh", []string{"-c", "go test ./..."}, ""},
		{"/bin/sh", []string{"-c", "go test ./pkg/storage"}, "pkg/storage"},
		{"/bin/sh", []string{"-c", "go test ././foo"}, ""},
		{"/bin/sh", []string{"-c", "go test ./"}, ""},
	}
	for _, tt := range tests {
		got := ExtractPackagePathFromRunWrapperCommand(tt.command, tt.args)
		if got != tt.want {
			t.Errorf("ExtractPackagePathFromRunWrapperCommand(%q, %v) = %q, want %q", tt.command, tt.args, got, tt.want)
		}
	}
}

func TestPackageConcurrencyLimiter_AcquireRelease(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, 2*time.Second)
	limiter.MergeLimits(map[string]int{"pkg/storage": 1})

	ctx := context.Background()

	// First acquire for pkg/storage should succeed
	if err := limiter.Acquire(ctx, "pkg/storage"); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	// Second acquire should block; use short timeout to avoid hanging test
	ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	err := limiter.Acquire(ctx2, "pkg/storage")
	cancel()
	if err == nil {
		limiter.Release("pkg/storage") // release the first before failing test
		t.Fatal("expected second Acquire to block until timeout")
	}

	// Release first, then second acquire should succeed
	limiter.Release("pkg/storage")
	if err := limiter.Acquire(ctx, "pkg/storage"); err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	limiter.Release("pkg/storage")
}

func TestPackageConcurrencyLimiter_Snapshot(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	l := NewConcurrencyLimiter(logger, time.Second)
	l.MergeLimits(map[string]int{"pkg/a": 2})

	lim, used := l.Snapshot("pkg/a")
	if lim != 2 || used != 0 {
		t.Fatalf("before acquire: limit=%d inUse=%d want 2,0", lim, used)
	}
	ctx := context.Background()
	if err := l.Acquire(ctx, "pkg/a"); err != nil {
		t.Fatal(err)
	}
	lim, used = l.Snapshot("pkg/a")
	if lim != 2 || used != 1 {
		t.Fatalf("after one acquire: limit=%d inUse=%d want 2,1", lim, used)
	}
	l.Release("pkg/a")
	lim, used = l.Snapshot("pkg/a")
	if lim != 2 || used != 0 {
		t.Fatalf("after release: limit=%d inUse=%d want 2,0", lim, used)
	}
}

func TestPackageConcurrencyLimiter_ShouldLimit(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, time.Minute)
	limiter.MergeLimits(map[string]int{"pkg/storage": 1})

	if !limiter.ShouldLimit("pkg/storage") {
		t.Error("ShouldLimit(pkg/storage) want true")
	}
	if limiter.ShouldLimit("cmd/zqk/utility") {
		t.Error("ShouldLimit(cmd/zqk/utility) want false")
	}
	if limiter.ShouldLimit("") {
		t.Error("ShouldLimit(\"\") want false")
	}
}

func TestPackageConcurrencyLimiter_MergeLimitsZeroRemoves(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, time.Minute)
	limiter.MergeLimits(map[string]int{"pkg/storage": 1})
	if !limiter.ShouldLimit("pkg/storage") {
		t.Fatal("want limit before remove")
	}
	limiter.MergeLimits(map[string]int{"pkg/storage": 0})
	if limiter.ShouldLimit("pkg/storage") {
		t.Fatal("ShouldLimit false after zero merge")
	}
}

func TestPackageConcurrencyLimiter_MergeLimitsIncreases(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, time.Second)
	limiter.MergeLimits(map[string]int{"pkg/a": 2})
	lim, _ := limiter.Snapshot("pkg/a")
	if lim != 2 {
		t.Fatalf("start limit=%d want 2", lim)
	}
	limiter.MergeLimits(map[string]int{"pkg/a": 4})
	lim, _ = limiter.Snapshot("pkg/a")
	if lim != 4 {
		t.Fatalf("after increase limit=%d want 4", lim)
	}
}

func TestPackageConcurrencyLimiter_TrimDotSlashPrefix(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, time.Second)
	limiter.MergeLimits(map[string]int{"./pkg/test": 1})
	if !limiter.ShouldLimit("pkg/test") {
		t.Fatal("ShouldLimit(pkg/test) want true after merging ./pkg/test")
	}
}

func TestPackageConcurrencyLimiter_EmptyPackagePath(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, time.Second)
	ctx := context.Background()
	if err := limiter.Acquire(ctx, ""); err != nil {
		t.Fatalf("Acquire empty path: %v", err)
	}
	limiter.Release("")
}

func TestPackageConcurrencyLimiter_MergeLimitsNilNoOp(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, time.Second)
	limiter.MergeLimits(map[string]int{"pkg/test": 1})
	limiter.MergeLimits(nil)
	if !limiter.ShouldLimit("pkg/test") {
		t.Fatal("nil merge should leave pkg/test limited")
	}
}
