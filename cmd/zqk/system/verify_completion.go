package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kernel/verification"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewVerifyCompletionCmd creates a new verify-completion command
func NewVerifyCompletionCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Verify completion of an object",
		"Objectively verify the cryptographic completion hash of an object against disk state.",
		"",
		"This command verifies that the verification_hash of a test_case or backlog_item matches",
		"the actual cryptographic hash of its referenced artifacts on disk.",
	).
		AddExample("Verify a backlog item", "%s system verify-completion BLI-123")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemVerifyCompletionCommandBuilder(), &cobra.Command{
		Use:   "verify-completion <object-id>",
		Short: "Verify cryptographic completion",
		Args:  cobra.ExactArgs(1),
	})

	cmd.Flags().Bool("organizer", false, "Execute composite execution organizer for multi-criteria test verification")
	cmd.Flags().String("topology", "sequential", "Execution topology mode for organizer (sequential, concurrent, hybrid_dag)")

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runVerifyCompletion(cmd, args[0])
	})

	helpBuilder.ApplyToCommand(cmd)

	return cmd
}

func runVerifyCompletion(cmd *cobra.Command, objectID string) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}

	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()
	store := proc.Storage()

	// 1. Load the object
	obj, err := store.Read(ctx, secCtx, objectID)
	if err != nil {
		if err == storage.ErrObjectNotFound {
			return errfmt.Errorf("object %s not found", objectID)
		}
		return errfmt.Newf("failed to read object").Wrap(err)
	}

	kind, _ := obj[objects.FieldKeyKind].(string)
	switch kind {
	case "test_case":
		return verifyTestCase(cmd, proc, obj, objectID)
	case "backlog_item":
		return verifyBacklogItem(cmd, proc, obj, objectID)
	}

	return errfmt.Errorf("verification not supported for kind %s", kind)
}

func verifyTestCase(cmd *cobra.Command, proc *cli.Processor, obj map[string]any, objectID string) error {
	projectRoot := zqkenv.ProjectRoot().Get()
	if projectRoot == "" {
		projectRoot = zqkenv.TestRoot().Get()
	}
	if projectRoot == "" {
		return errfmt.Errorf("project root not set")
	}

	expectedHash, _ := obj[objects.FieldKeyVerificationHash].(string)
	if expectedHash == "" {
		return errfmt.Errorf("test_case %s has no verification_hash", objectID)
	}

	artifactsRaw, ok := obj[objects.FieldKeyVerifiedArtifactRefs].([]any)
	if !ok || len(artifactsRaw) == 0 {
		return errfmt.Errorf("test_case %s has no verified_artifact_refs", objectID)
	}

	artifacts := make([]string, 0, len(artifactsRaw))
	for _, a := range artifactsRaw {
		if str, ok := a.(string); ok {
			artifacts = append(artifacts, str)
		}
	}

	// Compute hash
	actualHash, err := computeTestCaseHash(projectRoot, artifacts)
	if err != nil {
		return err
	}

	if actualHash != expectedHash {
		return errfmt.Errorf("verification failed for test_case %s: expected hash %s, actual hash %s", objectID, expectedHash, actualHash)
	}

	useOrganizer, _ := cmd.Flags().GetBool("organizer")
	if useOrganizer {
		if err := verifyTestCaseWithOrganizer(cmd, proc, obj, objectID); err != nil {
			return err
		}
	}

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Successfully verified test_case %s.\n", objectID)))
	return nil
}

