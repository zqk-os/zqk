package packcmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewPackCmd creates the top-level 'zqk pack' command.
func NewPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Holonic swarm package management (scaffold, seal, validate)",
		Long:  "Initialize, cryptographically seal, and validate portable holonic swarm packages.",
	}

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newSealCmd())
	cmd.AddCommand(newValidateCmd())

	return cmd
}

func newInitCmd() *cobra.Command {
	var (
		name    string
		version string
		desc    string
	)

	cmd := &cobra.Command{
		Use:   "init [target_dir]",
		Short: "Scaffold a new holonic swarm package structure",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}

			if name == "" {
				name = filepath.Base(filepath.Clean(targetDir))
				if name == "." || name == "/" {
					name = "custom-swarm-pack"
				}
			}

			// Validate SemVer upfront
			if _, err := pack.ParseSemVer(version); err != nil {
				return fmt.Errorf("invalid package version: %w", err)
			}

			if err := fileutil.MkdirAll(targetDir, paths.DirPerm755); err != nil {
				return fmt.Errorf("failed to create target directory: %w", err)
			}

			templatesDir := filepath.Join(targetDir, "templates")
			if err := fileutil.MkdirAll(templatesDir, paths.DirPerm755); err != nil {
				return fmt.Errorf("failed to create templates directory: %w", err)
			}

			membranesDir := filepath.Join(targetDir, "membranes")
			if err := fileutil.MkdirAll(membranesDir, paths.DirPerm755); err != nil {
				return fmt.Errorf("failed to create membranes directory: %w", err)
			}

			manifestPath := filepath.Join(targetDir, "swarm.yaml")
			if fileutil.Exists(manifestPath) {
				return fmt.Errorf("swarm.yaml already exists at %s", manifestPath)
			}

			samplePkg := pack.SwarmPackage{
				Schema:      "https://zqk.dev/schemas/swarm_package_spec.schema.json",
				Name:        name,
				Version:     version,
				Description: desc,
				License:     "Apache-2.0",
				Parameters: map[string]pack.ParameterDef{
					"baseline": {
						Type:        "float",
						Default:     4.5,
						Description: "Target quality baseline",
						Required:    false,
					},
				},
				Membranes: []pack.MembraneRule{
					{Path: ".zqk/process/", Mode: "read_only"},
					{Path: ".zqk/audit/", Mode: "audit_log"},
				},
				Agents: []pack.AgentConfig{
					{
						Name:         "Lead Specialist",
						Role:         "software_engineer",
						Skills:       []string{"ASK-COMMUNITY-CODE-CRAFTSMAN"},
						SystemPrompt: "Execute tasks adhering to clean craftsmanship principles.",
					},
					{
						Name:         "Adversarial Verifier",
						Role:         "qa_auditor",
						Skills:       []string{"ASK-COMMUNITY-QA-VERIFICATION"},
						SystemPrompt: "Subject outcomes to falsifiable verification and fail-closed bounds.",
					},
				},
				Tasks: []pack.TaskConfig{
					{
						ID:    "task-evaluate",
						Title: fmt.Sprintf("Execute core evaluation for %s", name),
						Role:  "software_engineer",
					},
					{
						ID:        "task-verify",
						Title:     "Verify evaluation outcomes and certify compliance",
						Role:      "qa_auditor",
						DependsOn: []string{"task-evaluate"},
					},
				},
			}

			manifestBytes, err := yaml.Marshal(&samplePkg)
			if err != nil {
				return fmt.Errorf("failed to marshal scaffold manifest: %w", err)
			}

			if err := fileutil.WriteFile(manifestPath, manifestBytes, paths.FilePerm644); err != nil {
				return fmt.Errorf("failed to write swarm.yaml: %w", err)
			}

			// Add sample prompt template
			sampleTpl := filepath.Join(templatesDir, "eval_prompt.yaml")
			tplContent := `prompt: |
  Evaluate the system against defined requirements and record findings.
`
			_ = fileutil.WriteFile(sampleTpl, []byte(tplContent), paths.FilePerm644)

			cmd.Printf("✓ Successfully scaffolded holonic swarm package %q in %s\n", name, targetDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Package name")
	cmd.Flags().StringVar(&version, "version", "1.0.0", "Semantic version (SemVer 2.0.0)")
	cmd.Flags().StringVar(&desc, "description", "A portable holonic swarm package", "Package description")

	return cmd
}

func newSealCmd() *cobra.Command {
	var (
		signerID  string
		keyPath   string
		pubOut    string
	)

	cmd := &cobra.Command{
		Use:   "seal [pack_dir]",
		Short: "Cryptographically seal a swarm package with Ed25519 signature",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packDir := "."
			if len(args) > 0 {
				packDir = args[0]
			}

			var privKey ed25519.PrivateKey
			var pubKey ed25519.PublicKey

			if keyPath != "" {
				keyData, err := fileutil.ReadFile(keyPath)
				if err != nil {
					return fmt.Errorf("failed to read private key from %s: %w", keyPath, err)
				}
				keyStr := strings.TrimSpace(string(keyData))
				decoded, err := hex.DecodeString(keyStr)
				if err != nil || len(decoded) != ed25519.PrivateKeySize {
					// Fall back to raw bytes
					if len(keyData) == ed25519.PrivateKeySize {
						privKey = ed25519.PrivateKey(keyData)
					} else {
						return fmt.Errorf("invalid private key in %s (must be 64 bytes or hex-encoded)", keyPath)
					}
				} else {
					privKey = ed25519.PrivateKey(decoded)
				}
				pubKey = privKey.Public().(ed25519.PublicKey)
			} else {
				// Ephemeral/default key generation for sealing
				var err error
				pubKey, privKey, err = ed25519.GenerateKey(rand.Reader)
				if err != nil {
					return fmt.Errorf("failed to generate Ed25519 key: %w", err)
				}
			}

			if pubOut != "" {
				pubHex := hex.EncodeToString(pubKey)
				if err := fileutil.WriteFile(pubOut, []byte(pubHex), paths.FilePerm644); err != nil {
					return fmt.Errorf("failed to write public key to %s: %w", pubOut, err)
				}
			}

			integrity, err := pack.SealPack(packDir, privKey, signerID)
			if err != nil {
				return fmt.Errorf("failed to seal swarm package: %w", err)
			}

			cmd.Printf("✓ Cryptographically sealed pack in %s\n", packDir)
			cmd.Printf("  Signer:  %s\n", integrity.SignerID)
			cmd.Printf("  Digest:  %s\n", integrity.ContentDigest)
			cmd.Printf("  Pubkey:  %s\n", hex.EncodeToString(pubKey))
			return nil
		},
	}

	cmd.Flags().StringVar(&signerID, "signer-id", "architect@zqk.dev", "Signer identity or email")
	cmd.Flags().StringVar(&keyPath, "key", "", "Path to Ed25519 private key (optional)")
	cmd.Flags().StringVar(&pubOut, "pubkey-out", "", "Path to write public key hex (optional)")

	return cmd
}

