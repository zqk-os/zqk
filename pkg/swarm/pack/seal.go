package pack

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// ErrUnsealedPack is returned when a pack has no integrity section.
	ErrUnsealedPack = errors.New("swarm package is not sealed (missing integrity block)")

	// ErrDigestMismatch is returned when the computed content digest does not match the manifest integrity digest.
	ErrDigestMismatch = errors.New("swarm package content digest mismatch (tampering detected)")

	// ErrInvalidSignature is returned when the Ed25519 signature fails verification.
	ErrInvalidSignature = errors.New("swarm package cryptographic signature verification failed")

	// ErrUnsupportedAlgorithm is returned when the signing algorithm is not ed25519.
	ErrUnsupportedAlgorithm = errors.New("unsupported signature algorithm")
)

// ComputePackDigest calculates a deterministic SHA-256 Merkle root digest over the files in packDir.
// The manifest's integrity block is excluded from the digest calculation so that signing is idempotent.
func ComputePackDigest(packDir string) (string, error) {
	manifestPath := filepath.Join(packDir, "swarm.yaml")
	if !fileutil.Exists(manifestPath) {
		return "", fmt.Errorf("swarm manifest not found at %s", manifestPath)
	}

	pkg, err := LoadManifestFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to load manifest for digest: %w", err)
	}

	canonicalPkg := *pkg
	canonicalPkg.Integrity = nil
	canonicalManifest, err := yaml.Marshal(&canonicalPkg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal canonical manifest: %w", err)
	}

	type fileHash struct {
		relPath string
		hash    string
	}
	var hashes []fileHash

	// Manifest hash
	hManifest := sha256.Sum256(canonicalManifest)
	hashes = append(hashes, fileHash{
		relPath: "swarm.yaml",
		hash:    hex.EncodeToString(hManifest[:]),
	})

	// Walk packDir for additional files (e.g. templates, membranes, schemas)
	err = filepath.Walk(packDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(packDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "swarm.yaml" || strings.HasPrefix(rel, ".") {
			return nil
		}

		fBytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h := sha256.Sum256(fBytes)
		hashes = append(hashes, fileHash{
			relPath: rel,
			hash:    hex.EncodeToString(h[:]),
		})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to walk pack directory for digest: %w", err)
	}

	// Sort file hashes deterministically by relative path
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].relPath < hashes[j].relPath
	})

	// Compute Merkle root
	hasher := sha256.New()
	for _, fh := range hashes {
		entry := fmt.Sprintf("%s:%s\n", fh.relPath, fh.hash)
		io.WriteString(hasher, entry)
	}
	rootDigest := hex.EncodeToString(hasher.Sum(nil))
	return "sha256:" + rootDigest, nil
}

// stripIntegrityFromNode removes the integrity mapping key if present.
func stripIntegrityFromNode(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.DocumentNode {
		for _, child := range n.Content {
			stripIntegrityFromNode(child)
		}
		return
	}
	if n.Kind == yaml.MappingNode {
		var newContent []*yaml.Node
		for i := 0; i < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			valNode := n.Content[i+1]
			if keyNode.Value == "integrity" {
				continue
			}
			newContent = append(newContent, keyNode, valNode)
		}
		n.Content = newContent
	}
}

// SealPack signs the swarm package in packDir using the provided Ed25519 private key.
// It updates swarm.yaml with the calculated Merkle content digest and Ed25519 signature.
func SealPack(packDir string, privKey ed25519.PrivateKey, signerID string) (*Integrity, error) {
	manifestPath := filepath.Join(packDir, "swarm.yaml")
	pkg, err := LoadManifestFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot seal invalid manifest: %w", err)
	}

	digest, err := ComputePackDigest(packDir)
	if err != nil {
		return nil, fmt.Errorf("failed to compute pack digest: %w", err)
	}

	sig := ed25519.Sign(privKey, []byte(digest))
	sigHex := hex.EncodeToString(sig)

	integrity := &Integrity{
		Algorithm:     "ed25519",
		SignerID:      signerID,
		ContentDigest: digest,
		Signature:     sigHex,
	}

	pkg.Integrity = integrity
	updatedBytes, err := yaml.Marshal(pkg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signed manifest: %w", err)
	}

	if err := fileutil.WriteFile(manifestPath, updatedBytes, paths.FilePerm644); err != nil {
		return nil, fmt.Errorf("failed to write sealed manifest: %w", err)
	}

	return integrity, nil
}

// VerifyPackIntegrity verifies that the swarm package in packDir has a valid Merkle digest
// and valid Ed25519 cryptographic signature.
func VerifyPackIntegrity(packDir string, pubKey ed25519.PublicKey) (*SwarmPackage, error) {
	manifestPath := filepath.Join(packDir, "swarm.yaml")
	pkg, err := LoadManifestFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot verify invalid manifest: %w", err)
	}

	if pkg.Integrity == nil {
		return nil, ErrUnsealedPack
	}

	if strings.ToLower(pkg.Integrity.Algorithm) != "ed25519" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, pkg.Integrity.Algorithm)
	}

	currentDigest, err := ComputePackDigest(packDir)
	if err != nil {
		return nil, fmt.Errorf("failed to compute pack digest during verification: %w", err)
	}

	if currentDigest != pkg.Integrity.ContentDigest {
		return nil, fmt.Errorf("%w: recorded %s, recomputed %s", ErrDigestMismatch, pkg.Integrity.ContentDigest, currentDigest)
	}

	sigBytes, err := hex.DecodeString(pkg.Integrity.Signature)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid hex signature: %v", ErrInvalidSignature, err)
	}

	if len(pubKey) > 0 {
		if !ed25519.Verify(pubKey, []byte(pkg.Integrity.ContentDigest), sigBytes) {
			return nil, ErrInvalidSignature
		}
	}

	return pkg, nil
}
