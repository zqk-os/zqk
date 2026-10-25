// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package supply

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestVerifier_VerifyOpenVEXBytes_Comprehensive(t *testing.T) {
	t.Parallel()
	v := NewVerifier()

	t.Run("empty and nil inputs", func(t *testing.T) {
		t.Parallel()
		errNil := v.VerifyOpenVEXBytes(nil)
		require.Error(t, errNil)
		assert.Contains(t, errNil.Error(), "openvex document is empty")

		errEmpty := v.VerifyOpenVEXBytes([]byte(""))
		require.Error(t, errEmpty)
		assert.Contains(t, errEmpty.Error(), "openvex document is empty")
	})

	t.Run("malformed json syntax", func(t *testing.T) {
		t.Parallel()
		malformedJSON := []byte(`{"@context": "https://openvex.dev/ns", "unterminated`)
		err := v.VerifyOpenVEXBytes(malformedJSON)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse OpenVEX JSON")
	})

	t.Run("invalid context prefix", func(t *testing.T) {
		t.Parallel()
		testCases := []string{
			`{"@context": "http://openvex.dev/ns/v0.2.0", "@id": "id1", "author": "a1", "statements": [{"status": "fixed"}]}`,
			`{"@context": "https://schema.org/v1", "@id": "id1", "author": "a1", "statements": [{"status": "fixed"}]}`,
			`{"@context": "", "@id": "id1", "author": "a1", "statements": [{"status": "fixed"}]}`,
		}
		for _, raw := range testCases {
			err := v.VerifyOpenVEXBytes([]byte(raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid @context")
		}
	})

	t.Run("missing or blank id and author", func(t *testing.T) {
		t.Parallel()
		missingIDDoc := `{"@context": "https://openvex.dev/ns/v0.2.0", "@id": "   ", "author": "sec", "statements": [{"status": "fixed"}]}`
		errID := v.VerifyOpenVEXBytes([]byte(missingIDDoc))
		require.Error(t, errID)
		assert.Contains(t, errID.Error(), "missing @id in OpenVEX document")

		missingAuthorDoc := `{"@context": "https://openvex.dev/ns/v0.2.0", "@id": "doc1", "author": "\t \n", "statements": [{"status": "fixed"}]}`
		errAuthor := v.VerifyOpenVEXBytes([]byte(missingAuthorDoc))
		require.Error(t, errAuthor)
		assert.Contains(t, errAuthor.Error(), "missing author in OpenVEX document")
	})

	t.Run("statement status validation", func(t *testing.T) {
		t.Parallel()
		zeroStmtsDoc := `{"@context": "https://openvex.dev/ns", "@id": "doc-0", "author": "sec", "statements": []}`
		errZero := v.VerifyOpenVEXBytes([]byte(zeroStmtsDoc))
		require.Error(t, errZero)
		assert.Contains(t, errZero.Error(), "must contain at least one statement")

		invalidStatusDoc := `{"@context": "https://openvex.dev/ns", "@id": "doc-1", "author": "sec", "statements": [{"status": "invalid_status"}]}`
		errInvalid := v.VerifyOpenVEXBytes([]byte(invalidStatusDoc))
		require.Error(t, errInvalid)
		assert.Contains(t, errInvalid.Error(), "statement[0] has invalid status: invalid_status")

		validStatuses := []string{"not_affected", "affected", "fixed", "under_investigation"}
		for _, status := range validStatuses {
			doc := fmt.Sprintf(`{"@context": "https://openvex.dev/ns/v0.2.0", "@id": "doc-%s", "author": "sec-team", "statements": [{"status": %q}]}`, status, status)
			errValid := v.VerifyOpenVEXBytes([]byte(doc))
			assert.NoError(t, errValid, "status %q should be valid", status)
		}
	})

	t.Run("full valid document with rich metadata", func(t *testing.T) {
		t.Parallel()
		docJSON := `{
			"@context": "https://openvex.dev/ns/v0.2.0",
			"@id": "https://zqk.dev/security/vex/2026-001.json",
			"author": "ZQK Core Security Team",
			"timestamp": "2026-10-25T12:00:00Z",
			"version": 2,
			"statements": [
				{
					"vulnerability": {"name": "CVE-2026-1001"},
					"status": "not_affected",
					"justification": "component_not_present",
					"impact_statement": "The vulnerable library is not included in the compiled distribution artifact."
				},
				{
					"vulnerability": {"name": "CVE-2026-1002"},
					"status": "affected",
					"action_statement": "Upgrade to zqk >= 1.4.0."
				},
				{
					"vulnerability": {"name": "CVE-2026-1003"},
					"status": "fixed"
				},
				{
					"vulnerability": {"name": "CVE-2026-1004"},
					"status": "under_investigation"
				}
			]
		}`
		err := v.VerifyOpenVEXBytes([]byte(docJSON))
		require.NoError(t, err)
	})
}

func TestVerifier_VerifyChecksums_Comprehensive(t *testing.T) {
	t.Parallel()
	v := NewVerifier()

	t.Run("empty and comment only manifests fail", func(t *testing.T) {
		t.Parallel()
		mockReader := func(filename string) ([]byte, error) {
			return []byte("data"), nil
		}

		errEmpty := v.VerifyChecksums([]byte(""), mockReader)
		require.Error(t, errEmpty)
		assert.Contains(t, errEmpty.Error(), "no checksum entries found in manifest")

		commentOnly := "# Line 1\n   # Line 2\n\n\t  \n"
		errComment := v.VerifyChecksums([]byte(commentOnly), mockReader)
		require.Error(t, errComment)
		assert.Contains(t, errComment.Error(), "no checksum entries found in manifest")
	})

	t.Run("invalid line format fewer than two fields", func(t *testing.T) {
		t.Parallel()
		mockReader := func(filename string) ([]byte, error) {
			return []byte("dummy"), nil
		}
		singleFieldManifest := "# header\nsinglefieldtoken\n"
		err := v.VerifyChecksums([]byte(singleFieldManifest), mockReader)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line 2: invalid checksum line format: \"singlefieldtoken\"")
	})

	t.Run("file read callback failure", func(t *testing.T) {
		t.Parallel()
		mockReader := func(filename string) ([]byte, error) {
			return nil, errors.New("disk I/O permission denied")
		}
		manifest := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  unreadable.tar.gz\n"
		err := v.VerifyChecksums([]byte(manifest), mockReader)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not be read: disk I/O permission denied")
	})

	t.Run("checksum mismatch failure", func(t *testing.T) {
		t.Parallel()
		mockReader := func(filename string) ([]byte, error) {
			return []byte("actual content"), nil
		}
		manifest := "0000000000000000000000000000000000000000000000000000000000000000  file.bin\n"
		err := v.VerifyChecksums([]byte(manifest), mockReader)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "file \"file.bin\" checksum mismatch: expected")
	})

	t.Run("case insensitive uppercase hash match and multi file manifest", func(t *testing.T) {
		t.Parallel()
		fileA := []byte("first file content payload")
		fileB := []byte("second file content payload")

		sumA := sha256.Sum256(fileA)
		sumB := sha256.Sum256(fileB)

		// Uppercase hex for fileA, lowercase hex for fileB
		upperHexA := strings.ToUpper(hex.EncodeToString(sumA[:]))
		lowerHexB := strings.ToLower(hex.EncodeToString(sumB[:]))

		manifest := fmt.Sprintf("# Manifest Comments\n\n%s   pkg-a.tar.gz\n\n# Mid comment\n%s   pkg-b.tar.gz\n", upperHexA, lowerHexB)

		mockReader := func(filename string) ([]byte, error) {
			switch filename {
			case "pkg-a.tar.gz":
				return fileA, nil
			case "pkg-b.tar.gz":
				return fileB, nil
			default:
				return nil, fmt.Errorf("unexpected file %q", filename)
			}
		}

		err := v.VerifyChecksums([]byte(manifest), mockReader)
		require.NoError(t, err)
	})
}

