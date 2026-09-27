// Package diagnostics provides pluggable, configurable diagnostic extraction.
//
// The extractor framework lets subsystems register deterministic diagnostic
// extractors with a shared registry. Runs are fail-closed: a single failing
// extractor must not abort the whole run, and cancelled contexts surface as
// explicit per-extractor errors.
package diagnostics

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

// DiagnosticsNameMaxLen bounds extractor names so identifiers stay log-safe.
const DiagnosticsNameMaxLen = 64

// diagsNamePattern enforces a safe, unambiguous identifier (lowercase, digits, '_' or '-').
var diagsNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// DiagnosticArtifact is the value produced by a single extractor.
// Payload is opaque to the registry; consumers own its decoding.
type DiagnosticArtifact struct {
	Name      string
	SizeBytes int64
	MIMEType  string
	Payload   any
}

// String renders a log-safe summary. It deliberately omits Payload bytes.
func (a DiagnosticArtifact) String() string {
	return fmt.Sprintf("DiagnosticArtifact{name=%q size=%d mime=%q}", a.Name, a.SizeBytes, a.MIMEType)
}

// ResultRecord captures the outcome of running one extractor during a report run.
type ResultRecord struct {
	Extractor string
	OK        bool
	Error     error
	Artifact  *DiagnosticArtifact
	Duration  time.Duration
}

// DiagnosticReport is the bounded summary of one registry.Run call.
type DiagnosticReport struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Results    []ResultRecord
}

// OKCount returns the number of successful result records.
func (r *DiagnosticReport) OKCount() int {
	var n int
	for i := range r.Results {
		if r.Results[i].OK {
			n++
		}
	}
	return n
}

// FailedCount returns the number of failed result records.
func (r *DiagnosticReport) FailedCount() int {
	return len(r.Results) - r.OKCount()
}

// Extractor is the pluggable diagnostic extraction contract.
// Implementations must be side-effect-free beyond producing the artifact,
// must respect ctx cancellation, and must be safe for concurrent use.
type Extractor interface {
	// Name is the unique registry identifier (lowercase [a-z0-9_-]).
	Name() string
	// Extract produces the diagnostic value for this subsystem.
	Extract(ctx context.Context) (DiagnosticArtifact, error)
}

// validateExtractorName rejects blank, overly long, or unsafe names.
func validateExtractorName(name string) error {
	if !diagsNamePattern.MatchString(name) {
		return fmt.Errorf("diagnostics: invalid extractor name %q (want 1-%d chars of [a-z0-9_-], starting alnum)", name, DiagnosticsNameMaxLen)
	}
	return nil
}

// DiagnosticExtractorRegistry holds named extractors and runs them deterministically.
// It is safe for concurrent use.
type DiagnosticExtractorRegistry struct {
	mu         sync.RWMutex
	extractors map[string]Extractor
}

// NewDiagnosticExtractorRegistry returns an empty, ready-to-use registry.
func NewDiagnosticExtractorRegistry() *DiagnosticExtractorRegistry {
	return &DiagnosticExtractorRegistry{extractors: make(map[string]Extractor)}
}

// Register adds an extractor under its name.
// It fails closed on nil extractors, malformed names, and any duplicate
// name (even when the previous registration had the same value).
func (r *DiagnosticExtractorRegistry) Register(e Extractor) error {
	if e == nil {
		return fmt.Errorf("diagnostics: refuse to register nil extractor")
	}
	name := e.Name()
	if err := validateExtractorName(name); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.extractors[name]; exists {
		return fmt.Errorf("diagnostics: extractor %q already registered", name)
	}
	r.extractors[name] = e
	return nil
}

// ByName returns the registered extractor under name.
func (r *DiagnosticExtractorRegistry) ByName(name string) (Extractor, error) {
	if err := validateExtractorName(name); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.extractors[name]
	if !ok {
		return nil, fmt.Errorf("diagnostics: extractor %q not registered", name)
	}
	return e, nil
}

// Names returns the registered extractor names in stable, sorted order.
func (r *DiagnosticExtractorRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.extractors))
	for n := range r.extractors {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ExtractorFilter optionally restricts which extractor names this run visits.
// A nil filter means "run every registered extractor" (in sorted name order).
type ExtractorFilter func(name string) bool

// Run executes the selected extractors sequentially in sorted name order and
// returns a DiagnosticReport. One failing extractor never aborts the run;
// its error is recorded in the corresponding ResultRecord. A cancelled ctx
// before an extractor starts is recorded as a failed result (fail-closed).
func (r *DiagnosticExtractorRegistry) Run(ctx context.Context, filter ExtractorFilter) DiagnosticReport {
	names := r.Names()
	report := DiagnosticReport{StartedAt: time.Now()}
	for _, name := range names {
		if filter != nil && !filter(name) {
			continue
		}
		if err := validateExtractorName(name); err != nil {
			report.Results = append(report.Results, ResultRecord{Extractor: name, Error: err})
			continue
		}
		e, err := r.ByName(name)
		if err != nil {
			report.Results = append(report.Results, ResultRecord{Extractor: name, Error: err})
			continue
		}

		rec := ResultRecord{Extractor: name}
		if ctxErr := ctx.Err(); ctxErr != nil {
			rec.Error = fmt.Errorf("diagnostics: extractor %q not started: %w", name, ctxErr)
			report.Results = append(report.Results, rec)
			continue
		}

		start := time.Now()
		art, extractErr := e.Extract(ctx)
		rec.Duration = time.Since(start)
		if extractErr != nil {
			rec.Error = extractErr
			report.Results = append(report.Results, rec)
			continue
		}
		rec.OK = true
		a := art
		if a.Name == "" {
			a.Name = name
		}
		rec.Artifact = &a
		report.Results = append(report.Results, rec)
	}
	report.FinishedAt = time.Now()
	return report
}

// timeoutExtractor wraps an extractor with an additional context deadline.
type timeoutExtractor struct {
	inner   Extractor
	timeout time.Duration
}

// WithTimeout returns a wrapper extractor whose Extract call additionally
// bounds the base extractor by timeout. A non-positive timeout bypasses the
// derived deadline, relying entirely on the caller context.
func WithTimeout(e Extractor, timeout time.Duration) Extractor {
	return timeoutExtractor{inner: e, timeout: timeout}
}

// Name delegates to the wrapped extractor.
func (t timeoutExtractor) Name() string { return t.inner.Name() }

// Extract enforces the derived deadline and calls the wrapped extractor.
func (t timeoutExtractor) Extract(ctx context.Context) (DiagnosticArtifact, error) {
	if t.timeout <= 0 {
		return t.inner.Extract(ctx)
	}
	cctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	return t.inner.Extract(cctx)
}
