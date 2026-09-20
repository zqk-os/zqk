package transport

import (
	"context"
	"errors"
	"testing"
)

func TestNewRPCHandler_Success(t *testing.T) {
	type ReqType struct {
		Data string
	}
	type ResType struct {
		Result string
	}

	opts := RPCOptions{
		HandlerName: "TestRPC",
	}

	handle := func(ctx context.Context, req ReqType) (ResType, error) {
		return ResType{Result: req.Data + "_processed"}, nil
	}

	h := NewRPCHandler(opts, handle)
	res, err := h(context.Background(), ReqType{Data: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Result != "hello_processed" {
		t.Errorf("unexpected result: %s", res.Result)
	}
}

func TestNewRPCHandler_Error(t *testing.T) {
	opts := RPCOptions{
		HandlerName: "TestRPC",
		OnError: func(err error) error {
			return errors.New("wrapped: " + err.Error())
		},
	}

	handle := func(ctx context.Context, req int) (int, error) {
		return 0, errors.New("original error")
	}

	h := NewRPCHandler(opts, handle)
	_, err := h(context.Background(), 123)
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "wrapped: original error" {
		t.Errorf("unexpected error message: %v", err)
	}
}
