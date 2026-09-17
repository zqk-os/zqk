package scheduler

import (
	"context"
	"testing"
)

func TestRunCallbackListenerViaPipeline_NilHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-test", JobType: JobTypeCallbackListener}
	err := RunCallbackListenerViaPipeline(ctx, nil, job)
	if err == nil {
		t.Fatal("expected error when handler is nil")
	}
}

func TestRunCallbackListenerViaPipeline_NilJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	handler := &CallbackListenerHandler{logger: nil}
	err := RunCallbackListenerViaPipeline(ctx, handler, nil)
	if err == nil {
		t.Fatal("expected error when job is nil")
	}
}

func TestCallbackListenerHandler_Timeouts(t *testing.T) {
	t.Parallel()
	handler := &CallbackListenerHandler{}
	srv := handler.BuildHTTPServer("127.0.0.1:8080", nil)
	if srv == nil {
		t.Fatal("expected non-nil http.Server")
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Errorf("expected ReadHeaderTimeout > 0, got %v", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout <= 0 {
		t.Errorf("expected ReadTimeout > 0, got %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout <= 0 {
		t.Errorf("expected WriteTimeout > 0, got %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Errorf("expected IdleTimeout > 0, got %v", srv.IdleTimeout)
	}
}
