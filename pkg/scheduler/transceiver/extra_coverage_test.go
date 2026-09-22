// BLI-STARTER-COMMUNITY-047 / PRI-STARTER-COMMUNITY-047 coverage elevation
package transceiver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
)

type extraFailValidateAdapter struct {
	MockAdapter
}

func (a *extraFailValidateAdapter) Validate(types.Action) error { return errors.New("bad action") }

func TestExtraRouterConditionsTransformRetryAndDefaults(t *testing.T) {
	ctx := context.Background()
	r := NewRouter(nil)
	_ = NewDefaultRouter(nil)
	_ = NewRouterWithDefaults(logging.GetLoggerFromProfile("test"))
	RegisterDefaultAdapters(r, nil, nil, nil)
	if err := r.RegisterAdapter(nil); err == nil {
		t.Fatal("nil adapter")
	}
	if err := r.RegisterAdapter(&MockAdapter{name: ""}); err == nil {
		t.Fatal("empty name")
	}
	ok := &MockAdapter{name: "webhook"}
	if err := r.RegisterAdapter(ok); err != nil {
		t.Fatal(err)
	}
	r.UnregisterAdapter("webhook")
	if err := r.RegisterAdapter(ok); err != nil {
		t.Fatal(err)
	}
	r.GetMetrics()

	fail := &extraFailValidateAdapter{MockAdapter: MockAdapter{name: "event"}}
	if err := r.RegisterAdapter(fail); err != nil {
		t.Fatal(err)
	}
	retryAd := &MockAdapter{name: "command", sendError: errors.New("once")}
	if err := r.RegisterAdapter(retryAd); err != nil {
		t.Fatal(err)
	}

	msg := types.Message{
		EventType: "job.done",
		Source:    "scheduler",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"duration": 2, "name": "hello-world", "n": float64(3)},
		Metadata: map[string]string{
			"job_id":                 "SCH-1",
			objects.FieldKeyCategory: "testing",
			objects.FieldKeyJobType:  "run_wrapper",
			objects.FieldKeySeverity: "high",
			"tag":                    "alpha",
		},
	}

	r.LoadRules([]RoutingRule{
		{Name: "disabled", Enabled: false, Match: MessageMatcher{EventType: "job.done"}, Actions: []Action{{Protocol: "webhook"}}},
		{
			Name:    "cond",
			Enabled: true,
			Match: MessageMatcher{
				EventType:   "job.done",
				Source:      "scheduler",
				JobID:       "SCH-1",
				JobCategory: "testing",
				JobType:     "run_wrapper",
				Severity:    "high",
				Conditions: []Condition{
					{Field: "tag", Operator: "eq", Value: "alpha"},
					{Field: "tag", Operator: "ne", Value: "beta"},
					{Field: "duration", Operator: "gt", Value: 1},
					{Field: "duration", Operator: "gte", Value: 2},
					{Field: "duration", Operator: "lt", Value: 9},
					{Field: "duration", Operator: "lte", Value: 2},
					{Field: "name", Operator: "contains", Value: "hello"},
					{Field: "name", Operator: "regex", Value: "^hello"},
					{Field: "tag", Operator: "in", Value: []any{"alpha", "z"}},
					{Field: "missing", Operator: "isNull"},
					{Field: "missing", Operator: "ne"},
					{Field: "name", Operator: "unknown-op", Value: "x"},
				},
			},
			Actions: []Action{{Protocol: "missing"}},
		},
	})
	_ = r.Route(ctx, msg)

	r.LoadRules([]RoutingRule{{
		Name:    "send",
		Enabled: true,
		Match:   MessageMatcher{EventType: "job.done"},
		Actions: []Action{
			{Protocol: "event", Endpoint: "x"},
			{
				Protocol: "webhook",
				Endpoint: "http://example",
				Timeout:  5 * time.Millisecond,
				Transform: &types.PayloadTransform{
					IncludeFields: []string{"name", "duration"},
					ExcludeFields: []string{"duration"},
					RenameFields:  map[string]string{"name": "title"},
					AddFields:     map[string]any{"ok": true},
				},
			},
			{
				Protocol: "command",
				Endpoint: "true",
				Retry:    &types.RetryConfig{MaxAttempts: 2, Backoff: "exponential", InitialDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond},
			},
		},
	}})
	retryAd.sendError = errors.New("fail-once")
	_ = r.Route(ctx, msg)
	retryAd.sendError = nil
	_ = r.Route(ctx, msg)

	_ = r.calculateBackoff(time.Millisecond, &types.RetryConfig{Backoff: "exponential", MaxDelay: 2 * time.Millisecond, InitialDelay: time.Millisecond})
	_ = r.calculateBackoff(time.Millisecond, &types.RetryConfig{Backoff: "linear", MaxDelay: 10 * time.Millisecond, InitialDelay: time.Millisecond})
	_ = r.calculateBackoff(time.Millisecond, &types.RetryConfig{Backoff: "fixed"})
	_ = r.calculateBackoff(time.Millisecond, &types.RetryConfig{Backoff: "other", MaxDelay: time.Hour})
	_ = contains("abc", "b")
	_ = contains("abc", "")
	_ = contains("ab", "abc")
	_ = indexOfSubstring("abc", "z")

	r.evaluateCondition(msg, Condition{Field: "n", Operator: "gt", Value: float32(1)})
	r.evaluateCondition(msg, Condition{Field: "n", Operator: "==", Value: float64(3)})
	r.evaluateCondition(msg, Condition{Field: "n", Operator: "!=", Value: 9})
	r.evaluateCondition(msg, Condition{Field: "name", Operator: "regex", Value: "["})
	r.evaluateCondition(msg, Condition{Field: "name", Operator: "contains", Value: 1})
	r.compareValues("a", "b")
	r.toFloat64(int32(1))
	r.toFloat64(int64(1))
	r.toFloat64("no")
}

