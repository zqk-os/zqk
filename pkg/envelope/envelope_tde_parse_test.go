// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package envelope_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/envelope"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestParseTDEEnvelope_SuccessCases(t *testing.T) {
	testCases := []struct {
		name       string
		rawJSON    string
		wantID     string
		wantKind   string
		wantStatus envelope.Status
		wantAgent  string
		wantIntent string
		wantData   string
	}{
		{
			name:       "minimal required envelope",
			rawJSON:    `{"status": "pending"}`,
			wantStatus: envelope.StatusPending,
		},
		{
			name: "full envelope with all fields",
			rawJSON: `{
				"id": "env-cycle-193",
				"kind": "task_decision",
				"status": "success",
				"agent_id": "agent-worker-1",
				"intent": "verify-invariants",
				"payload": "eyJhY3Rpb24iOiAidGVzdCJ9"
			}`,
			wantID:     "env-cycle-193",
			wantKind:   "task_decision",
			wantStatus: envelope.StatusSuccess,
			wantAgent:  "agent-worker-1",
			wantIntent: "verify-invariants",
			wantData:   "eyJhY3Rpb24iOiAidGVzdCJ9",
		},
		{
			name:       "empty string payload",
			rawJSON:    `{"id": "env-empty-payload", "status": "failure", "payload": ""}`,
			wantID:     "env-empty-payload",
			wantStatus: envelope.StatusFailure,
			wantData:   "",
		},
		{
			name:       "whitespace payload",
			rawJSON:    `{"id": "env-ws-payload", "status": "deferred", "payload": "   "}`,
			wantID:     "env-ws-payload",
			wantStatus: envelope.StatusDeferred,
			wantData:   "   ",
		},
		{
			name:       "escalated status",
			rawJSON:    `{"id": "env-escalated", "status": "escalated"}`,
			wantID:     "env-escalated",
			wantStatus: envelope.StatusEscalated,
		},
		{
			name:       "custom non-empty status",
			rawJSON:    `{"id": "env-custom", "status": "custom_decision_state"}`,
			wantID:     "env-custom",
			wantStatus: envelope.Status("custom_decision_state"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := envelope.ParseTDEEnvelope([]byte(tc.rawJSON))
			require.NoError(t, err)
			require.NotNil(t, env)
			assert.Equal(t, tc.wantID, env.ID)
			assert.Equal(t, tc.wantKind, env.Kind)
			assert.Equal(t, tc.wantStatus, env.Status)
			assert.Equal(t, tc.wantAgent, env.AgentID)
			assert.Equal(t, tc.wantIntent, env.Intent)
			assert.Equal(t, tc.wantData, env.Payload)
		})
	}
}

func TestParseTDEEnvelope_FailureCases(t *testing.T) {
	testCases := []struct {
		name        string
		rawInput    []byte
		errContains string
	}{
		{
			name:        "empty byte slice",
			rawInput:    []byte{},
			errContains: "unexpected end of JSON input",
		},
		{
			name:        "whitespace-only input",
			rawInput:    []byte("   \n\t  "),
			errContains: "unexpected end of JSON input",
		},
		{
			name:        "malformed syntax",
			rawInput:    []byte(`{"id": "env-broken", "status": `),
			errContains: "unexpected end of JSON input",
		},
		{
			name:        "invalid json token",
			rawInput:    []byte(`{not-json}`),
			errContains: "invalid character",
		},
		{
			name:        "array root instead of object",
			rawInput:    []byte(`["pending"]`),
			errContains: "cannot unmarshal array",
		},
		{
			name:        "missing status field",
			rawInput:    []byte(`{"id": "env-missing-status", "agent_id": "agt-1"}`),
			errContains: "missing required field: status",
		},
		{
			name:        "explicit empty status string",
			rawInput:    []byte(`{"id": "env-empty-status", "status": ""}`),
			errContains: "missing required field: status",
		},
		{
			name:        "numeric status type mismatch",
			rawInput:    []byte(`{"id": "env-num-status", "status": 200}`),
			errContains: "cannot unmarshal number",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := envelope.ParseTDEEnvelope(tc.rawInput)
			require.Error(t, err)
			assert.Nil(t, env)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

func TestTDEEnvelope_Validate(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var env *envelope.TDEEnvelope
		err := env.Validate()
		require.Error(t, err)
		assert.Equal(t, "envelope cannot be nil", err.Error())
	})

	t.Run("empty status returns validation error", func(t *testing.T) {
		env := &envelope.TDEEnvelope{
			ID:     "env-val-1",
			Status: "",
		}
		err := env.Validate()
		require.Error(t, err)
		assert.Equal(t, "missing required field: status", err.Error())
	})

	t.Run("valid status passes validation", func(t *testing.T) {
		env := &envelope.TDEEnvelope{
			ID:     "env-val-2",
			Status: envelope.StatusPending,
		}
		err := env.Validate()
		require.NoError(t, err)
	})
}

func TestTDEEnvelope_Serialize(t *testing.T) {
	t.Run("nil receiver serialization error", func(t *testing.T) {
		var env *envelope.TDEEnvelope
		data, err := env.Serialize()
		require.Error(t, err)
		assert.Nil(t, data)
		assert.Equal(t, "cannot serialize nil envelope", err.Error())
	})

	t.Run("valid envelope serialization", func(t *testing.T) {
		orig := &envelope.TDEEnvelope{
			ID:      "env-serialize-1",
			Kind:    "tde_envelope",
			Status:  envelope.StatusSuccess,
			AgentID: "agent-alpha",
			Intent:  "serialize-test",
			Payload: `{"ok":true}`,
		}
		data, err := orig.Serialize()
		require.NoError(t, err)
		require.NotEmpty(t, data)

		var parsed envelope.TDEEnvelope
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, *orig, parsed)
	})
}

