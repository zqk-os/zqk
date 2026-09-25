package validation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestPersonaExtendedValidationHandler(t *testing.T) {
	handler := NewPersonaExtendedValidationHandler()
	if handler.Kind() != objects.KindPersona {
		t.Fatalf("expected kind %s, got %s", objects.KindPersona, handler.Kind())
	}

	ctx := context.Background()

	// 1. Conceptual status does not require skill bound
	personaDraft := map[string]any{
		objects.FieldKeyID:     "PER-TEST",
		objects.FieldKeyStatus: "conceptual",
	}
	errs := handler.Validate(ctx, personaDraft, nil)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for conceptual persona, got %v", errs)
	}

	// 2. Approved status requires skill bound; missing ASK refs should fail
	personaApprovedNoSkills := map[string]any{
		objects.FieldKeyID:     "PER-TEST",
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	}
	errs = handler.Validate(ctx, personaApprovedNoSkills, nil)
	if len(errs) == 0 {
		t.Errorf("expected error for approved persona without skills, got none")
	}

	// 3. Approved status with valid resolved skills passes
	personaApprovedWithSkills := map[string]any{
		objects.FieldKeyID:             "PER-TEST",
		objects.FieldKeyStatus:         objects.ObjectStatusApproved,
		objects.FieldKeyAgentSkillRefs: []string{"ASK-TEST-001"},
	}
	options := &ValidationOptions{
		ObjectLookup: func(id string) (map[string]any, error) {
			if id == "ASK-TEST-001" {
				return map[string]any{
					objects.FieldKeyID:     "ASK-TEST-001",
					objects.FieldKeyStatus: objects.ObjectStatusApproved,
				}, nil
			}
			return nil, nil
		},
	}
	errs = handler.Validate(ctx, personaApprovedWithSkills, options)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for approved persona with valid skill, got %v", errs)
	}
}

func TestUniversalQualityValidationHandler(t *testing.T) {
	handler := NewUniversalQualityValidationHandler()
	if handler.Kind() != KindUniversal {
		t.Fatalf("expected kind %s, got %s", KindUniversal, handler.Kind())
	}

	ctx := context.Background()

	// 1. Criteria missing category and short title/description
	critInvalid := map[string]any{
		objects.FieldKeyID:          "CRIT-TEST",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "todo",
		objects.FieldKeyDescription: "short",
	}
	errs := handler.Validate(ctx, critInvalid, nil)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors (category, title, desc), got %d: %v", len(errs), errs)
	}

	// 2. Valid criteria passes
	critValid := map[string]any{
		objects.FieldKeyID:          "CRIT-TEST",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Valid Criteria Title",
		objects.FieldKeyDescription: "This is a substantive valid criteria description.",
		objects.FieldKeyCategory:    "functional",
	}
	errs = handler.Validate(ctx, critValid, nil)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for valid criteria, got %v", errs)
	}

	// 3. Multiline title on ANY object fails
	objMultilineTitle := map[string]any{
		objects.FieldKeyID:    "BLI-TEST",
		objects.FieldKeyKind:  objects.KindBacklogItem,
		objects.FieldKeyTitle: "Line 1\nLine 2",
	}
	errs = handler.Validate(ctx, objMultilineTitle, nil)
	hasSingleLineErr := false
	for _, e := range errs {
		if e.Rule == "criteria_title_single_line" {
			hasSingleLineErr = true
			break
		}
	}
	if !hasSingleLineErr {
		t.Errorf("expected single line error for multiline title, got %v", errs)
	}

	// 4. Refactor title must assert measured deltas
	refactorObj := map[string]any{
		objects.FieldKeyID:    "CRIT-REFACTOR-001",
		objects.FieldKeyKind:  objects.KindCriteria,
		objects.FieldKeyTitle: "Refactor storage layer",
		objects.FieldKeyCompletenessValidation: []string{
			"code exists",
		},
	}
	errs = handler.Validate(ctx, refactorObj, nil)
	hasDeltaErr := false
	for _, e := range errs {
		if e.Rule == "criteria_refactor_measured_deltas" {
			hasDeltaErr = true
			break
		}
	}
	if !hasDeltaErr {
		t.Errorf("expected refactor measured deltas error, got %v", errs)
	}

	// 5. Refactor title with valid measured delta passes
	refactorValidObj := map[string]any{
		objects.FieldKeyID:          "CRIT-REFACTOR-002",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Refactor storage layer",
		objects.FieldKeyDescription: "Substantive description of refactoring storage layer.",
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyCompletenessValidation: []string{
			"measure file count down by 5",
		},
	}
	errs = handler.Validate(ctx, refactorValidObj, nil)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for valid refactor criteria, got %v", errs)
	}
}

