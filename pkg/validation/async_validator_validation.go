package validation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// validateObject performs validation on a single object
func (av *AsyncValidator) validateObject(ctx context.Context, objectID, objectKind, filePath string) (*ValidationState, error) {
	// Read file once
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf(ConstMagic9713ffa0).Wrap(err)
	}

	// Get validation function (acquire lock, read, release lock)
	var validationFunc ValidationFunc
	if err := concurrency.RunInRLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorValidateObjectGetFunc,
		lockLoggerSystem(),
		func() error {
			validationFunc = av.validationFunc
			return nil
		},
	); err != nil {
		logging.
			// Log and continue with nil validationFunc (fallback logic will handle it)
			Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstMagic316746f4, err).Log()
	}

	return av.validateObjectWithFunc(ctx, objectID, objectKind, filePath, data, validationFunc)
}

// validateObjectWithData performs validation using pre-read file data
// This avoids reading the file multiple times
//
//nolint:unparam // objectKind always receives "test_object" in current test usage, but kept for API consistency
func (av *AsyncValidator) validateObjectWithData(ctx context.Context, objectID, objectKind, filePath string, data []byte) (*ValidationState, error) {
	// Get validation function (acquire lock, read, release lock)
	var validationFunc ValidationFunc
	if err := concurrency.RunInRLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorValidateObjectWithDataGetFunc,
		lockLoggerSystem(),
		func() error {
			validationFunc = av.validationFunc
			return nil
		},
	); err != nil {
		logging.
			// Log and continue with nil validationFunc (fallback logic will handle it)
			Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstMagic2ed20aec, err).Log()
	}

	return av.validateObjectWithFunc(ctx, objectID, objectKind, filePath, data, validationFunc)
}

// validateObjectWithFunc performs validation using pre-read file data and a validation function
// FIXED: Validation function is passed in (no lock acquisition during validation)
func (av *AsyncValidator) validateObjectWithFunc(ctx context.Context, objectID, objectKind, filePath string, data []byte, validationFunc ValidationFunc) (*ValidationState, error) {
	// Compute checksum from data
	checksum := av.computeChecksumFromData(data)

	if validationFunc != nil {
		// Use the provided validation function (real validation logic)
		state, err := validationFunc(ctx, objectID, objectKind, filePath, data)
		if err != nil {
			return nil, errfmt.Newf(ConstMagic38f59d5e).Wrap(err)
		}
		// Ensure checksum is set
		if state.Checksum == emptyValue {
			state.Checksum = checksum
		}
		return state, nil
	}

	// Fallback: create a basic validation state (placeholder)
	// This should only happen if validation function is not set
	state := &ValidationState{
		ObjectID:      objectID,
		ObjectKind:    objectKind,
		FilePath:      filePath,
		LastValidated: time.Now(),
		Checksum:      checksum,
		Issues:        []ValidationIssue{},
		Metadata:      make(map[string]string),
	}

	return state, nil
}

// computeChecksum computes SHA256 checksum of a file
func (av *AsyncValidator) computeChecksum(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return av.computeChecksumFromData(data)
}

// computeChecksumFromData computes SHA256 checksum of data
func (av *AsyncValidator) computeChecksumFromData(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
