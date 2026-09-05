package testkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// SignTestCaseCompletion computes SHA-256 hashes of the supplied artifacts/files
// and updates the test_case object's verification_hash and status to complete.
func SignTestCaseCompletion(ctx context.Context, testCaseID string, artifacts []string) error {
	// Prefer TestRoot under tests so inherited ZQK_PROJECT_ROOT (Local CI worktree) cannot
	// resolve artifacts against the wrong tree. TRACK: TDE-1785808957221945000-fcd15e47.
	projectRoot := os.Getenv(zqkenv.ProjectRoot())
	if testRoot := os.Getenv(zqkenv.TestRoot()); testRoot != "" && (zqkenv.IsInTest() || projectRoot == "") {
		projectRoot = testRoot
	}
	if projectRoot == "" {
		return fmt.Errorf("project root not set")
	}

	// Ensure stable sorting for hash verification
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
			return fmt.Errorf("failed to open artifact %s: %w", artifact, err)
		}

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return fmt.Errorf("failed to hash artifact %s: %w", artifact, err)
		}
		f.Close()

		fileHash := hex.EncodeToString(h.Sum(nil))

		// Add to combined hash
		if _, err := hasher.Write([]byte(artifact + ":" + fileHash + "\n")); err != nil {
			return fmt.Errorf("failed to write to combined hash: %w", err)
		}
	}

	finalHash := hex.EncodeToString(hasher.Sum(nil))

	// Update the test_case object
	storeFactory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return fmt.Errorf("failed to create storage factory: %w", err)
	}
	defer func() {
		_ = storeFactory.Shutdown(ctx)
	}()

	store := storeFactory.GetStorageForKind("test_case")
	secCtx := pkgctx.NewSystemSecurityContext()

	updates := map[string]any{
		objects.FieldKeyStatus:               objects.ObjectStatusComplete,
		objects.FieldKeyVerifiedArtifactRefs: artifacts,
		objects.FieldKeyVerificationHash:     finalHash,
	}

	err = store.Update(ctx, secCtx, testCaseID, updates)
	if err != nil {
		return fmt.Errorf("failed to update test_case %s: %w", testCaseID, err)
	}

	return nil
}