func TestTDEEnvelope_RoundTrip(t *testing.T) {
	testEnvelopes := []*envelope.TDEEnvelope{
		{
			ID:      "rt-1",
			Kind:    "task_decision",
			Status:  envelope.StatusPending,
			AgentID: "agt-1",
			Intent:  "deploy",
			Payload: "payload-1",
		},
		{
			ID:      "rt-2",
			Kind:    "tde_envelope",
			Status:  envelope.StatusEscalated,
			AgentID: "agt-2",
			Intent:  "escalate",
			Payload: "escalation-details",
		},
	}

	for i, orig := range testEnvelopes {
		t.Run(fmt.Sprintf("envelope-%d", i), func(t *testing.T) {
			rawBytes, err := orig.Serialize()
			require.NoError(t, err)

			reconstructed, err := envelope.ParseTDEEnvelope(rawBytes)
			require.NoError(t, err)
			require.NotNil(t, reconstructed)
			assert.Equal(t, orig.ID, reconstructed.ID)
			assert.Equal(t, orig.Kind, reconstructed.Kind)
			assert.Equal(t, orig.Status, reconstructed.Status)
			assert.Equal(t, orig.AgentID, reconstructed.AgentID)
			assert.Equal(t, orig.Intent, reconstructed.Intent)
			assert.Equal(t, orig.Payload, reconstructed.Payload)
		})
	}
}

func TestParseTDEEnvelope_ConcurrentLoad(t *testing.T) {
	const workerCount = 10
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*iterations*2)

	for w := 0; w < workerCount; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("tde_concurrency_worker", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runConcurrentEnvelopeWorker(workerID, iterations, errCh)
			})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}

func runConcurrentEnvelopeWorker(workerID, iterations int, errCh chan<- error) {
	for i := 0; i < iterations; i++ {
		validJSON := []byte(fmt.Sprintf(
			`{"id":"env-%d-%d","kind":"tde_envelope","status":"success","agent_id":"agent-%d","intent":"test","payload":"data-%d"}`,
			workerID, i, workerID, i,
		))
		env, err := envelope.ParseTDEEnvelope(validJSON)
		if err != nil {
			errCh <- fmt.Errorf("unexpected error on valid input: %w", err)
			return
		}
		if env == nil || env.Status != envelope.StatusSuccess {
			errCh <- fmt.Errorf("unexpected envelope content: %+v", env)
			return
		}

		invalidJSON := []byte(fmt.Sprintf(`{"id":"env-%d-%d","status":""}`, workerID, i))
		_, err = envelope.ParseTDEEnvelope(invalidJSON)
		if err == nil {
			errCh <- fmt.Errorf("expected error on empty status but got nil for worker %d iter %d", workerID, i)
			return
		}
	}
}
