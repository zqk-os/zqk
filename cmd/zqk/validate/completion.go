package validate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
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

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewCompletionCommandBuilder(), &cobra.Command{
		Use:   "completion <object-id>",
		Short: "Verify cryptographic completion",
		Args:  cobra.ExactArgs(1),
	})

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
