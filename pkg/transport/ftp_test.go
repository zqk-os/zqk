package transport

import (
	"context"
	"errors"
	"testing"
)

func TestNewFTPHandler_Success(t *testing.T) {
	type ReqType struct {
		Path string
	}
	type ResType struct {
		Status string
	}

	opts := FTPOptions{
		HandlerName: "TestFTP",
	}

	handle := func(req FTPRequest[ReqType]) (FTPResponse[ResType], error) {
		return FTPResponse[ResType]{
			Code:    200,
			Message: "Command okay",
			Body:    ResType{Status: req.Payload.Path + "_ok"},
		}, nil
	}

	h := NewFTPHandler(opts, handle)

	req := FTPRequest[ReqType]{
		Context: context.Background(),
		Command: "RETR",
		Payload: ReqType{Path: "/test.txt"},
	}

	res, err := h(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 200 {
		t.Errorf("expected 200 code, got %d", res.Code)
	}
	if res.Body.Status != "/test.txt_ok" {
		t.Errorf("unexpected status: %s", res.Body.Status)
	}
}

func TestNewFTPHandler_Error(t *testing.T) {
	opts := FTPOptions{
		HandlerName: "TestFTP",
		OnError: func(err error) (int, string) {
			return 550, "Requested action not taken"
		},
	}

	handle := func(req FTPRequest[string]) (FTPResponse[string], error) {
		return FTPResponse[string]{}, errors.New("file not found")
	}

	h := NewFTPHandler(opts, handle)

	req := FTPRequest[string]{
		Context: context.Background(),
		Command: "RETR",
	}

	res, err := h(req)
	if err == nil {
		t.Fatalf("expected error")
	}
	if res.Code != 550 {
		t.Errorf("expected 550 code, got %d", res.Code)
	}
	if res.Message != "Requested action not taken" {
		t.Errorf("unexpected message: %s", res.Message)
	}
}
