package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
	"github.com/lanceman/zqk/pkg/infrastructure/hts"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/policy"
)

// P2PTransferProtocol manages the movement of DataCells between kernels.
type P2PTransferProtocol struct {
	spine  infrastructure.SpinalSpine
	signer crypto.Signer
}

// NewP2PTransferProtocol creates a new P2PTransferProtocol.
func NewP2PTransferProtocol(spine infrastructure.SpinalSpine, signer crypto.Signer) *P2PTransferProtocol {
	return &P2PTransferProtocol{
		spine:  spine,
		signer: signer,
	}
}

// Transfer initiates a P2P transfer of a DataCell.
func (p *P2PTransferProtocol) Transfer(ctx context.Context, sourceID string, targetID string, cell *hts.DataCell) error {
	// 1. Sign locally if a signer is available
	if p.signer != nil {
		if err := cell.Sign(p.signer); err != nil {
			return fmt.Errorf("failed to sign cell: %v", err)
		}
	}

	// 2. Verify locally before sending
	if err := cell.Verify(); err != nil {
		return fmt.Errorf("refusing to transfer invalid cell: %v", err)
	}

	// 3. Create TDE Envelope for Transfer
	tde := &policy.TrustDomainEnvelope{
		ID:                "TDE-P2P-" + cell.Metadata["cell_id"].(string),
		Scope:             "cell_transfer",
		AllowedNamespaces: []string{"*"},
		MaxRuntimeSeconds: 30,
		MCPPermissions:    []string{"cell:transfer"},
		Neurological: &policy.NeurologicalGovernance{
			Confidence:     0.99,
			IsStreaming:    false,
			DecisionBranch: "core_mesh_transfer",
		},
		Payload: map[string]any{
			"source_kernel_id":          sourceID,
			"target_kernel_id":          targetID,
			"root_hash":                 cell.RootHash,
			objects.FieldKeyObjectCount: cell.Metadata[objects.FieldKeyObjectCount],
			objects.FieldKeySignature:   cell.Signature,
			objects.FieldKeyPublicKey:   cell.PublicKey,
		},
	}
	if p.signer != nil {
		if err := tde.Sign(p.signer); err != nil {
			return fmt.Errorf("failed to sign TDE: %v", err)
		}
	}

	// 4. Publish Transfer Event to the Industrial Spine
	event := infrastructure.Event{
		ObjectID: cell.Metadata["cell_id"].(string),
		Kind:     "cell_transfer",
		Op:       "initiated",
		Payload: map[string]any{
			"tde_envelope": tde,
			"cell":         cell,
		},
	}

	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("[P2P] Initiating transfer of cell %s from %s to %s...\n", event.ObjectID, sourceID, targetID)).Log()

	sigStatus := "unsigned"
	if tde.Signature != "" {
		sigStatus = "signed:" + tde.Signature[:8]
	}
	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [transfer] [%s] %s: Sending expertise cell %s to %s\n", sigStatus, sourceID, event.ObjectID, targetID)).Log()

	return p.spine.Publish(ctx, event)
}

// HandleTransfer receives a transfer event and performs verification (Handshaker-ee).
func (p *P2PTransferProtocol) HandleTransfer(ctx context.Context, event infrastructure.Event) error {
	// 1. Enforce TDE
	tdeRaw, ok := event.Payload["tde_envelope"]
	if !ok {
		return fmt.Errorf("ABORT: Handshake rejected: missing TDE envelope")
	}

	var env policy.TrustDomainEnvelope
	tdeBytes, err := json.Marshal(tdeRaw)
	if err != nil {
		return fmt.Errorf("ABORT: Invalid TDE format: %v", err)
	}
	if err := json.Unmarshal(tdeBytes, &env); err != nil {
		return fmt.Errorf("ABORT: Failed to parse TDE: %v", err)
	}

	if err := env.Verify(); err != nil {
		return fmt.Errorf("ABORT: Invalid TDE: %v", err)
	}

	if env.Scope != "cell_transfer" && env.Scope != "admin" {
		return fmt.Errorf("ABORT: TDE scope '%s' does not permit cell transfer", env.Scope)
	}

	// 2. Extract payload
	payloadMap, ok := env.Payload.(map[string]any)
	if !ok {
		return fmt.Errorf("ABORT: Invalid TDE payload format")
	}

	rootHash, _ := payloadMap["root_hash"].(string)

	// 3. Merkle Proof Verification (Verify the cell itself)
	cellRaw, ok := event.Payload["cell"]
	if !ok {
		return fmt.Errorf("ABORT: Transfer rejected: missing cell data")
	}

	var cell hts.DataCell
	cellBytes, err := json.Marshal(cellRaw)
	if err != nil {
		return fmt.Errorf("ABORT: Invalid cell format: %v", err)
	}
	if err := json.Unmarshal(cellBytes, &cell); err != nil {
		return fmt.Errorf("ABORT: Failed to parse cell: %v", err)
	}

	if err := cell.Verify(); err != nil {
		return fmt.Errorf("ABORT: Cell Merkle proof / integrity verification failed: %v", err)
	}

	if cell.RootHash != rootHash {
		return fmt.Errorf("ABORT: Cell root hash does not match TDE authorized root hash")
	}

	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("[P2P] Received transfer request for cell %s (Root: %s) within verified TDE %s.\n", event.ObjectID, rootHash, env.ID)).Log()

	// Simulation of success:
	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("[P2P] Cell %s verified. Integrating into local ontologies.\n", event.ObjectID)).Log()

	return nil
}
