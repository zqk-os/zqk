package community

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	ErrNegotiationInvalid       = errors.New("negotiation: request is invalid or missing required fields")
	ErrIncompatibleCapabilities = errors.New("negotiation: no compatible provider satisfies required capabilities")
	ErrIncompatibleProtocol     = errors.New("negotiation: unsupported client protocol version")
)

// CapabilityRequirement specifies a capability name and optional version constraint.
type CapabilityRequirement struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// NegotiationRequest represents a client's capability negotiation handshake.
type NegotiationRequest struct {
	ClientID        string                  `json:"client_id"`
	ProtocolVersion string                  `json:"protocol_version"`
	Capabilities    []CapabilityRequirement `json:"capabilities"`
}

// Validate checks static structural invariants.
func (r *NegotiationRequest) Validate() error {
	if r == nil {
		return ErrNegotiationInvalid
	}
	if strutil.IsBlank(r.ClientID) {
		return fmt.Errorf("%w: missing client_id", ErrNegotiationInvalid)
	}
	if strutil.IsBlank(r.ProtocolVersion) {
		return fmt.Errorf("%w: missing protocol_version", ErrNegotiationInvalid)
	}
	if len(r.Capabilities) == 0 {
		return fmt.Errorf("%w: at least one capability must be specified", ErrNegotiationInvalid)
	}
	return nil
}

// NegotiationReceipt confirms agreed protocol and matched provider.
type NegotiationReceipt struct {
	SessionID           string    `json:"session_id"`
	ProviderID          string    `json:"provider_id"`
	AgreedProtocol      string    `json:"agreed_protocol"`
	MatchedCapabilities []string  `json:"matched_capabilities"`
	NegotiatedAt        time.Time `json:"negotiated_at"`
}

// HandshakeEngine negotiates capabilities between client requests and registered providers.
type HandshakeEngine struct {
	mu        sync.RWMutex
	providers map[string]*ProviderManifest
}

// NewHandshakeEngine initializes a new negotiation engine.
func NewHandshakeEngine() *HandshakeEngine {
	return &HandshakeEngine{
		providers: make(map[string]*ProviderManifest),
	}
}

// RegisterProvider adds a candidate provider to the negotiation catalog.
func (e *HandshakeEngine) RegisterProvider(manifest *ProviderManifest) error {
	if manifest == nil {
		return errors.New("negotiation: provider manifest cannot be nil")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers[manifest.ID] = manifest
	return nil
}

// Negotiate evaluates a client request and produces a negotiation receipt.
func (e *HandshakeEngine) Negotiate(ctx context.Context, req *NegotiationRequest) (*NegotiationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Only 1.0 protocol supported in v0.3.6
	if req.ProtocolVersion != "1.0" && req.ProtocolVersion != "v1" {
		return nil, fmt.Errorf("%w: client requested %s, supported: 1.0, v1", ErrIncompatibleProtocol, req.ProtocolVersion)
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, provider := range e.providers {
		provCaps := make(map[string]bool)
		for _, cap := range provider.Capabilities {
			provCaps[strings.ToLower(cap)] = true
		}

		allRequiredMatch := true
		var matched []string

		for _, reqCap := range req.Capabilities {
			nameLower := strings.ToLower(reqCap.Name)
			if provCaps[nameLower] {
				matched = append(matched, reqCap.Name)
			} else if reqCap.Required {
				allRequiredMatch = false
				break
			}
		}

		if allRequiredMatch {
			sessionID := fmt.Sprintf("sess-%s-%d", req.ClientID, time.Now().UnixNano())
			return &NegotiationReceipt{
				SessionID:           sessionID,
				ProviderID:          provider.ID,
				AgreedProtocol:      req.ProtocolVersion,
				MatchedCapabilities: matched,
				NegotiatedAt:        time.Now(),
			}, nil
		}
	}

	return nil, ErrIncompatibleCapabilities
}
