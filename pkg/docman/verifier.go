package docman

import (
	"context"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// VerificationResult summarizes the results of doc_entry cryptographic verification
type VerificationResult struct {
	TotalChecked int                             `json:"total_checked"`
	Passed       int                             `json:"passed"`
	Drifted      int                             `json:"drifted"`
	Unsealed     int                             `json:"unsealed"`
	Missing      int                             `json:"missing"`
	AutoSealed   int                             `json:"auto_sealed"`
	Violations   []storage.DocIntegrityViolation `json:"violations,omitempty"`
}

// VerifyOptions configures the verification parameters
type VerifyOptions struct {
	IDs         []string
	Subtrees    []string
	ShippedOnly bool
	AutoSeal    bool
	Strict      bool
}

// Verifier executes cryptographic verification and drift detection for doc_entries
type Verifier struct {
	storageProvider storage.ObjectStorageProvider
	projectRoot     string
}

// NewVerifier creates a new Verifier
func NewVerifier(storageProvider storage.ObjectStorageProvider, projectRoot string) *Verifier {
	return &Verifier{
		storageProvider: storageProvider,
		projectRoot:     projectRoot,
	}
}

// Verify runs cryptographic verification across matched doc_entry objects
func (v *Verifier) Verify(ctx context.Context, profile string, opts VerifyOptions) (*VerificationResult, error) {
	logger := logging.GetLoggerFromProfile(profile)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	var docObjects []map[string]any

	if len(opts.IDs) > 0 {
		for _, id := range opts.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			obj, err := v.storageProvider.Read(ctx, secCtx, id)
			if err != nil {
				return nil, errfmt.Errorf("failed to read doc_entry %s: %w", id, err)
			}
			docObjects = append(docObjects, obj)
		}
	} else {
		filter := storage.ListFilter{
			Kind: objects.KindDocEntry,
		}
		res, err := v.storageProvider.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			return nil, errfmt.Errorf("failed to list doc_entries: %w", err)
		}
		docObjects = res.Objects
	}

	// Filter by subtrees or shippedOnly if configured
	subtrees := opts.Subtrees
	if opts.ShippedOnly && len(subtrees) == 0 {
		subtrees = ShippedInitDocSubtrees
	}

	result := &VerificationResult{}

	for _, docObj := range docObjects {
		id, _ := docObj[objects.FieldKeyID].(string)
		rawPath, _ := docObj[objects.FieldKeyPath].(string)
		normRel := paths.NormalizeDocEntryPathForKey(rawPath)

		if len(subtrees) > 0 {
			matched := false
			for _, sub := range subtrees {
				subClean := strings.TrimPrefix(strings.TrimPrefix(sub, "./"), "/")
				if strings.HasPrefix(normRel, subClean) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		result.TotalChecked++

		violations := storage.ValidateDocEntryIntegrity(v.projectRoot, docObj)
		if len(violations) == 0 {
			result.Passed++
			continue
		}

		hasDrift := false
		hasUnsealed := false
		hasMissing := false

		for _, viol := range violations {
			switch viol.Code {
			case storage.DocIntegrityCodeTargetMissing, storage.DocIntegrityCodeTargetUnreadable:
				hasMissing = true
			case storage.DocIntegrityCodeUnsealedPublished:
				hasUnsealed = true
			case storage.DocIntegrityCodeHashMismatch, storage.DocIntegrityCodeSizeMismatch:
				hasDrift = true
			}
		}

		if hasMissing {
			result.Missing++
		} else if hasDrift {
			result.Drifted++
		} else if hasUnsealed {
			result.Unsealed++
		}

		// Check if auto-sealing is enabled and target is readable
		if opts.AutoSeal && !hasMissing {
			resolvedPath, err := paths.ResolveDocEntryPath(v.projectRoot, rawPath)
			if err != nil || resolvedPath == "" {
				resolvedPath = filepath.Join(v.projectRoot, normRel)
			}
			newHash, newSize, err := storage.ComputeFileIntegrity(resolvedPath)
			if err == nil {
				_, readErr := v.storageProvider.Read(ctx, secCtx, id)
				if readErr != nil {
					logging.Fluent(logger).Warn("Auto-seal read failed").ObjectID(id).WithError(readErr).Log()
				} else {
					updates := map[string]any{
						"content_hash": newHash,
						"content_size": newSize,
					}
					if updateErr := v.storageProvider.Update(ctx, secCtx, id, updates); updateErr == nil {
						result.AutoSealed++
						logging.Fluent(logger).Info("Auto-sealed doc_entry cryptographic integrity").
							ObjectID(id).
							String("content_hash", newHash).
							Int("content_size", int(newSize)).
							Log()
						// Clear violations since it was auto-sealed
						violations = nil
						result.Passed++
					} else {
						logging.Fluent(logger).Warn("Auto-seal update failed").ObjectID(id).WithError(updateErr).Log()
					}
				}
			}
		}

		if len(violations) > 0 {
			result.Violations = append(result.Violations, violations...)
		}
	}

	return result, nil
}