func computeTestCaseHash(projectRoot string, artifacts []string) (string, error) {
	sortedArtifacts := make([]string, len(artifacts))
	copy(sortedArtifacts, artifacts)
	sort.Strings(sortedArtifacts)

	hasher := sha256.New()
	for _, artifact := range sortedArtifacts {
		path := artifact
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectRoot, path)
		}

		f, err := fileutil.Open(path)
		if err != nil {
			return "", errfmt.Newf("failed to open artifact %s", artifact).Wrap(err)
		}

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", errfmt.Newf("failed to hash artifact %s", artifact).Wrap(err)
		}
		_ = f.Close()

		fileHash := hex.EncodeToString(h.Sum(nil))
		hasher.Write([]byte(artifact + ":" + fileHash + "\n"))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func verifyBacklogItem(cmd *cobra.Command, proc *cli.Processor, obj map[string]any, objectID string) error {
	expectedHash, _ := obj[objects.FieldKeyVerificationHash].(string)
	if expectedHash == "" {
		return errfmt.Errorf("backlog_item %s has no verification_hash", objectID)
	}

	// List test cases referencing this BLI
	filter := storage.ListFilter{
		Kind: "test_case",
		Filters: map[string]any{
			objects.FieldKeyBacklogItemRefs: map[string]any{string(storage.OpHas): objectID},
		},
	}

	res, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), nil, filter)
	if err != nil {
		return errfmt.Newf("failed to list test cases").Wrap(err)
	}

	if len(res.Objects) == 0 {
		return errfmt.Errorf("backlog_item %s has no test cases", objectID)
	}

	projectRoot := zqkenv.ProjectRoot().Get()
	if projectRoot == "" {
		projectRoot = zqkenv.TestRoot().Get()
	}
	if projectRoot == "" {
		return errfmt.Errorf("project root not set")
	}

	// Compute hashes of all test cases
	tcHashes := make(map[string]string)
	var tcIDs []string
	for _, tc := range res.Objects {
		tcID, _ := tc[objects.FieldKeyID].(string)
		tcIDs = append(tcIDs, tcID)

		artifactsRaw, ok := tc[objects.FieldKeyVerifiedArtifactRefs].([]any)
		if !ok || len(artifactsRaw) == 0 {
			continue
		}
		artifacts := make([]string, 0, len(artifactsRaw))
		for _, a := range artifactsRaw {
			if str, ok := a.(string); ok {
				artifacts = append(artifacts, str)
			}
		}

		tcHash, err := computeTestCaseHash(projectRoot, artifacts)
		if err != nil {
			return errfmt.Newf("failed to compute hash for test_case %s", tcID).Wrap(err)
		}
		tcHashes[tcID] = tcHash
	}

	sort.Strings(tcIDs)
	bliHasher := sha256.New()
	for _, tcID := range tcIDs {
		bliHasher.Write([]byte(tcID + ":" + tcHashes[tcID] + "\n"))
	}

	actualHash := hex.EncodeToString(bliHasher.Sum(nil))

	if actualHash != expectedHash {
		return errfmt.Errorf("verification failed for backlog_item %s: expected hash %s, actual hash %s", objectID, expectedHash, actualHash)
	}

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Successfully verified backlog_item %s.\n", objectID)))
	return nil
}

// ExecuteVerificationTopology executes the supplied execution organizer via verification.Engine,
// supporting sequential, concurrent, and hybrid DAG dispatch topologies with panic isolation.
func ExecuteVerificationTopology(ctx context.Context, org *verification.ExecutionOrganizer) (*verification.VerificationReport, error) {
	engine := verification.NewEngine()
	return engine.Execute(ctx, org)
}

func verifyTestCaseWithOrganizer(cmd *cobra.Command, proc *cli.Processor, obj map[string]any, objectID string) error {
	topoModeStr, _ := cmd.Flags().GetString("topology")
	var mode verification.TopologyMode
	switch topoModeStr {
	case "concurrent":
		mode = verification.TopologyConcurrent
	case "hybrid_dag":
		mode = verification.TopologyHybridDAG
	default:
		mode = verification.TopologySequential
	}

	var criteriaIDs []string
	if raw, ok := obj[objects.FieldKeyCriteriaRefs].([]string); ok {
		criteriaIDs = raw
	} else if raw, ok := obj[objects.FieldKeyCriteriaRefs].([]any); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok {
				criteriaIDs = append(criteriaIDs, s)
			}
		}
	}

	stages := make([]verification.VerificationStage, 0, len(criteriaIDs))
	for _, critID := range criteriaIDs {
		cID := critID
		stage := verification.VerificationStage{
			ID:       cID,
			Category: verification.CategoryOperationalProof,
			Verify: func(ctx context.Context) error {
				critObj, err := proc.Storage().Read(ctx, proc.SecurityContext(), cID)
				if err != nil {
					return fmt.Errorf("criteria %s not found: %w", cID, err)
				}
				if simPanic, _ := critObj["simulate_panic"].(bool); simPanic {
					panic(fmt.Sprintf("simulated verification stage panic for %s", cID))
				}
				return nil
			},
		}
		stages = append(stages, stage)
	}

	org := &verification.ExecutionOrganizer{
		TestCaseID: objectID,
		Mode:       mode,
		Stages:     stages,
	}

	report, err := ExecuteVerificationTopology(proc.OperationContext(), org)
	if err != nil {
		return errfmt.Newf("verification topology execution failed for test_case %s", objectID).Wrap(err)
	}

	if report.PanicsCaught > 0 {
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Verification Organizer isolated %d stage panic(s).\n", report.PanicsCaught)))
	}

	if !report.Passed {
		return errfmt.Errorf("composite verification failed for test_case %s in mode %s", objectID, report.Mode)
	}

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Successfully verified composite organizer for test_case %s (mode: %s, stages: %d, panics_caught: %d, duration: %v).\n",
		objectID, report.Mode, len(report.StageResults), report.PanicsCaught, report.TotalDuration)))
	return nil
}

