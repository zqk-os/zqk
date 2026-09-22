package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// 1. Handler & MethodRouter & TraceMiddleware comprehensive tests
func TestDeep8_HandlerAndMiddlewares(t *testing.T) {
	ctx := context.Background()

	// MethodRouter
	mr := NewMethodRouter()
	mr.RegisterFunc("test.ping", func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "pong", nil
	})
	mr.Register("test.echo", HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return string(params), nil
	}))

	// Hit registered method
	res, err := mr.Handle(ctx, "test.ping", nil)
	if err != nil || res != "pong" {
		t.Errorf("expected pong, got %v, %v", res, err)
	}

	// Method not found without default handler
	_, err = mr.Handle(ctx, "unknown.method", nil)
	if err == nil {
		t.Error("expected method not found error")
	}

	// Set default handler and hit it
	mr.SetDefaultHandler(HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "default:" + method, nil
	}))
	resDef, err := mr.Handle(ctx, "unknown.method", nil)
	if err != nil || resDef != "default:unknown.method" {
		t.Errorf("expected default response, got %v, %v", resDef, err)
	}

	// Chain
	mw1 := func(h Handler) Handler {
		return HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			return h.Handle(ctx, method+"_mw1", params)
		})
	}
	mw2 := func(h Handler) Handler {
		return HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			return h.Handle(ctx, method+"_mw2", params)
		})
	}
	chained := Chain(mw1, mw2)(HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return method, nil
	}))
	resChained, _ := chained.Handle(ctx, "base", nil)
	if resChained != "base_mw1_mw2" {
		t.Errorf("unexpected chain result: %v", resChained)
	}

	// TraceMiddleware with io.Writer
	var traceBuf bytes.Buffer
	tmWriter := TraceMiddleware(&traceBuf)
	hWriter := tmWriter(HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		if method == "error" {
			return nil, errors.New("trace error")
		}
		if method == "sentinel" {
			return nil, &NotificationSentinel{}
		}
		return "ok", nil
	}))

	// 1. Success with request ID
	paramsWithID, _ := json.Marshal(map[string]any{"id": "req-1", "data": "val"})
	_, _ = hWriter.Handle(ctx, "ping", paramsWithID)

	// 2. Notification (no request ID)
	_, _ = hWriter.Handle(ctx, "notify", []byte(`{}`))

	// 3. Sentinel error
	_, _ = hWriter.Handle(ctx, "sentinel", paramsWithID)

	// 4. Regular error
	_, _ = hWriter.Handle(ctx, "error", paramsWithID)

	// TraceMiddleware with TraceWriterFunc
	twFunc := TraceWriterFunc(func() io.Writer { return &traceBuf })
	tmFunc := TraceMiddleware(twFunc)
	hFunc := tmFunc(HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "ok", nil
	}))
	_, _ = hFunc.Handle(ctx, "test", paramsWithID)

	// TraceMiddleware with func() io.Writer
	plainFunc := func() io.Writer { return &traceBuf }
	tmPlainFunc := TraceMiddleware(plainFunc)
	hPlain := tmPlainFunc(HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "ok", nil
	}))
	_, _ = hPlain.Handle(ctx, "test", paramsWithID)

	// TraceMiddleware with invalid type (defaults to nil writer)
	tmInvalid := TraceMiddleware(12345)
	hInv := tmInvalid(HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "ok", nil
	}))
	_, _ = hInv.Handle(ctx, "test", paramsWithID)
}

// 2. SpecAccessControl setters and helpers
func TestDeep8_SpecAccessControl_Setters(t *testing.T) {
	sac := NewSpecAccessControl(&mockSpecLoaderDeep2{})
	sac.SetLogger(nil)
	sac.SetEventEmitter(nil)
	sac.SetMCPServerContext(nil)
	sac.SetPermissionCache(nil)

	// getObjID helper
	if id := getObjID(map[string]any{objects.FieldKeyID: "obj-123"}); id != "obj-123" {
		t.Errorf("expected obj-123, got %s", id)
	}
	if id := getObjID(map[string]any{}); id != "" {
		t.Errorf("expected empty string, got %s", id)
	}
	if id := getObjID(nil); id != "" {
		t.Errorf("expected empty string for nil, got %s", id)
	}

	// HasFieldAccess nil secCtx
	if sac.HasFieldAccess("field", map[string]any{}, nil, "read") {
		t.Error("expected false for nil secCtx")
	}

	// HasFieldAccess with secCtx
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"developer"}, []string{"read:*"})
	_ = sac.HasFieldAccess("title", map[string]any{objects.FieldKeyKind: "backlog_item"}, secCtx, "read")
}

// 3. ServerEvents handlers comprehensive
func TestDeep8_ServerHandlersEvents_Comprehensive(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	// NotificationSentinel Error()
	sentinel := &NotificationSentinel{}
	if sentinel.Error() != "notification processed (no response)" {
		t.Errorf("unexpected sentinel error message: %s", sentinel.Error())
	}

	// handleEventsSubscribe
	// 1. Invalid JSON
	_, err := s.handleEventsSubscribe(ctx, "events/subscribe", []byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid json in subscribe")
	}

	// 2. Valid subscribe
	subRes, err := s.handleEventsSubscribe(ctx, "events/subscribe", []byte(`{"eventTypes":["log_info"],"clientId":"c1"}`))
	if err != nil {
		t.Errorf("handleEventsSubscribe failed: %v", err)
	}
	subMap, ok := subRes.(EventsSubscribeResult)
	if !ok || subMap.SubscriptionID == "" {
		t.Errorf("unexpected subscribe result: %v", subRes)
	}

	// handleEventsUnsubscribe
	// 1. Invalid JSON
	_, err = s.handleEventsUnsubscribe(ctx, "events/unsubscribe", []byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid json in unsubscribe")
	}

	// 2. Valid unsubscribe
	unsubRes, err := s.handleEventsUnsubscribe(ctx, "events/unsubscribe", []byte(`{"subscriptionId":"sub-test"}`))
	if err != nil {
		t.Errorf("handleEventsUnsubscribe failed: %v", err)
	}
	if unsubRes == nil {
		t.Error("expected non-nil unsubscribe result")
	}

	// handleEventsList
	listRes, err := s.handleEventsList(ctx, "events/list", nil)
	if err != nil {
		t.Errorf("handleEventsList failed: %v", err)
	}
	if listRes == nil {
		t.Error("expected non-nil events list result")
	}

	// handleNotificationEvent
	// 1. Valid event
	_, err1 := s.handleNotificationEvent(ctx, "notifications/event", []byte(`{"type":"log_info","message":"hello"}`))
	if _, ok := err1.(*NotificationSentinel); !ok {
		t.Errorf("expected *NotificationSentinel, got %v", err1)
	}

	// 2. Wrapped event
	_, _ = s.handleNotificationEvent(ctx, "notifications/event", []byte(`{"event":{"type":"log_info","message":"hello"}}`))

	// 3. Invalid JSON
	_, _ = s.handleNotificationEvent(ctx, "notifications/event", []byte(`{invalid`))

	// 4. Empty params
	_, _ = s.handleNotificationEvent(ctx, "notifications/event", nil)
}
