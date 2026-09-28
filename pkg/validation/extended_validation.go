package validation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// KindUniversal is the wildcard kind matching all object kinds.
const KindUniversal = "*"

// ExtendedValidationHandler evaluates domain-specific or universal quality invariants
// beyond generic spec schema validation. The handler is bound to its target kind upon instantiation.
type ExtendedValidationHandler interface {
	// Kind returns the object kind this handler evaluates (e.g. objects.KindPersona, KindUniversal).
	Kind() string

	// Validate evaluates extended invariants for the object. The handler already knows its kind.
	Validate(ctx context.Context, obj map[string]any, options *ValidationOptions) []ValidationError
}

// ValidationEventType defines the lifecycle event type during validation.
type ValidationEventType string

const (
	ValidationEventStarted   ValidationEventType = "validation_started"
	ValidationEventPassed    ValidationEventType = "validation_passed"
	ValidationEventFailed    ValidationEventType = "validation_failed"
	ValidationEventCompleted ValidationEventType = "validation_completed"
)

// ValidationEvent represents an event emitted during object validation.
type ValidationEvent struct {
	Type      ValidationEventType `json:"type"`
	Kind      string              `json:"kind"`
	ObjectID  string              `json:"object_id"`
	Timestamp time.Time           `json:"timestamp"`
	Duration  time.Duration       `json:"duration"`
	Errors    []ValidationError   `json:"errors,omitempty"`
	Metadata  map[string]any      `json:"metadata,omitempty"`
}

// ValidationEventListener receives validation events.
type ValidationEventListener func(event ValidationEvent)

// ValidationEventBus manages pub-sub distribution of validation events.
type ValidationEventBus struct {
	mu        sync.RWMutex
	listeners []ValidationEventListener
}

// NewValidationEventBus creates a new validation event bus.
func NewValidationEventBus() *ValidationEventBus {
	return &ValidationEventBus{
		listeners: make([]ValidationEventListener, 0),
	}
}

// Subscribe registers a listener for validation events.
func (b *ValidationEventBus) Subscribe(l ValidationEventListener) {
	if l == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners = append(b.listeners, l)
}

// Publish emits a validation event to all registered listeners.
func (b *ValidationEventBus) Publish(event ValidationEvent) {
	if b == nil {
		return
	}
	b.mu.RLock()
	listeners := append([]ValidationEventListener(nil), b.listeners...)
	b.mu.RUnlock()

	for _, l := range listeners {
		l(event)
	}
}

// ValidationPool manages bounded concurrent execution of validation tasks to prevent blocking hot paths.
type ValidationPool struct {
	concurrencyLimit int
	sem              chan struct{}
}

// NewValidationPool creates a new validation worker pool with bounded concurrency.
func NewValidationPool(concurrencyLimit int) *ValidationPool {
	if concurrencyLimit <= 0 {
		concurrencyLimit = 4
	}
	return &ValidationPool{
		concurrencyLimit: concurrencyLimit,
		sem:              make(chan struct{}, concurrencyLimit),
	}
}

// ConcurrencyLimit returns the maximum number of concurrent validations.
func (p *ValidationPool) ConcurrencyLimit() int {
	if p == nil {
		return 1
	}
	return p.concurrencyLimit
}

// ExtendedValidationRegistry manages kind-specific and universal validation handlers.
type ExtendedValidationRegistry struct {
	mu                sync.RWMutex
	universalHandlers []ExtendedValidationHandler
	handlers          map[string][]ExtendedValidationHandler
	pool              *ValidationPool
	eventBus          *ValidationEventBus
}

var (
	globalExtendedValidationRegistry *ExtendedValidationRegistry
	extendedValidationRegistryOnce   sync.Once
)

// GetGlobalExtendedValidationRegistry returns the singleton registry of extended validation handlers.
func GetGlobalExtendedValidationRegistry() *ExtendedValidationRegistry {
	extendedValidationRegistryOnce.Do(func() {
		globalExtendedValidationRegistry = NewExtendedValidationRegistry()
		globalExtendedValidationRegistry.Register(NewUniversalQualityValidationHandler())
		globalExtendedValidationRegistry.Register(NewPersonaExtendedValidationHandler())
	})
	return globalExtendedValidationRegistry
}