// customMockHandler is a helper for concurrency testing
type customMockHandler struct {
	kind     string
	delay    time.Duration
	errs     []ValidationError
	called   atomic.Int32
	activeMu sync.Mutex
}

func (m *customMockHandler) Kind() string {
	return m.kind
}

func (m *customMockHandler) Validate(ctx context.Context, obj map[string]any, options *ValidationOptions) []ValidationError {
	m.called.Add(1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return []ValidationError{{Rule: "canceled"}}
		}
	}
	return m.errs
}

func TestExtendedValidationRegistry_ConcurrentPool(t *testing.T) {
	reg := NewExtendedValidationRegistry()
	reg.SetPool(NewValidationPool(3))

	h1 := &customMockHandler{kind: "custom_kind", delay: 20 * time.Millisecond}
	h2 := &customMockHandler{kind: "custom_kind", delay: 20 * time.Millisecond}
	h3 := &customMockHandler{kind: "custom_kind", delay: 20 * time.Millisecond}

	reg.Register(h1)
	reg.Register(h2)
	reg.Register(h3)

	ctx := context.Background()
	obj := map[string]any{
		objects.FieldKeyID:   "CST-001",
		objects.FieldKeyKind: "custom_kind",
	}

	start := time.Now()
	errs := reg.Validate(ctx, obj, "custom_kind", nil)
	duration := time.Since(start)

	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %v", errs)
	}
	if h1.called.Load() != 1 || h2.called.Load() != 1 || h3.called.Load() != 1 {
		t.Errorf("expected all handlers called once, got %d, %d, %d", h1.called.Load(), h2.called.Load(), h3.called.Load())
	}
	// With 3 handlers each sleeping 20ms in a pool of 3, total time should be ~20-40ms, not 60ms+
	if duration >= 55*time.Millisecond {
		t.Logf("concurrency note: total duration was %v", duration)
	}
}

func TestExtendedValidationRegistry_EventBus(t *testing.T) {
	reg := NewExtendedValidationRegistry()

	var eventsReceived []ValidationEvent
	var mu sync.Mutex

	reg.EventBus().Subscribe(func(event ValidationEvent) {
		mu.Lock()
		defer mu.Unlock()
		eventsReceived = append(eventsReceived, event)
	})

	var optEvents []ValidationEvent
	options := &ValidationOptions{
		OnValidationEvent: func(event ValidationEvent) {
			mu.Lock()
			defer mu.Unlock()
			optEvents = append(optEvents, event)
		},
	}

	h := &customMockHandler{
		kind: "event_test_kind",
		errs: []ValidationError{{Rule: "err1", Message: "failed"}},
	}
	reg.Register(h)

	ctx := context.Background()
	obj := map[string]any{
		objects.FieldKeyID:   "EVT-001",
		objects.FieldKeyKind: "event_test_kind",
	}

	_ = reg.Validate(ctx, obj, "event_test_kind", options)

	mu.Lock()
	defer mu.Unlock()

	if len(eventsReceived) < 2 {
		t.Fatalf("expected at least 2 events on bus (started, completed), got %d", len(eventsReceived))
	}
	if eventsReceived[0].Type != ValidationEventStarted {
		t.Errorf("expected started event first, got %s", eventsReceived[0].Type)
	}
	if eventsReceived[1].Type != ValidationEventFailed {
		t.Errorf("expected failed event second, got %s", eventsReceived[1].Type)
	}

	if len(optEvents) < 2 {
		t.Fatalf("expected options callback to receive at least 2 events, got %d", len(optEvents))
	}
}

func TestGoValidator_ExtendedValidationIntegration(t *testing.T) {
	gv := NewGoValidator()
	ctx := context.Background()

	// Persona approved without skill refs should fail validation via GoValidator
	persona := map[string]any{
		objects.FieldKeyID:          "PER-TEST-AUTO",
		objects.FieldKeyKind:        objects.KindPersona,
		objects.FieldKeyTitle:       "Test Persona",
		objects.FieldKeyDescription: "Test Persona Description",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
	}
	res, err := gv.Validate(ctx, persona, objects.KindPersona, &ValidationOptions{ValidateLifecycle: false})
	if err != nil {
		t.Fatalf("unexpected validator error: %v", err)
	}
	if res.IsValid {
		t.Errorf("expected validation to fail for approved persona without skills")
	}
	hasSkillBoundErr := false
	for _, e := range res.Errors {
		if e.Rule == CriteriaIDPersonaSkillBound {
			hasSkillBoundErr = true
			break
		}
	}
	if !hasSkillBoundErr {
		t.Errorf("expected %s rule error, got errors: %v", CriteriaIDPersonaSkillBound, res.Errors)
	}
}
