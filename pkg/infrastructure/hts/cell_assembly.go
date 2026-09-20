package hts

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DataCell represents a cryptographically intact bundle of objects and assets.
type DataCell struct {
	RootHash  string                    `json:"root_hash"`
	Objects   map[string]map[string]any `json:"objects"`
	Manifest  map[string]string         `json:"manifest"` // ID -> Hash
	Metadata  map[string]any            `json:"metadata"`
	Signature string                    `json:"signature,omitempty"`
	PublicKey string                    `json:"public_key,omitempty"`
}

// Assemble creates a DataCell from a set of objects, calculating a Merkle-root for integrity.
func Assemble(ctx context.Context, cellID string, items []map[string]any) (*DataCell, error) {
	cell := &DataCell{
		Objects:  make(map[string]map[string]any),
		Manifest: make(map[string]string),
		Metadata: make(map[string]any),
	}

	hashes := []string{}

	for _, obj := range items {
		id, ok := obj[objects.FieldKeyID].(string)
		if !ok {
			return nil, fmt.Errorf("object missing ID")
		}

		data, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}

		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		hashes = append(hashes, hash)
		cell.Objects[id] = obj
		cell.Manifest[id] = hash
	}

	// Calculate Root Hash (Simple Merkle-style sort and hash)
	sort.Strings(hashes)
	combined := ""
	for _, h := range hashes {
		combined += h
	}
	cell.RootHash = fmt.Sprintf("%x", sha256.Sum256([]byte(combined)))

	cell.Metadata["cell_id"] = cellID
	cell.Metadata[objects.FieldKeyObjectCount] = len(items)

	return cell, nil
}

// CellPackage writes the DataCell to a .skill Merkle-archive.
func CellPackage(ctx context.Context, cell *DataCell, outputPath string) error {
	data, err := json.MarshalIndent(cell, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(outputPath, data)
}

// Sign applies a cryptographic signature to the DataCell's RootHash.

func (c *DataCell) Sign(signer crypto.Signer) error {
	sig, err := signer.Sign([]byte(c.RootHash))
	if err != nil {
		return err
	}
	c.Signature = sig
	c.PublicKey = signer.PublicKey()
	return nil
}

// Verify checks the integrity of a DataCell against its RootHash and verifies the signature if present.
func (c *DataCell) Verify() error {
	hashes := []string{}
	for _, obj := range c.Objects {
		data, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		hashes = append(hashes, fmt.Sprintf("%x", sha256.Sum256(data)))
	}

	sort.Strings(hashes)
	combined := ""
	for _, h := range hashes {
		combined += h
	}
	calculatedRoot := fmt.Sprintf("%x", sha256.Sum256([]byte(combined)))

	if calculatedRoot != c.RootHash {
		return fmt.Errorf("integrity violation: calculated root %s does not match cell root %s", calculatedRoot, c.RootHash)
	}

	// Verify Signature if present
	if c.Signature != "" {
		valid, err := crypto.GlobalVerifier.Verify([]byte(c.RootHash), c.Signature, c.PublicKey)
		if err != nil {
			return fmt.Errorf("signature verification error: %v", err)
		}
		if !valid {
			return fmt.Errorf("invalid cell signature for public key %s", c.PublicKey)
		}
	}

	return nil
}