// NewExtendedValidationRegistry creates a new extended validation registry with a concurrency pool and event bus.
func NewExtendedValidationRegistry() *ExtendedValidationRegistry {
	return &ExtendedValidationRegistry{
		universalHandlers: make([]ExtendedValidationHandler, 0),
		handlers:          make(map[string][]ExtendedValidationHandler),
		pool:              NewValidationPool(4),
		eventBus:          NewValidationEventBus(),
	}
}

// EventBus returns the registry's validation event bus.
func (r *ExtendedValidationRegistry) EventBus() *ValidationEventBus {
	return r.eventBus
}

// SetPool overrides the validation worker pool.
func (r *ExtendedValidationRegistry) SetPool(pool *ValidationPool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pool = pool
}

// Register registers an extended validation handler bound to its kind (or universal if Kind == "*").
func (r *ExtendedValidationRegistry) Register(h ExtendedValidationHandler) {
	if h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kind := h.Kind()
	if kind == KindUniversal || kind == "" {
		r.universalHandlers = append(r.universalHandlers, h)
	} else {
		r.handlers[kind] = append(r.handlers[kind], h)
	}
}

// Validate executes all applicable universal and kind-specific handlers against obj concurrently.
func (r *ExtendedValidationRegistry) Validate(ctx context.Context, obj map[string]any, kind string, options *ValidationOptions) []ValidationError {
	if r == nil || obj == nil {
		return nil
	}
	if kind == "" {
		kind, _ = obj[objects.FieldKeyKind].(string)
	}

	r.mu.RLock()
	handlers := append([]ExtendedValidationHandler(nil), r.universalHandlers...)
	if kind != "" && kind != KindUniversal {
		handlers = append(handlers, r.handlers[kind]...)
	}
	pool := r.pool
	bus := r.eventBus
	r.mu.RUnlock()

	if len(handlers) == 0 {
		return nil
	}

	objectID, _ := obj[objects.FieldKeyID].(string)
	startTime := time.Now()

	// Emit validation started event
	startEvent := ValidationEvent{
		Type:      ValidationEventStarted,
		Kind:      kind,
		ObjectID:  objectID,
		Timestamp: startTime,
	}
	if bus != nil {
		bus.Publish(startEvent)
	}
	if options != nil && options.OnValidationEvent != nil {
		options.OnValidationEvent(startEvent)
	}

	var errs []ValidationError

	if len(handlers) == 1 {
		// Fast path: single handler executes directly on current goroutine
		errs = handlers[0].Validate(ctx, obj, options)
	} else {
		// Multi-handler path: execute concurrently bounded by validation pool
		var wg sync.WaitGroup
		var errsMu sync.Mutex

		for _, h := range handlers {
			select {
			case <-ctx.Done():
				errsMu.Lock()
				errs = append(errs, ValidationError{
					Field:   objects.FieldKeyID,
					Message: ctx.Err().Error(),
					Rule:    "context_canceled",
				})
				errsMu.Unlock()
				break
			default:
			}

			handler := h
			goroutinelabels.NewGoroutine("extended_validation_handler", "running extended validation handler").
				WithContext(ctx).
				WithWaitGroup(&wg).
				StartSimple(func() {
					// Acquire pool slot
					if pool != nil && pool.sem != nil {
						select {
						case pool.sem <- struct{}{}:
							defer func() { <-pool.sem }()
						case <-ctx.Done():
							return
						}
					}

					subErrs := handler.Validate(ctx, obj, options)
					if len(subErrs) > 0 {
						errsMu.Lock()
						errs = append(errs, subErrs...)
						errsMu.Unlock()
					}
				})
		}
		wg.Wait()
	}

	duration := time.Since(startTime)
	var finalType ValidationEventType
	if len(errs) > 0 {
		finalType = ValidationEventFailed
	} else {
		finalType = ValidationEventPassed
	}

	completedEvent := ValidationEvent{
		Type:      finalType,
		Kind:      kind,
		ObjectID:  objectID,
		Timestamp: time.Now(),
		Duration:  duration,
		Errors:    errs,
	}
	if bus != nil {
		bus.Publish(completedEvent)
	}
	if options != nil && options.OnValidationEvent != nil {
		options.OnValidationEvent(completedEvent)
	}

	return errs
}

