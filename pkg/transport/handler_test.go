package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPHandler_Success(t *testing.T) {
	type ReqType struct {
		Data string `json:"data"`
	}
	type ResType struct {
		Result string `json:"result"`
	}

	opts := Options{
		HandlerName: "TestHandler",
	}

	handle := func(req Request[ReqType]) (Response[ResType], error) {
		return Response[ResType]{
			StatusCode: 200,
			Body: ResType{
				Result: req.Payload.Data + "_processed",
			},
			Headers: map[string]string{"X-Custom": "Value"},
		}, nil
	}

	h := NewHTTPHandler(opts, handle)

	reqBody := `{"data":"hello"}`
	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if w.Header().Get("X-Custom") != "Value" {
		t.Errorf("expected header X-Custom=Value")
	}

	var resBody ResType
	if err := json.NewDecoder(w.Body).Decode(&resBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resBody.Result != "hello_processed" {
		t.Errorf("unexpected result: %s", resBody.Result)
	}
}

func TestNewHTTPHandler_DecodeError(t *testing.T) {
	type ReqType struct {
		Data string `json:"data"`
	}
	type ResType struct {
		Result string `json:"result"`
	}

	opts := Options{
		HandlerName: "TestHandler",
	}

	handle := func(req Request[ReqType]) (Response[ResType], error) {
		return Response[ResType]{StatusCode: 200}, nil
	}

	h := NewHTTPHandler(opts, handle)

	// Invalid JSON
	reqBody := `{"data":`
	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestNewHTTPHandler_HandlerError(t *testing.T) {
	type ReqType struct{}
	type ResType struct{}

	opts := Options{
		HandlerName: "TestHandler",
	}

	handle := func(req Request[ReqType]) (Response[ResType], error) {
		return Response[ResType]{}, errors.New("internal handler error")
	}

	h := NewHTTPHandler(opts, handle)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	h(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
