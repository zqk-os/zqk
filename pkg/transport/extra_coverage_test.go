// BLI-STARTER-COMMUNITY-042 / PRI-STARTER-COMMUNITY-042 coverage elevation
package transport

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
)

type extraLog struct{}

func (extraLog) Debug(string, ...logging.Field)              {}
func (extraLog) Info(string, ...logging.Field)               {}
func (extraLog) Warn(string, ...logging.Field)               {}
func (extraLog) Error(string, error, ...logging.Field)       {}
func (extraLog) Fatal(string, error, ...logging.Field)       {}
func (extraLog) WithFields(...logging.Field) logging.Logger  { return extraLog{} }
func (extraLog) WithContext(context.Context) logging.Logger  { return extraLog{} }
func (extraLog) WithObjectRef(string, string) logging.Logger { return extraLog{} }

func TestExtraHTTPHandlerLoggerAndOnError(t *testing.T) {
	log := extraLog{}
	var saw error
	h := NewHTTPHandler(Options{
		Logger:      log,
		HandlerName: "extra",
		OnError: func(w http.ResponseWriter, r *http.Request, err error) {
			saw = err
			http.Error(w, "custom", http.StatusTeapot)
		},
	}, func(req Request[map[string]any]) (Response[map[string]any], error) {
		return Response[map[string]any]{}, errors.New("handler boom")
	})
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"a":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req)
	if saw == nil || w.Code != http.StatusTeapot {
		t.Fatalf("decode onerror = %v %d", saw, w.Code)
	}

	saw = nil
	okH := NewHTTPHandler(Options{Logger: log, HandlerName: "ok"}, func(req Request[map[string]any]) (Response[map[string]any], error) {
		return Response[map[string]any]{StatusCode: http.StatusCreated, Body: map[string]any{"ok": true}, Headers: map[string]string{"X-A": "1"}}, nil
	})
	req = httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"a":1}`))
	w = httptest.NewRecorder()
	okH(w, req)
	if w.Code != http.StatusCreated || w.Header().Get("X-A") != "1" {
		t.Fatalf("ok logger = %d %s", w.Code, w.Header().Get("X-A"))
	}

	errH := NewHTTPHandler(Options{Logger: log, HandlerName: "err"}, func(req Request[map[string]any]) (Response[map[string]any], error) {
		return Response[map[string]any]{}, errors.New("fail")
	})
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	w = httptest.NewRecorder()
	errH(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("err logger = %d", w.Code)
	}

	rpc := NewRPCHandler(RPCOptions{Logger: log, HandlerName: "rpc"}, func(ctx context.Context, n int) (int, error) {
		return n + 1, nil
	})
	if got, err := rpc(context.Background(), 1); err != nil || got != 2 {
		t.Fatalf("rpc ok = %d %v", got, err)
	}
	rpcErr := NewRPCHandler(RPCOptions{Logger: log, HandlerName: "rpc-err"}, func(ctx context.Context, n int) (int, error) {
		return 0, errors.New("rpc fail")
	})
	if _, err := rpcErr(context.Background(), 1); err == nil {
		t.Fatal("rpc err")
	}

	ftp := NewFTPHandler(FTPOptions{Logger: log, HandlerName: "ftp"}, func(req FTPRequest[string]) (FTPResponse[string], error) {
		return FTPResponse[string]{Code: 200, Message: "ok"}, nil
	})
	if res, err := ftp(FTPRequest[string]{Context: context.Background(), Command: "LIST"}); err != nil || res.Code != 200 {
		t.Fatalf("ftp ok = %#v %v", res, err)
	}
	ftpErr := NewFTPHandler(FTPOptions{Logger: log, HandlerName: "ftp-err"}, func(req FTPRequest[string]) (FTPResponse[string], error) {
		return FTPResponse[string]{}, errors.New("ftp fail")
	})
	res, err := ftpErr(FTPRequest[string]{Context: context.Background(), Command: "LIST"})
	if err == nil || res.Code != 500 {
		t.Fatalf("ftp default err = %#v %v", res, err)
	}
}