var (
	specEnumCache sync.Map // key: "kind:fieldName" -> []string
)

// GetSpecEnumValues returns the valid enum values defined in the spec for a given kind and field.
// Cached thread-safely in memory so specs are loaded once and queried without repeated disk I/O.
func GetSpecEnumValues(kind, fieldName string) []string {
	if kind == "" || fieldName == "" {
		return nil
	}
	cacheKey := kind + ":" + fieldName
	if val, ok := specEnumCache.Load(cacheKey); ok {
		return val.([]string)
	}

	loader := objects.GetGlobalSpecLoader()
	if loader == nil {
		loader = objects.NewSpecLoader("")
	}

	spec, err := loader.LoadSpecWithInheritance(kind + ".yaml")
	if err == nil && spec != nil && spec.ResolvedFields != nil {
		if fieldDef, ok := spec.ResolvedFields[fieldName].(map[string]any); ok {
			if valMap, ok := fieldDef["validation"].(map[string]any); ok {
				if rawEnum, ok := valMap["enum"]; ok {
					var enumValues []string
					switch items := rawEnum.(type) {
					case []any:
						for _, item := range items {
							if s, ok := item.(string); ok && s != "" {
								enumValues = append(enumValues, s)
							}
						}
					case []string:
						enumValues = append([]string(nil), items...)
					}
					if len(enumValues) > 0 {
						specEnumCache.Store(cacheKey, enumValues)
						return enumValues
					}
				}
			}
		}
	}

	// Fallback for criteria category when spec directory is unavailable in isolated unit tests
	if kind == objects.KindCriteria && fieldName == objects.FieldKeyCategory {
		canonicalCriteriaCategories := []string{
			"functional", "non-functional", "acceptance", "test", "performance", "security", "compliance",
		}
		specEnumCache.Store(cacheKey, canonicalCriteriaCategories)
		return canonicalCriteriaCategories
	}

	return nil
}

// UniversalQualityValidationHandler evaluates universal object quality invariants across all kinds:
// category enum validity (read from spec enum), substantive titles (min 5, single-line, non-placeholder),
// substantive descriptions (min 10), and refactor/extraction measured deltas.
type UniversalQualityValidationHandler struct{}

// NewUniversalQualityValidationHandler instantiates a new UniversalQualityValidationHandler bound to KindUniversal.
func NewUniversalQualityValidationHandler() *UniversalQualityValidationHandler {
	return &UniversalQualityValidationHandler{}
}

func (h *UniversalQualityValidationHandler) Kind() string {
	return KindUniversal
}

