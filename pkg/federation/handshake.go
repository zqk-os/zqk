package federation

import (
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// HandshakeRequest is sent by a kernel initiating a federation handshake.
type HandshakeRequest struct {
	ProtocolVersion string       `json:"protocol_version"`
	KernelID        string       `json:"kernel_id"`
	Namespace       string       `json:"namespace"`
	PublicKey       string       `json:"public_key"`
	Capabilities    []Capability `json:"capabilities"`
	Timestamp       time.Time    `json:"timestamp"`
}

// HandshakeResponse is sent by a kernel accepting or declining a federation handshake.
type HandshakeResponse struct {
	Accepted     bool         `json:"accepted"`
	Message      string       `json:"message,omitempty"`
	KernelID     string       `json:"kernel_id"`
	PublicKey    string       `json:"public_key"`
	Capabilities []Capability `json:"capabilities"`
}

// Capability represents a shared skill or resource offered by a kernel.
type Capability struct {
	Kind string `json:"kind"` // "agent_skill", "resource", "compute"
	ID   string `json:"id"`   // unique identifier for the capability
	Name string `json:"name"` // human-readable name
}

// RemoteKernelState represents the result of a successful handshake.
type RemoteKernelState struct {
	ID               string
	Endpoint         string
	PublicKey        string
	SharedNamespaces []string
	Capabilities     []Capability
	LastHandshake    time.Time
}

// RemoteSkill represents a skill available on a remote kernel.
type RemoteSkill struct {
	ID       string
	Provider string
	Endpoint string
	Token    string
}

// ToMap converts the state to a Kernel object map.
func (s *RemoteKernelState) ToMap() map[string]any {
	caps := make([]string, len(s.Capabilities))
	for i, c := range s.Capabilities {
		caps[i] = c.ID
	}

	id := s.ID
	if !strings.HasPrefix(id, "REM-") {
		id = "REM-" + id
	}

	return map[string]any{
		objects.FieldKeyID:               id,
		objects.FieldKeyKind:             objects.KindRemoteKernel,
		objects.FieldKeyTitle:            "Remote Kernel: " + s.ID,
		objects.FieldKeyEndpoint:         s.Endpoint,
		objects.FieldKeyPublicKey:        s.PublicKey,
		objects.FieldKeySharedNamespaces: s.SharedNamespaces,
		objects.FieldKeyCapabilities:     caps,
		objects.FieldKeyLastHeartbeat:    s.LastHandshake.Format(time.RFC3339),
		objects.FieldKeyStatus:           "implemented",
	}
}
