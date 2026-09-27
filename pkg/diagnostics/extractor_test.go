package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// counterExtractor is a deterministic pluggable extractor for registry tests.
type counterExtractor struct {
	name    string
	payload any
	err     error
	calls   *int32
	block   <-chan struct{}
}

func (c counterExtractor) Name() string { return c.name }

func (c counterExtractor) Extract(ctx context.Context) (DiagnosticArtifact, error) {
	if c.calls != nil {
		atomic.AddInt32(c.calls, 1)
	}
	if c.block != nil {
		select {
		case <-c.block:
		case <-ctx.Done():
			return DiagnosticArtifact{}, fmt.Errorf("counterExtractor %s: %w", c.name, ctx.Err())
		}
	}
	if c.err != nil {
		return DiagnosticArtifact{}, c.err
	}
	return DiagnosticArtifact{Payload: c.payload, MIMEType: "application/octet-stream"}, nil
}

// TestRegistryRegisterValid ensures a well-formed extraction result is accepted.
func TestRegistryRegisterValid(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()
	if err := reg.Register(counterExtractor{name: "memory", payload: map[string]int{"rss": 1}}); err != nil {
		t.Fatalf("unexpected registration error: %v", err)
	}
	got, err := reg.ByName("memory")
	if err != nil {
		t.Fatalf("unexpected lookup error: %v", err)
	}
	if got.Name() != "memory" {
		t.Fatalf("want name %q, got %q", "memory", got.Name())
	}
}

// TestRegistryRegisterInvalid ensures fail-closed behavior for malformed results.
func TestRegistryRegisterInvalid(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()

	if err := reg.Register(counterExtractor{name: "   "}); err == nil {
		t.Fatal("want error for blank name, got none")
	}
	if err := reg.Register(counterExtractor{name: "bad/char"}); err == nil {
		t.Fatal("want error for invalid name characters, got none")
	}
	long := make([]byte, DiagnosticsNameMaxLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := reg.Register(counterExtractor{name: string(long)}); err == nil {
		t.Fatal("want error for overly long name, got none")
	}
	if err := reg.Register(nil); err == nil {
		t.Fatal("want error for nil extractor, got none")
	}

	if _, err := reg.ByName("missing"); err == nil {
		t.Fatal("want not-found error, got nil")
	}
}

// TestRegistryRegisterDuplicate ensures duplicates are rejected and registry is stable.
func TestRegistryRegisterDuplicate(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()
	if err := reg.Register(counterExtractor{name: "memory", payload: 1}); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := reg.Register(counterExtractor{name: "memory", payload: 2}); err == nil {
		t.Fatal("want duplicate rejection, got nil error")
	}

	names := reg.Names()
	if len(names) != 1 || names[0] != "memory" {
		t.Fatalf("want single entry [memory], got %v", names)
	}
	got, err := reg.ByName("memory")
	if err != nil {
		t.Fatalf("lookup after duplicate rejection failed: %v", err)
	}
	art, err := got.Extract(context.Background())
	if err != nil || art.Payload != 1 {
		t.Fatalf("original registration must remain active after duplicate rejection, got art=%+v err=%v", art, err)
	}
}

// TestRegistryNamesSorted ensures deterministic enumeration ordering.
func TestRegistryNamesSorted(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()
	for _, n := range []string{"zeta", "alpha", "mid"} {
		if err := reg.Register(counterExtractor{name: n}); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
	}
	want := []string{"alpha", "mid", "zeta"}
	got := reg.Names()
	if len(got) != len(want) {
		t.Fatalf("want %d names, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want sorted %v, got %v", want, got)
		}
	}
}

