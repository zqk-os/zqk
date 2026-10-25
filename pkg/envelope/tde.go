// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

// Package envelope provides standard Task Decision Envelope (TDE) definitions and ingestion parsing.
package envelope

import (
	"encoding/json"
	"fmt"
)

// Status represents the execution or intake state of a Task Decision Envelope.
type Status string

const (
	// StatusPending indicates the envelope is awaiting execution or human approval.
	StatusPending Status = "pending"
	// StatusSuccess indicates the envelope action completed successfully.
	StatusSuccess Status = "success"
	// StatusFailure indicates the envelope action failed execution.
	StatusFailure Status = "failure"
	// StatusDeferred indicates the envelope action is deferred for later evaluation.
	StatusDeferred Status = "deferred"
	// StatusEscalated indicates the envelope action required human or swarm escalation.
	StatusEscalated Status = "escalated"
)

// TDEEnvelope models a Task Decision Envelope exchanged during autonomous agent execution.
type TDEEnvelope struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Status  Status `json:"status"`
	AgentID string `json:"agent_id"`
	Intent  string `json:"intent"`
	Payload string `json:"payload"`
}

// Validate verifies that the envelope satisfies structural and invariant requirements.
func (e *TDEEnvelope) Validate() error {
	if e == nil {
		return fmt.Errorf("envelope cannot be nil")
	}
	if e.Status == "" {
		return fmt.Errorf("missing required field: status")
	}
	return nil
}

// Serialize marshals the envelope into JSON format.
func (e *TDEEnvelope) Serialize() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("cannot serialize nil envelope")
	}
	return json.Marshal(e)
}

// ParseTDEEnvelope unmarshals JSON input into a TDEEnvelope and validates required invariants.
func ParseTDEEnvelope(input []byte) (*TDEEnvelope, error) {
	var env TDEEnvelope
	if err := json.Unmarshal(input, &env); err != nil {
		return nil, err
	}
	if err := env.Validate(); err != nil {
		return nil, err
	}
	return &env, nil
}
