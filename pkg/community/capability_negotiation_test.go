package community

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCapabilityNegotiation_StaticFloorInvariant(t *testing.T) {
	req := &NegotiationRequest{
		ClientID:        "client-alpha",
		ProtocolVersion: "1.0",
		Capabilities: []CapabilityRequirement{
			{Name: "ast.parse", Required: true},
			{Name: "ast.highlight", Required: false},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got: %v", err)
	}

	if req.ClientID != "client-alpha" {
		t.Errorf("expected client-alpha, got: %s", req.ClientID)
	}
	if len(req.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(req.Capabilities))
	}
}

func TestCapabilityNegotiation_OperationalDynamicProof(t *testing.T) {
	engine := NewHandshakeEngine()

	manifest := &ProviderManifest{
		ID:               "zqk.provider.parser",
		Name:             "Parser Provider",
		Version:          "1.0.0",
		MinEngineVersion: "0.3.6",
		Capabilities:     []string{"ast.parse", "ast.highlight", "telemetry"},
	}

	if err := engine.RegisterProvider(manifest); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	req := &NegotiationRequest{
		ClientID:        "client-worker",
		ProtocolVersion: "1.0",
		Capabilities: []CapabilityRequirement{
			{Name: "ast.parse", Required: true},
			{Name: "telemetry", Required: false},
		},
	}

	receipt, err := engine.Negotiate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful negotiation, got: %v", err)
	}

	if receipt.ProviderID != "zqk.provider.parser" {
		t.Errorf("expected provider zqk.provider.parser, got: %s", receipt.ProviderID)
	}
	if receipt.AgreedProtocol != "1.0" {
		t.Errorf("expected protocol 1.0, got: %s", receipt.AgreedProtocol)
	}
	if len(receipt.MatchedCapabilities) != 2 {
		t.Errorf("expected 2 matched capabilities, got %d", len(receipt.MatchedCapabilities))
	}
}

func TestCapabilityNegotiation_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	engine := NewHandshakeEngine()

	manifest := &ProviderManifest{
		ID:               "zqk.provider.storage",
		Name:             "Storage Provider",
		Version:          "1.0.0",
		MinEngineVersion: "0.3.6",
		Capabilities:     []string{"blob.read"},
	}
	if err := engine.RegisterProvider(manifest); err != nil {
		t.Fatalf("RegisterProvider failed: %v", err)
	}

	// Case 1: Empty Client ID
	reqBadClient := &NegotiationRequest{
		ClientID:        "",
		ProtocolVersion: "1.0",
		Capabilities:    []CapabilityRequirement{{Name: "blob.read", Required: true}},
	}
	if _, err := engine.Negotiate(context.Background(), reqBadClient); !errors.Is(err, ErrNegotiationInvalid) {
		t.Errorf("expected ErrNegotiationInvalid for empty client id, got: %v", err)
	}

	// Case 2: Unsupported Protocol Version
	reqBadProto := &NegotiationRequest{
		ClientID:        "client-1",
		ProtocolVersion: "99.0",
		Capabilities:    []CapabilityRequirement{{Name: "blob.read", Required: true}},
	}
	if _, err := engine.Negotiate(context.Background(), reqBadProto); !errors.Is(err, ErrIncompatibleProtocol) {
		t.Errorf("expected ErrIncompatibleProtocol, got: %v", err)
	}

	// Case 3: Incompatible Capabilities (Required capability missing)
	reqMissingCap := &NegotiationRequest{
		ClientID:        "client-1",
		ProtocolVersion: "1.0",
		Capabilities:    []CapabilityRequirement{{Name: "vector.search", Required: true}},
	}
	if _, err := engine.Negotiate(context.Background(), reqMissingCap); !errors.Is(err, ErrIncompatibleCapabilities) {
		t.Errorf("expected ErrIncompatibleCapabilities, got: %v", err)
	}
}

func TestCapabilityNegotiation_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_CAPABILITY_NEGOTIATION_AND_PROTOCOL_HANDSHAKE_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0057 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0057 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0057 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