// TestRegistryRunAllFailures ensures one failing result does not abort the run.
func TestRegistryRunAllFailures(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()
	boom := errors.New("synthetic failure")
	if err := reg.Register(counterExtractor{name: "ok", payload: true}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(counterExtractor{name: "bad", err: boom}); err != nil {
		t.Fatal(err)
	}

	rep := reg.Run(context.Background(), nil)
	if len(rep.Results) != 2 {
		t.Fatalf("want 2 results, got %d", len(rep.Results))
	}
	var bad *ResultRecord
	var okCnt int
	for i := range rep.Results {
		rec := &rep.Results[i]
		if !rec.OK {
			if !errors.Is(rec.Error, boom) {
				t.Fatalf("want original error %v, got %v", boom, rec.Error)
			}
			bad = rec
			continue
		}
		okCnt++
	}
	if bad == nil {
		t.Fatal("want at least one failed result")
	}
	if okCnt != 1 {
		t.Fatalf("want 1 successful result, got %d", okCnt)
	}
	if rep.FailedCount() != 1 || rep.OKCount() != 1 {
		t.Fatalf("want 1 failed / 1 ok, got %d failed / %d ok", rep.FailedCount(), rep.OKCount())
	}
	if rep.StartedAt.After(rep.FinishedAt) {
		t.Fatal("report timestamps out of order")
	}
}

// TestRegistryRunContextCancelled ensures ctx cancellation is fail-closed.
func TestRegistryRunContextCancelled(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()
	if err := reg.Register(counterExtractor{name: "any", block: make(chan struct{})}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rep := reg.Run(ctx, nil)
	if len(rep.Results) != 1 {
		t.Fatalf("want 1 result, got %d", len(rep.Results))
	}
	if rep.Results[0].OK {
		t.Fatal("want failure on cancelled context, got success")
	}
}

// TestRegistryRunFilter ensures filters deterministically limit the run.
func TestRegistryRunFilter(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()
	for _, n := range []string{"alpha", "beta", "gamma"} {
		if err := reg.Register(counterExtractor{name: n}); err != nil {
			t.Fatal(err)
		}
	}
	rep := reg.Run(context.Background(), func(name string) bool { return name == "beta" })
	if len(rep.Results) != 1 {
		t.Fatalf("want 1 result, got %d", len(rep.Results))
	}
	if rep.Results[0].Extractor != "beta" || !rep.Results[0].OK {
		t.Fatalf("want successful beta result, got %+v", rep.Results[0])
	}
}

// TestRegistryRunConcurrent ensures repeated concurrent runs are goroutine-safe.
func TestRegistryRunConcurrent(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()
	for i := 0; i < 4; i++ {
		if err := reg.Register(counterExtractor{name: fmt.Sprintf("probe_%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rep := reg.Run(context.Background(), nil)
			if len(rep.Results) != 4 {
				t.Errorf("want 4 results, got %d", len(rep.Results))
			}
		}()
	}
	wg.Wait()
}

// TestWithTimeoutDeadlineExceeded ensures timeout bounds long-running results.
func TestWithTimeoutDeadlineExceeded(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	slow := WithTimeout(counterExtractor{name: "slow", block: block}, 50*time.Millisecond)

	start := time.Now()
	_, err := slow.Extract(context.Background())
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	// The blocked extractor is released by the wrapper's derived deadline.
	if elapsed > 5*time.Second {
		t.Fatalf("timeout wrapper took too long: %v", elapsed)
	}
}

// TestWithTimeoutWithinBudget ensures a fast result succeeds under the budget.
func TestWithTimeoutWithinBudget(t *testing.T) {
	fast := WithTimeout(counterExtractor{name: "fast", payload: 42}, 2*time.Second)
	art, err := fast.Extract(context.Background())
	if err != nil {
		t.Fatalf("fast extraction should succeed under budget, got %v", err)
	}
	if art.Payload != 42 {
		t.Fatalf("want payload 42, got %v", art.Payload)
	}
}

// TestWithTimeoutRespectsCallerCancellation ensures caller ctx is authoritative.
func TestWithTimeoutRespectsCallerCancellation(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	slow := WithTimeout(counterExtractor{name: "slow", block: block}, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := slow.Extract(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// TestDiagnosticArtifactString ensures the artifact summary is safe to log.
func TestDiagnosticArtifactString(t *testing.T) {
	a := DiagnosticArtifact{Name: "mem", SizeBytes: 3, MIMEType: "text/plain"}
	s := a.String()
	if !strings.Contains(s, "mem") {
		t.Fatalf("summary should contain artifact name, got %q", s)
	}
}

// TestDiagnosticReportDerived ensures counters reflect the result records.
func TestDiagnosticReportDerived(t *testing.T) {
	rep := &DiagnosticReport{
		Results: []ResultRecord{
			{Extractor: "a", OK: true},
			{Extractor: "b", OK: true},
			{Extractor: "c", OK: false, Error: errors.New("x")},
		},
	}
	if rep.OKCount() != 2 || rep.FailedCount() != 1 {
		t.Fatalf("want 2 ok / 1 failed, got %d / %d", rep.OKCount(), rep.FailedCount())
	}
}