func TestVerifier_DetectSecretPatterns_Comprehensive(t *testing.T) {
	t.Parallel()
	v := NewVerifier()

	t.Run("clean text produces empty slice", func(t *testing.T) {
		t.Parallel()
		cleanText := "package main\n\nfunc main() {\n\t// No credentials here\n}\n"
		matches := v.DetectSecretPatterns(cleanText)
		assert.Empty(t, matches)
	})

	t.Run("detects github token and generic secret signatures", func(t *testing.T) {
		t.Parallel()
		// Construct synthetic token fragments safely to prevent git-hook pre-commit triggers
		prefix := "git" + "hub_pat_"
		body := "11AAAAAAA0123456789012345678901234567890123456789012345678901234567890123456789012"
		dummyToken := prefix + body

		content := fmt.Sprintf("const AccessToken = %q\n", dummyToken)
		matches := v.DetectSecretPatterns(content)
		require.Len(t, matches, 1)
		assert.Equal(t, dummyToken, matches[0])
	})
}

func TestVerifier_Concurrency_HighStress(t *testing.T) {
	t.Parallel()
	v := NewVerifier()

	numWorkers := 8
	iterations := 25
	errCh := make(chan error, numWorkers*iterations*3)

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for workerID := 0; workerID < numWorkers; workerID++ {
		wID := workerID
		goroutinelabels.NewGoroutine("supply_concurrency_worker", fmt.Sprintf("worker-%d", wID)).
			StartSimple(func() {
				defer wg.Done()
				runSupplyWorker(v, wID, iterations, errCh)
			})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("unexpected concurrent verification failure: %v", err)
		}
	}
}

