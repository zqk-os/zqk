// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// cycle234MockLogger provides a thread-safe logger implementation for verifying telemetry.
type cycle234MockLogger struct {
	mu     sync.Mutex
	debugs []string
	infos  []string
	warns  []string
	errors []string
}

func (l *cycle234MockLogger) Debug(msg string, fields ...logging.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugs = append(l.debugs, msg)
}

func (l *cycle234MockLogger) Info(msg string, fields ...logging.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, msg)
}

func (l *cycle234MockLogger) Warn(msg string, fields ...logging.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func (l *cycle234MockLogger) Error(msg string, err error, fields ...logging.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, fmt.Sprintf("%s: %v", msg, err))
}

func (l *cycle234MockLogger) Fatal(msg string, err error, fields ...logging.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, fmt.Sprintf("FATAL: %s: %v", msg, err))
}

func (l *cycle234MockLogger) WithFields(...logging.Field) logging.Logger {
	return l
}

func (l *cycle234MockLogger) WithContext(context.Context) logging.Logger {
	return l
}

func (l *cycle234MockLogger) WithObjectRef(string, string) logging.Logger {
	return l
}

func (l *cycle234MockLogger) counts() (int, int, int, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.debugs), len(l.infos), len(l.warns), len(l.errors)
}

// TestCycle234_HTTPHandler_HandlerErrorWithCustomOnError exercises the error branch in handler.go.
func TestCycle234_HTTPHandler_HandlerErrorWithCustomOnError(t *testing.T) {
	logger := &cycle234MockLogger{}
	var capturedErr error
	customOnErrorCalled := false

	opts := Options{
		Logger:      logger,
		HandlerName: "CustomErrorHandler",
		OnError: func(w http.ResponseWriter, r *http.Request, err error) {
			customOnErrorCalled = true
			capturedErr = err
			http.Error(w, "custom-error-response", http.StatusBadGateway)
		},
	}

	handlerErr := errors.New("upstream service failure")
	h := NewHTTPHandler(opts, func(req Request[map[string]any]) (Response[map[string]any], error) {
		return Response[map[string]any]{}, handlerErr
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()

	h(rec, req)

	if !customOnErrorCalled {
		t.Fatal("expected custom OnError handler to be called")
	}
	if !errors.Is(capturedErr, handlerErr) {
		t.Fatalf("expected captured error %v, got %v", handlerErr, capturedErr)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status code %d, got %d", http.StatusBadGateway, rec.Code)
	}

	_, _, _, errCount := logger.counts()
	if errCount != 1 {
		t.Fatalf("expected 1 logged error, got %d", errCount)
	}
}

// TestCycle234_HTTPHandler_HandlerErrorWithoutLogger exercises handler error with nil logger.
func TestCycle234_HTTPHandler_HandlerErrorWithoutLogger(t *testing.T) {
	customOnErrorCalled := false
	opts := Options{
		Logger:      nil,
		HandlerName: "NoLoggerErrorHandler",
		OnError: func(w http.ResponseWriter, r *http.Request, err error) {
			customOnErrorCalled = true
			http.Error(w, "handled-without-logger", http.StatusServiceUnavailable)
		},
	}

	h := NewHTTPHandler(opts, func(req Request[map[string]any]) (Response[map[string]any], error) {
		return Response[map[string]any]{}, errors.New("unlogged failure")
	})

	req := httptest.NewRequest(http.MethodGet, "/service", nil)
	rec := httptest.NewRecorder()

	h(rec, req)

	if !customOnErrorCalled {
		t.Fatal("expected custom OnError to be called without logger")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status code %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

// TestCycle234_HTTPHandler_DecodeErrorWithoutLogger exercises decode error with nil logger.
func TestCycle234_HTTPHandler_DecodeErrorWithoutLogger(t *testing.T) {
	opts := Options{
		Logger:      nil,
		HandlerName: "DecodeNoLogger",
		OnError:     nil,
	}

	h := NewHTTPHandler(opts, func(req Request[map[string]any]) (Response[map[string]any], error) {
		return Response[map[string]any]{StatusCode: http.StatusOK}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/data", bytes.NewBufferString("{invalid-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d for invalid json, got %d", http.StatusBadRequest, rec.Code)
	}
}

// TestCycle234_HTTPHandler_EmptyBodyWithContentLength exercises io.EOF handling in decoder.
func TestCycle234_HTTPHandler_EmptyBodyWithContentLength(t *testing.T) {
	logger := &cycle234MockLogger{}
	opts := Options{
		Logger:      logger,
		HandlerName: "EmptyBodyHandler",
	}

	type Payload struct {
		Name string `json:"name"`
	}

	h := NewHTTPHandler(opts, func(req Request[Payload]) (Response[map[string]string], error) {
		return Response[map[string]string]{
			StatusCode: http.StatusOK,
			Body: map[string]string{
				"received_name": req.Payload.Name,
			},
		}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/empty", bytes.NewReader([]byte{}))
	req.ContentLength = 0
	rec := httptest.NewRecorder()

	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	debugCount, _, warnCount, errCount := logger.counts()
	if debugCount != 1 || warnCount != 0 || errCount != 0 {
		t.Fatalf("unexpected log counts: debug=%d warn=%d err=%d", debugCount, warnCount, errCount)
	}
}

// TestCycle234_HTTPHandler_ResponseHeadersAndComplexBody verifies header serialization and body encoding.
func TestCycle234_HTTPHandler_ResponseHeadersAndComplexBody(t *testing.T) {
	type ComplexBody struct {
		ID       int      `json:"id"`
		Active   bool     `json:"active"`
		Tags     []string `json:"tags"`
		Metadata any      `json:"metadata"`
	}

	h := NewHTTPHandler(Options{HandlerName: "ComplexHandler"}, func(req Request[ComplexBody]) (Response[ComplexBody], error) {
		return Response[ComplexBody]{
			StatusCode: http.StatusCreated,
			Body: ComplexBody{
				ID:       req.Payload.ID * 2,
				Active:   !req.Payload.Active,
				Tags:     append(req.Payload.Tags, "processed"),
				Metadata: map[string]any{"source": "transport-test"},
			},
			Headers: map[string]string{
				"X-Custom-Header": "cycle-234-test",
				"X-Trace-ID":      "trace-9999",
			},
		}, nil
	})

	input := ComplexBody{
		ID:     42,
		Active: true,
		Tags:   []string{"alpha", "beta"},
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal test input: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/transform", bytes.NewReader(encoded))
	rec := httptest.NewRecorder()

	h(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom-Header") != "cycle-234-test" {
		t.Fatalf("unexpected custom header: %s", rec.Header().Get("X-Custom-Header"))
	}
	if rec.Header().Get("X-Trace-ID") != "trace-9999" {
		t.Fatalf("unexpected trace header: %s", rec.Header().Get("X-Trace-ID"))
	}

	var output ComplexBody
	if err := json.NewDecoder(rec.Body).Decode(&output); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if output.ID != 84 || output.Active || len(output.Tags) != 3 {
		t.Fatalf("unexpected output: %+v", output)
	}
}

// TestCycle234_FTPHandler_DefaultErrorHandling verifies 500 status fallback when OnError and Logger are nil.
func TestCycle234_FTPHandler_DefaultErrorHandling(t *testing.T) {
	opts := FTPOptions{
		HandlerName: "SilentFTPHandler",
		Logger:      nil,
		OnError:     nil,
	}

	expectedErr := errors.New("disk drive unmounted")
	h := NewFTPHandler(opts, func(req FTPRequest[string]) (FTPResponse[string], error) {
		return FTPResponse[string]{}, expectedErr
	})

	req := FTPRequest[string]{
		Context: context.Background(),
		Command: "STOR",
		Payload: "important_document.bin",
	}

	res, err := h(req)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if res.Code != 500 {
		t.Fatalf("expected fallback code 500, got %d", res.Code)
	}
	if res.Message != "Internal Server Error" {
		t.Fatalf("expected 'Internal Server Error', got %q", res.Message)
	}
}

// TestCycle234_FTPHandler_CustomOnErrorWithLogger verifies error formatting and logging when OnError is set.
func TestCycle234_FTPHandler_CustomOnErrorWithLogger(t *testing.T) {
	logger := &cycle234MockLogger{}
	opts := FTPOptions{
		HandlerName: "AuthFTPHandler",
		Logger:      logger,
		OnError: func(err error) (int, string) {
			return 530, "Not logged in, user credentials rejected"
		},
	}

	h := NewFTPHandler(opts, func(req FTPRequest[string]) (FTPResponse[string], error) {
		return FTPResponse[string]{}, errors.New("invalid password")
	})

	req := FTPRequest[string]{
		Context: context.Background(),
		Command: "PASS",
		Payload: "secret123",
	}

	res, err := h(req)
	if err == nil {
		t.Fatal("expected error from handler")
	}
	if res.Code != 530 {
		t.Fatalf("expected code 530, got %d", res.Code)
	}
	if res.Message != "Not logged in, user credentials rejected" {
		t.Fatalf("unexpected message: %s", res.Message)
	}

	_, _, _, errCount := logger.counts()
	if errCount != 1 {
		t.Fatalf("expected 1 logged error, got %d", errCount)
	}
}

// TestCycle234_FTPHandler_VerbsAndStreamingTransfer tests standard FTP commands and simulated file streaming.
func TestCycle234_FTPHandler_VerbsAndStreamingTransfer(t *testing.T) {
	commands := []string{"USER", "PASS", "PWD", "CWD", "PASV", "TYPE", "LIST", "QUIT", "NOOP"}

	for _, cmd := range commands {
		h := NewFTPHandler(FTPOptions{HandlerName: "CmdFTP"}, func(req FTPRequest[string]) (FTPResponse[string], error) {
			return FTPResponse[string]{
				Code:    200,
				Message: fmt.Sprintf("Command %s succeeded", req.Command),
				Body:    req.Payload + "_ack",
			}, nil
		})

		res, err := h(FTPRequest[string]{
			Context: context.Background(),
			Command: cmd,
			Payload: "arg",
		})
		if err != nil {
			t.Fatalf("command %s failed: %v", cmd, err)
		}
		if res.Code != 200 {
			t.Fatalf("expected 200 for %s, got %d", cmd, res.Code)
		}
	}

	fileContent := []byte("cycle234-stream-data-payload-content")
	srcReader := bytes.NewReader(fileContent)
	dstBuffer := &bytes.Buffer{}

	var discardBuffer bytes.Buffer
	type streamContext struct {
		Writer io.Writer
	}

	streamHandlerTyped := NewFTPHandler(FTPOptions{HandlerName: "StreamFTPTyped"}, func(req FTPRequest[streamContext]) (FTPResponse[string], error) {
		copied, copyErr := io.Copy(req.Payload.Writer, req.Data)
		if copyErr != nil {
			return FTPResponse[string]{}, copyErr
		}
		return FTPResponse[string]{
			Code:    226,
			Message: fmt.Sprintf("Transfer complete: %d bytes", copied),
			Body:    "OK",
			Data:    &discardBuffer,
		}, nil
	})

	streamRes, streamErr := streamHandlerTyped(FTPRequest[streamContext]{
		Context: context.Background(),
		Command: "STOR",
		Payload: streamContext{Writer: dstBuffer},
		Data:    srcReader,
	})
	if streamErr != nil {
		t.Fatalf("streaming handler failed: %v", streamErr)
	}
	if streamRes.Code != 226 {
		t.Fatalf("expected code 226, got %d", streamRes.Code)
	}
	if dstBuffer.String() != string(fileContent) {
		t.Fatalf("content mismatch: got %q, expected %q", dstBuffer.String(), string(fileContent))
	}
}

// TestCycle234_RPCHandler_AllErrorAndSuccessCombinations tests RPCHandler option permutations.
func TestCycle234_RPCHandler_AllErrorAndSuccessCombinations(t *testing.T) {
	logger := &cycle234MockLogger{}

	// Case 1: Success with nil logger
	hSuccessNoLogger := NewRPCHandler(RPCOptions{HandlerName: "NoLoggerSuccess"}, func(ctx context.Context, req int) (int, error) {
		return req * 3, nil
	})
	val, err := hSuccessNoLogger(context.Background(), 10)
	if err != nil || val != 30 {
		t.Fatalf("expected 30, got %d, err=%v", val, err)
	}

	// Case 2: Error with nil logger and nil OnError
	hErrNoLogger := NewRPCHandler(RPCOptions{HandlerName: "NoLoggerErr"}, func(ctx context.Context, req int) (int, error) {
		return 0, errors.New("raw error")
	})
	_, err = hErrNoLogger(context.Background(), 10)
	if err == nil || err.Error() != "raw error" {
		t.Fatalf("expected 'raw error', got %v", err)
	}

	// Case 3: Error with custom OnError and logger
	customMappedErr := errors.New("mapped custom error")
	hCustomErr := NewRPCHandler(RPCOptions{
		Logger:      logger,
		HandlerName: "MappedErrRPC",
		OnError: func(err error) error {
			return customMappedErr
		},
	}, func(ctx context.Context, req int) (int, error) {
		return 0, errors.New("original underlying error")
	})

	_, err = hCustomErr(context.Background(), 5)
	if !errors.Is(err, customMappedErr) {
		t.Fatalf("expected %v, got %v", customMappedErr, err)
	}

	debugCount, _, _, errCount := logger.counts()
	if errCount != 1 {
		t.Fatalf("expected 1 logged error, got %d", errCount)
	}
	if debugCount != 0 {
		t.Fatalf("expected 0 debug logs, got %d", debugCount)
	}
}

// TestCycle234_RPCHandler_ContextDeadlineAndCancellation tests context handling in RPCHandler.
func TestCycle234_RPCHandler_ContextDeadlineAndCancellation(t *testing.T) {
	h := NewRPCHandler(RPCOptions{HandlerName: "TimeoutRPC"}, func(ctx context.Context, timeout time.Duration) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(timeout):
			return "completed", nil
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := h(ctx, 100*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	normalCtx := context.Background()
	res, err := h(normalCtx, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error on normal execution: %v", err)
	}
	if res != "completed" {
		t.Fatalf("expected 'completed', got %q", res)
	}
}

// protocolMultiplexer routes incoming requests to the appropriate protocol handler.
type protocolMultiplexer struct {
	mu           sync.RWMutex
	httpHandlers map[string]http.HandlerFunc
	ftpHandlers  map[string]FTPHandler[string, string]
	rpcHandlers  map[string]RPCHandler[string, string]
}

func newProtocolMultiplexer() *protocolMultiplexer {
	return &protocolMultiplexer{
		httpHandlers: make(map[string]http.HandlerFunc),
		ftpHandlers:  make(map[string]FTPHandler[string, string]),
		rpcHandlers:  make(map[string]RPCHandler[string, string]),
	}
}

func (m *protocolMultiplexer) registerHTTP(pattern string, h http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpHandlers[pattern] = h
}

func (m *protocolMultiplexer) registerFTP(command string, h FTPHandler[string, string]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ftpHandlers[command] = h
}

func (m *protocolMultiplexer) registerRPC(method string, h RPCHandler[string, string]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rpcHandlers[method] = h
}

func (m *protocolMultiplexer) dispatch(protocol, route, payload string) (string, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch protocol {
	case "http":
		h, ok := m.httpHandlers[route]
		if !ok {
			return "", http.StatusNotFound, errors.New("route not found")
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, route, bytes.NewBufferString(fmt.Sprintf("%q", payload)))
		req.Header.Set("Content-Type", "application/json")
		h(rec, req)
		return rec.Body.String(), rec.Code, nil

	case "ftp":
		h, ok := m.ftpHandlers[route]
		if !ok {
			return "", 502, errors.New("unsupported command")
		}
		res, err := h(FTPRequest[string]{Context: context.Background(), Command: route, Payload: payload})
		return res.Message, res.Code, err

	case "rpc":
		h, ok := m.rpcHandlers[route]
		if !ok {
			return "", 404, errors.New("method not found")
		}
		res, err := h(context.Background(), payload)
		return res, 200, err

	default:
		return "", 0, fmt.Errorf("unknown protocol: %s", protocol)
	}
}

// TestCycle234_ProtocolMultiplexingAndRouting verifies multi-protocol dispatch through a unified router.
func TestCycle234_ProtocolMultiplexingAndRouting(t *testing.T) {
	mux := newProtocolMultiplexer()

	mux.registerHTTP("/data", NewHTTPHandler(Options{HandlerName: "MuxHTTP"}, func(req Request[string]) (Response[string], error) {
		return Response[string]{StatusCode: http.StatusOK, Body: "http-ack:" + req.Payload}, nil
	}))

	mux.registerFTP("STAT", NewFTPHandler(FTPOptions{HandlerName: "MuxFTP"}, func(req FTPRequest[string]) (FTPResponse[string], error) {
		return FTPResponse[string]{Code: 211, Message: "FTP server status ok"}, nil
	}))

	mux.registerRPC("ping", NewRPCHandler(RPCOptions{HandlerName: "MuxRPC"}, func(ctx context.Context, req string) (string, error) {
		return "pong:" + req, nil
	}))

	// HTTP dispatch
	body, code, err := mux.dispatch("http", "/data", "test-payload")
	if err != nil || code != http.StatusOK {
		t.Fatalf("http dispatch failed: code=%d err=%v body=%s", code, err, body)
	}

	// FTP dispatch
	msg, ftpCode, err := mux.dispatch("ftp", "STAT", "arg")
	if err != nil || ftpCode != 211 {
		t.Fatalf("ftp dispatch failed: code=%d err=%v msg=%s", ftpCode, err, msg)
	}

	// RPC dispatch
	rpcRes, rpcCode, err := mux.dispatch("rpc", "ping", "hello")
	if err != nil || rpcCode != 200 || rpcRes != "pong:hello" {
		t.Fatalf("rpc dispatch failed: res=%s code=%d err=%v", rpcRes, rpcCode, err)
	}

	// Unknown protocol
	_, _, err = mux.dispatch("websocket", "/stream", "")
	if err == nil {
		t.Fatal("expected error for unknown protocol")
	}

	// Missing route
	_, missingCode, err := mux.dispatch("http", "/missing", "")
	if err == nil || missingCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing route, got %d, err=%v", missingCode, err)
	}
}

// connectionState models connection lifecycle phases.
type connectionState int32

const (
	stateInit connectionState = iota
	stateActive
	stateDraining
	stateClosed
)

type connectionLifecycleManager struct {
	state         int32
	activeConns   int64
	totalHandled  int64
	maxConcurrent int64
	drainedSignal chan struct{}
	drainOnce     sync.Once
}

func newLifecycleManager() *connectionLifecycleManager {
	return &connectionLifecycleManager{
		state:         int32(stateInit),
		drainedSignal: make(chan struct{}),
	}
}

func (m *connectionLifecycleManager) acquire() bool {
	currentState := connectionState(atomic.LoadInt32(&m.state))
	if currentState != stateInit && currentState != stateActive {
		return false
	}
	atomic.StoreInt32(&m.state, int32(stateActive))
	atomic.AddInt64(&m.activeConns, 1)
	atomic.AddInt64(&m.totalHandled, 1)
	return true
}

func (m *connectionLifecycleManager) release() {
	curr := atomic.AddInt64(&m.activeConns, -1)
	if curr == 0 && connectionState(atomic.LoadInt32(&m.state)) == stateDraining {
		m.drainOnce.Do(func() {
			close(m.drainedSignal)
		})
	}
}

func (m *connectionLifecycleManager) drain() {
	atomic.StoreInt32(&m.state, int32(stateDraining))
	if atomic.LoadInt64(&m.activeConns) == 0 {
		m.drainOnce.Do(func() {
			close(m.drainedSignal)
		})
	}
}

func (m *connectionLifecycleManager) close() {
	atomic.StoreInt32(&m.state, int32(stateClosed))
}

// TestCycle234_ConnectionLifecycle_Transitions tests connection lifecycle states and draining.
func TestCycle234_ConnectionLifecycle_Transitions(t *testing.T) {
	mgr := newLifecycleManager()

	if !mgr.acquire() {
		t.Fatal("failed to acquire initial connection")
	}
	if !mgr.acquire() {
		t.Fatal("failed to acquire second connection")
	}

	if atomic.LoadInt64(&mgr.activeConns) != 2 {
		t.Fatalf("expected 2 active connections, got %d", atomic.LoadInt64(&mgr.activeConns))
	}

	mgr.drain()

	// New acquires must be rejected during draining
	if mgr.acquire() {
		t.Fatal("acquire should fail when manager is draining")
	}

	mgr.release()
	select {
	case <-mgr.drainedSignal:
		t.Fatal("drain signal should not fire while 1 connection remains")
	default:
	}

	mgr.release()

	select {
	case <-mgr.drainedSignal:
		// success: all connections drained
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for drain signal")
	}

	mgr.close()
	if mgr.acquire() {
		t.Fatal("acquire should fail when manager is closed")
	}
}

// TestCycle234_Concurrency_ManagedMultiTransport exercises concurrent dispatch using goroutinelabels.
func TestCycle234_Concurrency_ManagedMultiTransport(t *testing.T) {
	const numWorkers = 16
	const iterationsPerWorker = 20

	httpHandler := NewHTTPHandler(Options{HandlerName: "ConcurrentHTTP"}, func(req Request[int]) (Response[int], error) {
		return Response[int]{StatusCode: http.StatusOK, Body: req.Payload * 2}, nil
	})

	ftpHandler := NewFTPHandler(FTPOptions{HandlerName: "ConcurrentFTP"}, func(req FTPRequest[int]) (FTPResponse[int], error) {
		return FTPResponse[int]{Code: 200, Message: "OK", Body: req.Payload + 10}, nil
	})

	rpcHandler := NewRPCHandler(RPCOptions{HandlerName: "ConcurrentRPC"}, func(ctx context.Context, req int) (int, error) {
		return req * req, nil
	})

	var wg sync.WaitGroup
	var successfulOps int64
	var failedOps int64

	for w := 0; w < numWorkers; w++ {
		workerID := w
		wg.Add(1)

		builder := goroutinelabels.NewGoroutine(
			fmt.Sprintf("transport_worker_%d", workerID),
			"concurrent HTTP, FTP and RPC transport test execution",
		)

		builder.StartSimple(func() {
			defer wg.Done()

			for i := 0; i < iterationsPerWorker; i++ {
				val := workerID*100 + i

				// HTTP execution
				req := httptest.NewRequest(http.MethodPost, "/math", bytes.NewBufferString(fmt.Sprintf("%d", val)))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				httpHandler(rec, req)
				if rec.Code == http.StatusOK {
					atomic.AddInt64(&successfulOps, 1)
				} else {
					atomic.AddInt64(&failedOps, 1)
				}

				// FTP execution
				ftpRes, ftpErr := ftpHandler(FTPRequest[int]{
					Context: context.Background(),
					Command: "EVAL",
					Payload: val,
				})
				if ftpErr == nil && ftpRes.Code == 200 {
					atomic.AddInt64(&successfulOps, 1)
				} else {
					atomic.AddInt64(&failedOps, 1)
				}

				// RPC execution
				rpcRes, rpcErr := rpcHandler(context.Background(), val)
				if rpcErr == nil && rpcRes == val*val {
					atomic.AddInt64(&successfulOps, 1)
				} else {
					atomic.AddInt64(&failedOps, 1)
				}
			}
		})
	}

	wg.Wait()

	expectedTotal := int64(numWorkers * iterationsPerWorker * 3)
	if atomic.LoadInt64(&successfulOps) != expectedTotal {
		t.Fatalf("expected %d successful ops, got %d (failed: %d)", expectedTotal, successfulOps, failedOps)
	}
	if atomic.LoadInt64(&failedOps) != 0 {
		t.Fatalf("expected 0 failures, got %d", failedOps)
	}
}

// TestCycle234_HermeticFileTransferSimulation tests file operations via fileutil with t.TempDir().
func TestCycle234_HermeticFileTransferSimulation(t *testing.T) {
	tempDir := t.TempDir()
	sourceFilePath := filepath.Join(tempDir, "source_payload.bin")
	destFilePath := filepath.Join(tempDir, "dest_payload.bin")

	testData := []byte("hermetic-file-transfer-simulation-content-cycle-234")
	if err := fileutil.WriteFile(sourceFilePath, testData, 0600); err != nil {
		t.Fatalf("failed to write source test file: %v", err)
	}

	fileBytes, err := fileutil.ReadFile(sourceFilePath)
	if err != nil {
		t.Fatalf("failed to read source test file: %v", err)
	}

	h := NewFTPHandler(FTPOptions{HandlerName: "HermeticFTP"}, func(req FTPRequest[string]) (FTPResponse[string], error) {
		readData, readErr := io.ReadAll(req.Data)
		if readErr != nil {
			return FTPResponse[string]{}, readErr
		}
		if writeErr := fileutil.WriteFile(req.Payload, readData, 0600); writeErr != nil {
			return FTPResponse[string]{}, writeErr
		}
		return FTPResponse[string]{
			Code:    226,
			Message: "File transfer completed successfully",
			Body:    req.Payload,
		}, nil
	})

	res, err := h(FTPRequest[string]{
		Context: context.Background(),
		Command: "STOR",
		Payload: destFilePath,
		Data:    bytes.NewReader(fileBytes),
	})
	if err != nil {
		t.Fatalf("file transfer handler failed: %v", err)
	}
	if res.Code != 226 {
		t.Fatalf("expected FTP 226 code, got %d", res.Code)
	}

	copiedData, err := fileutil.ReadFile(destFilePath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if !bytes.Equal(copiedData, testData) {
		t.Fatalf("transferred content does not match original")
	}
}