func (h *UniversalQualityValidationHandler) Validate(ctx context.Context, obj map[string]any, options *ValidationOptions) []ValidationError {
	if obj == nil {
		return nil
	}
	var errs []ValidationError
	kind := strings.ToLower(strings.TrimSpace(objects.GetString(obj, objects.FieldKeyKind)))

	// 1. Criteria / Object category validation using spec-defined enum
	cat := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyCategory))
	validEnums := GetSpecEnumValues(kind, objects.FieldKeyCategory)
	if len(validEnums) > 0 {
		if cat == "" {
			if kind == objects.KindCriteria {
				errs = append(errs, ValidationError{
					Field:   objects.FieldKeyCategory,
					Message: fmt.Sprintf("criteria requires category (%s)", strings.Join(validEnums, "|")),
					Rule:    "criteria_category_required",
				})
			}
		} else {
			isValid := false
			for _, e := range validEnums {
				if strings.EqualFold(cat, e) {
					isValid = true
					break
				}
			}
			if !isValid {
				errs = append(errs, ValidationError{
					Field:   objects.FieldKeyCategory,
					Message: fmt.Sprintf("%s category %q is invalid; must be one of: %s", kind, cat, strings.Join(validEnums, ", ")),
					Rule:    "criteria_category_valid_enum",
				})
			}
		}
	}

	// 2. Title validation (min 5 chars, single-line, non-placeholder)
	title := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyTitle))
	if kind == objects.KindCriteria && title == "" {
		errs = append(errs, ValidationError{
			Field:   objects.FieldKeyTitle,
			Message: "criteria requires a non-empty title",
			Rule:    "criteria_title_required",
		})
	} else if title != "" {
		if len(title) < 5 {
			errs = append(errs, ValidationError{
				Field:   objects.FieldKeyTitle,
				Message: fmt.Sprintf("title %q is too short (min 5 characters)", title),
				Rule:    "criteria_title_min_length",
			})
		} else if strings.Contains(title, "\n") {
			errs = append(errs, ValidationError{
				Field:   objects.FieldKeyTitle,
				Message: "title must be single line",
				Rule:    "criteria_title_single_line",
			})
		} else {
			lowerTitle := strings.ToLower(title)
			if lowerTitle == "required" || lowerTitle == "todo" || lowerTitle == "title" || lowerTitle == "placeholder" {
				errs = append(errs, ValidationError{
					Field:   objects.FieldKeyTitle,
					Message: fmt.Sprintf("title cannot be placeholder %q", title),
					Rule:    "criteria_title_non_placeholder",
				})
			}
		}
	}

	// 3. Description validation (min 10 chars, non-placeholder)
	desc := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyDescription))
	if kind == objects.KindCriteria && desc == "" {
		errs = append(errs, ValidationError{
			Field:   objects.FieldKeyDescription,
			Message: "criteria requires a non-empty description (min 10 characters)",
			Rule:    "criteria_description_required",
		})
	} else if desc != "" && len(desc) < 10 {
		errs = append(errs, ValidationError{
			Field:   objects.FieldKeyDescription,
			Message: "description is too short (min 10 characters)",
			Rule:    "criteria_description_min_length",
		})
	}

	// 4. Refactor / extraction criteria measured deltas
	titleLower := strings.ToLower(title)
	if kind == objects.KindCriteria && (strings.Contains(titleLower, "refactor") || strings.Contains(titleLower, "extraction")) {
		valid := false
		vals := stringSliceField(obj[objects.FieldKeyCompletenessValidation])
		for _, v := range vals {
			str := strings.ToLower(v)
			if strings.Contains(str, "measure") || strings.Contains(str, "count") || strings.Contains(str, "assert") {
				valid = true
				break
			}
		}
		if !valid {
			errs = append(errs, ValidationError{
				Field:   objects.FieldKeyCompletenessValidation,
				Message: "Refactor or extraction criteria must assert measured deltas (e.g. file count down by N, symbol absent), not mere existence prose.",
				Rule:    "criteria_refactor_measured_deltas",
			})
		}
	}

	return errs
}

// PersonaExtendedValidationHandler evaluates persona-specific invariants,
// including CRI-PERSONA-SKILL-BOUND (shovel-ready+ personas must resolve ASK links).
type PersonaExtendedValidationHandler struct {
	kind string
}

// NewPersonaExtendedValidationHandler instantiates a new PersonaExtendedValidationHandler bound to objects.KindPersona.
func NewPersonaExtendedValidationHandler() *PersonaExtendedValidationHandler {
	return &PersonaExtendedValidationHandler{kind: objects.KindPersona}
}

func (h *PersonaExtendedValidationHandler) Kind() string {
	if h.kind != "" {
		return h.kind
	}
	return objects.KindPersona
}

func (h *PersonaExtendedValidationHandler) Validate(ctx context.Context, obj map[string]any, options *ValidationOptions) []ValidationError {
	if obj == nil {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(objects.GetString(obj, objects.FieldKeyStatus)))
	switch status {
	case objects.ObjectStatusApproved, objects.ObjectStatusInProgress, objects.ObjectStatusImplemented:
		// Shovel-ready status requires skill bound
	default:
		return nil
	}

	var resolve ASKResolver
	if options != nil && options.ObjectLookup != nil {
		resolve = func(askID string) (map[string]any, error) {
			return options.ObjectLookup(askID)
		}
	}

	res := EvaluatePersonaSkillBound(obj, resolve)
	if !res.Bound {
		return []ValidationError{
			{
				Field:   objects.FieldKeyAgentSkillRefs,
				Message: fmt.Sprintf("%s: missing/unresolved agent_skill link (%v)", CriteriaIDPersonaSkillBound, res.Missing),
				Rule:    CriteriaIDPersonaSkillBound,
			},
		}
	}
	return nil
}