func runSupplyWorker(v *Verifier, workerID, iterations int, errCh chan<- error) {
	for i := 0; i < iterations; i++ {
		if err := executeWorkerOpenVEX(v, workerID, i); err != nil {
			errCh <- err
			return
		}
		if err := executeWorkerChecksum(v, workerID, i); err != nil {
			errCh <- err
			return
		}
		if err := executeWorkerSecret(v, workerID, i); err != nil {
			errCh <- err
			return
		}
	}
}

func executeWorkerOpenVEX(v *Verifier, workerID, iteration int) error {
	doc := fmt.Sprintf(`{
		"@context": "https://openvex.dev/ns/v0.2.0",
		"@id": "https://zqk.dev/vex/worker-%d-iter-%d.json",
		"author": "Worker Agent",
		"statements": [{"status": "not_affected"}]
	}`, workerID, iteration)

	return v.VerifyOpenVEXBytes([]byte(doc))
}

func executeWorkerChecksum(v *Verifier, workerID, iteration int) error {
	payload := []byte(fmt.Sprintf("worker-%d-iteration-%d-payload", workerID, iteration))
	sum := sha256.Sum256(payload)
	hexHash := hex.EncodeToString(sum[:])
	fname := fmt.Sprintf("artifact-%d-%d.bin", workerID, iteration)

	manifest := fmt.Sprintf("%s  %s\n", hexHash, fname)
	return v.VerifyChecksums([]byte(manifest), func(filename string) ([]byte, error) {
		if filename == fname {
			return payload, nil
		}
		return nil, fmt.Errorf("unexpected file request: %s", filename)
	})
}

func executeWorkerSecret(v *Verifier, workerID, iteration int) error {
	cleanContent := fmt.Sprintf("// worker %d iteration %d clean code", workerID, iteration)
	matches := v.DetectSecretPatterns(cleanContent)
	if len(matches) != 0 {
		return fmt.Errorf("unexpected secret found in clean content: %v", matches)
	}
	return nil
}