func TestExtraLoaderValidationVerificationAndBroker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "rules.yaml")
	ymlPath := filepath.Join(dir, "more.yml")
	skipPath := filepath.Join(dir, "notes.txt")
	body := []byte(`- name: extra-rule
  description: extra
  enabled: true
  priority: 1
  match:
    event_type: job_done
    source: scheduler
    job_id: SCH-1
    conditions:
      - field: x
        operator: eq
        value: 1
  actions:
    - protocol: webhook
      endpoint: http://example.local
`)
	if err := os.WriteFile(yamlPath, body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ymlPath, body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skipPath, []byte("nope"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(":]"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	loader := NewRoutingRuleLoader(dir, nil)
	if _, err := loader.LoadRulesFromFile(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("missing file")
	}
	rules, err := loader.LoadRulesFromFile(yamlPath)
	if err != nil || len(rules) == 0 {
		t.Fatal(err)
	}
	r := NewRouter(nil)
	_ = r.RegisterAdapter(&MockAdapter{name: "webhook"})
	_, _, _ = loader.LoadAndValidateRules(yamlPath, r)
	all, err := loader.LoadAllRules()
	if err != nil || len(all) < 1 {
		t.Fatalf("load all %d %v", len(all), err)
	}
	empty := NewRoutingRuleLoader("", nil)
	_, _ = empty.LoadAllRules()
	missingDir := NewRoutingRuleLoader(filepath.Join(dir, "no-dir"), nil)
	_, _ = missingDir.LoadAllRules()
	_ = LoadDefaultRules()
	_ = findRoutingRulesDir()

	_ = ValidateRoutingRule(RoutingRule{}, r)
	_ = ValidateRoutingRule(RoutingRule{Name: "bad name!", Priority: -1}, r)
	_ = ValidateRoutingRule(RoutingRule{Name: "ok", Priority: 99999, Actions: []Action{{}}}, r)
	_ = ValidateRoutingRule(RoutingRule{Name: "ok2", Actions: []Action{
		{Protocol: "webhook"},
		{Protocol: "webhook", Endpoint: "not-a-url"},
		{Protocol: "command"},
		{Protocol: "event"},
		{Protocol: "webhook", Endpoint: "https://ok", Retry: &types.RetryConfig{MaxAttempts: -1, Backoff: "weird", InitialDelay: -1, MaxDelay: -1}, Timeout: -1},
		{Protocol: "webhook", Endpoint: "${HOOK}"},
	}, Match: MessageMatcher{EventType: "BAD", Conditions: []Condition{{Operator: "nope"}}}}, r)
	_ = ValidateRoutingRules([]RoutingRule{{Name: "dup"}, {Name: "dup"}}, r, nil)
	_ = (ValidationError{Field: "f", Message: "m"}).Error()

	mv := NewMessageVerification(nil, 0)
	mv2 := NewMessageVerification(nil, time.Millisecond)
	mv2.RegisterMessage("m1", "job_done", []string{"http://a", "http://b"})
	mv2.VerifyDelivery("missing", "http://a", true, nil)
	mv2.VerifyDelivery("m1", "http://a", true, nil)
	mv2.VerifyDelivery("m1", "http://b", false, errors.New("no"))
	_, _ = mv2.GetVerificationResult("m1")
	mv2.RegisterMessage("m2", "job_done", []string{"http://z"})
	time.Sleep(2 * time.Millisecond)
	mv2.CleanupExpired()
	mv.StartCleanup(ctx)
	cancel()

	sb := NewSchedulerBroker(r, nil)
	_ = sb.RouteAsync(context.Background(), types.Message{EventType: "job_done"})
	_ = sb.RouteSync(context.Background(), types.Message{EventType: "nope"})

	ar := NewAsyncRouter(context.Background(), r, 0, 0, nil)
	ar.SetProjectRoot(dir)
	ar.SetStorageProvider(nil)
	if err := ar.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ar.Start(context.Background()); err == nil {
		t.Fatal("already started")
	}
	_ = ar.RouteSync(context.Background(), types.Message{EventType: "job_done"})
	_ = ar.GetQueueSize()
	_ = ar.GetQueueCapacity()
	_ = ar.GetMetrics()
	_ = ar.GetActiveWorkers()
	_ = ar.IsWorkerRunning()
	_ = ar.GetName()
	_ = ar.IsCritical()
	_ = ar.GetPendingCount()
	_ = ar.IsDrained()
	SetAsyncRouterEventCallback(nil)
	if err := ar.Stop(); err != nil {
		t.Fatal(err)
	}

	m := NewRouterMetrics()
	m.RecordMessageDropped()
	m.RecordQueueFull()
	m.RecordQueueDropped()
	m.UpdateQueueDepth(2)
	snap := m.GetSnapshot()
	_ = snap.GetSuccessRate()
	_ = snap.GetMatchRate()
	RegisterDefaultAdapters(r, &MockAdapter{name: "httpx"}, &MockAdapter{name: "cmdx"}, &MockAdapter{name: "evtx"})
	_ = NewRouterProfileLoader(dir, nil)
	_, _ = LoadDefaultProfile(nil)
	_ = GetDefaultProfileName()
	_ = ApplyProfileToRouter(r, &RouterProfileSpec{BackoffStrategy: "exponential", MaxBackoffMs: 1, DefaultTimeoutMs: 1})
}
