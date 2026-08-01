// pkg/envelope/tde.go
package envelope

import (
	"encoding/json"
	"fmt"
)

type Status string

const (
	StatusPending Status = "pending"
)

type TDEEnvelope struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Status  Status `json:"status"`
	AgentID string `json:"agent_id"`
	Intent  string `json:"intent"`
	Payload string `json:"payload"`
}

func ParseTDEEnvelope(input []byte) (*TDEEnvelope, error) {
	var env TDEEnvelope
	if err := json.Unmarshal(input, &env); err != nil {
		return nil, err
	}
	if env.Status == "" {
		return nil, fmt.Errorf("missing required field: status")
	}
	return &env, nil
}
