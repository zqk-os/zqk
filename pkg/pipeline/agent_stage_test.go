package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/pipeline"
)

type mockDispatcher struct {
	dispatchFunc func(ctx context.Context, payload any) (any, error)
}

func (m *mockDispatcher) Dispatch(ctx context.Context, payload any) (any, error) {
	if m.dispatchFunc != nil {
		return m.dispatchFunc(ctx, payload)
	}
	return payload, nil
}

func TestAgentStage(t *testing.T) {
	ctx := &pipeline.Context{
		Ctx: context.Background(),
	}

	t.Run("RequiresDispatcher", func(t *testing.T) {
		stage := pipeline.AgentStage(pipeline.AgentStageOptions{})
		_, err := stage(ctx, "payload")
		if err == nil || err.Error() != "AgentStage requires a valid Dispatcher" {
			t.Errorf("Expected 'AgentStage requires a valid Dispatcher' error, got %v", err)
		}
	})

	t.Run("SuccessfulDispatch", func(t *testing.T) {
		dispatcher := &mockDispatcher{
			dispatchFunc: func(ctx context.Context, payload any) (any, error) {
				return "dispatched: " + payload.(string), nil
			},
		}

		stage := pipeline.AgentStage(pipeline.AgentStageOptions{
			Dispatcher: dispatcher,
		})

		result, err := stage(ctx, "test_payload")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result != "dispatched: test_payload" {
			t.Errorf("Unexpected result: %v", result)
		}
	})

	t.Run("WithTaskBuilder", func(t *testing.T) {
		dispatcher := &mockDispatcher{
			dispatchFunc: func(ctx context.Context, payload any) (any, error) {
				return payload, nil
			},
		}

		stage := pipeline.AgentStage(pipeline.AgentStageOptions{
			Dispatcher: dispatcher,
			TaskBuilder: func(pctx *pipeline.Context, payload any) (any, error) {
				return "built: " + payload.(string), nil
			},
		})

		result, err := stage(ctx, "test_payload")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result != "built: test_payload" {
			t.Errorf("Unexpected result: %v", result)
		}
	})

	t.Run("TaskBuilderError", func(t *testing.T) {
		dispatcher := &mockDispatcher{}
		expectedErr := errors.New("builder error")

		stage := pipeline.AgentStage(pipeline.AgentStageOptions{
			Dispatcher: dispatcher,
			TaskBuilder: func(pctx *pipeline.Context, payload any) (any, error) {
				return nil, expectedErr
			},
		})

		_, err := stage(ctx, "test_payload")
		if err == nil || err.Error() != "AgentStage task building failed: builder error" {
			t.Errorf("Expected task building error, got %v", err)
		}
	})

	t.Run("DispatchError", func(t *testing.T) {
		expectedErr := errors.New("dispatch error")
		dispatcher := &mockDispatcher{
			dispatchFunc: func(ctx context.Context, payload any) (any, error) {
				return nil, expectedErr
			},
		}

		stage := pipeline.AgentStage(pipeline.AgentStageOptions{
			Dispatcher: dispatcher,
		})

		_, err := stage(ctx, "test_payload")
		if err == nil || err.Error() != "AgentStage dispatch failed: dispatch error" {
			t.Errorf("Expected dispatch error, got %v", err)
		}
	})
}