func newValidateCmd() *cobra.Command {
	var keyPath string

	cmd := &cobra.Command{
		Use:   "validate [pack_dir]",
		Short: "Validate swarm package schema, SemVer, Merkle digest, and cryptographic signature",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packDir := "."
			if len(args) > 0 {
				packDir = args[0]
			}

			manifestPath := filepath.Join(packDir, "swarm.yaml")
			pkg, err := pack.LoadManifestFile(manifestPath)
			if err != nil {
				return fmt.Errorf("package validation failed: %w", err)
			}

			var pubKey ed25519.PublicKey
			if keyPath != "" {
				keyData, err := fileutil.ReadFile(keyPath)
				if err != nil {
					return fmt.Errorf("failed to read public key from %s: %w", keyPath, err)
				}
				keyStr := strings.TrimSpace(string(keyData))
				decoded, err := hex.DecodeString(keyStr)
				if err != nil || len(decoded) != ed25519.PublicKeySize {
					if len(keyData) == ed25519.PublicKeySize {
						pubKey = ed25519.PublicKey(keyData)
					} else {
						return fmt.Errorf("invalid public key (must be 32 bytes or hex-encoded)")
					}
				} else {
					pubKey = ed25519.PublicKey(decoded)
				}
			}

			if pkg.Integrity != nil {
				if _, err := pack.VerifyPackIntegrity(packDir, pubKey); err != nil {
					return fmt.Errorf("integrity verification failed: %w", err)
				}
				cmd.Printf("✓ Swarm package %s v%s is valid and SEALED (%s)\n", pkg.Name, pkg.Version, pkg.Integrity.ContentDigest)
			} else {
				cmd.Printf("⚠️ Swarm package %s v%s is semantically valid but UNSEALED\n", pkg.Name, pkg.Version)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&keyPath, "key", "", "Path to Ed25519 public key for signature verification (optional)")

	return cmd
}
