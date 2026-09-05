package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sort"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/quality"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

type AutoTransitionHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

func NewAutoTransitionHandler(storage storagepkg.ObjectStorageProvider) JobHandler {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	return &AutoTransitionHandler{
		storage: storage,
		logger:  logger,
	}
}

func (h *AutoTransitionHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info("auto_transition_job_start").JobID(job.ID).Log()

	eventData := ctx.Value(evtDataKey{})
	if eventData == nil {
		SLog(h.logger).Warn("no_event_data_for_auto_transition").Log()
		return nil
	}

	eventDataMap, ok := eventData.(map[string]any)
	if !ok {
		return errfmt.Errorf("invalid event data format")
	}

	status, _ := eventDataMap[objects.FieldKeyStatus].(string)
	if status != "complete" && status != "passing" {
		return nil
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	var parentRefs []string
	for _, refKey := range []string{"backlog_item_refs", "requirement_refs"} {
		if raw, ok := eventDataMap[refKey]; ok && raw != nil {
			switch arr := raw.(type) {
			case []any:
				for _, r := range arr {
					if s, ok := r.(string); ok && s != "" {
						parentRefs = append(parentRefs, s)
					}
				}
			case []string:
				for _, s := range arr {
					if s != "" {
						parentRefs = append(parentRefs, s)
					}
				}
			}
		}
	}

	var errs []error
	for _, parentID := range parentRefs {
		parentObj, err := h.storage.Read(ctx, secCtx, parentID)
		if err != nil {
			SLog(h.logger).Error("failed_read_parent", err).ParentID(parentID).Log()
			errs = append(errs, errfmt.Errorf("failed to read parent %s: %w", parentID, err))
			continue
		}

		parentStatus, _ := parentObj[objects.FieldKeyStatus].(string)
		parentKind, _ := parentObj[objects.FieldKeyKind].(string)
		if isTerminal, _ := objects.GetGlobalLifecycleLoader().IsTerminalStatusForKind(parentKind, parentStatus); isTerminal {
			continue
		}
		refKeyForParent := "backlog_item_refs"
		if parentKind == "requirement" {
			refKeyForParent = "requirement_refs"
		}

		filter := storagepkg.ListFilter{
			Kind: "test_case",
			Filters: map[string]any{
				refKeyForParent: map[string]any{string(storagepkg.OpHas): parentID},
			},
		}

		res, err := h.storage.List(ctx, secCtx, nil, filter)
		if err != nil {
			SLog(h.logger).Error("failed_list_children", err).ParentID(parentID).Log()
			errs = append(errs, errfmt.Errorf("failed to list children for %s: %w", parentID, err))
			continue
		}

		if len(res.Objects) == 0 {
			continue
		}

		allComplete := true
		var hashes []string
		for _, tc := range res.Objects {
			tcStatus, _ := tc[objects.FieldKeyStatus].(string)
			if tcStatus != "complete" && tcStatus != "passing" {
				allComplete = false
				break
			}
			if hash, ok := tc[objects.FieldKeyVerificationHash].(string); ok && hash != "" {
				hashes = append(hashes, tc[objects.FieldKeyID].(string)+":"+hash)
			}
		}

		if allComplete {
			SLog(h.logger).Info("auto_transitioning_parent").ParentID(parentID).Log()
			parentObj[objects.FieldKeyStatus] = "complete"

			if len(hashes) > 0 {
				sort.Strings(hashes)
				hasher := sha256.New()
				for _, hashStr := range hashes {
					if _, err := hasher.Write([]byte(hashStr + "\n")); err != nil {
						SLog(h.logger).Error("hashing_error", err).Log()
						errs = append(errs, errfmt.Errorf("hashing error: %w", err))
					}
				}
				hashVal := hex.EncodeToString(hasher.Sum(nil))
				parentObj[objects.FieldKeyVerificationHash] = hashVal

				if parentKind == "requirement" {
					projectRoot := "."
					if getter, ok := h.storage.(interface{ GetProjectRoot() string }); ok {
						projectRoot = getter.GetProjectRoot()
					}
					csvPath := filepath.Join(projectRoot, "docs/quality/features_traceability.csv")
					profilePath := filepath.Join(projectRoot, "docs/quality/features_traceability_matrix_profile.yaml")

					if _, err1 := fileutil.Stat(csvPath); err1 == nil {
						if _, err2 := fileutil.Stat(profilePath); err2 == nil {
							matrixName := "features_traceability"
							matchColumn := "Requirement ID"
							matchValue := parentID
							setPairs := []string{"Delivered=Yes", "Verification Hash=" + hashVal}
							_, updateErr := quality.UpdateMatrixCSVRow(csvPath, profilePath, matrixName, matchColumn, matchValue, setPairs, nil, false, nil)
							if updateErr != nil {
								SLog(h.logger).Error("matrix_update_error", updateErr).Log()
								errs = append(errs, errfmt.Errorf("failed to update matrix: %w", updateErr))
							}
						}
					}
				}
			}

			err = h.storage.Update(ctx, secCtx, parentID, parentObj)
			if err != nil {
				SLog(h.logger).Error("failed_to_auto_transition", err).ParentID(parentID).Log()
				errs = append(errs, errfmt.Errorf("failed to auto transition %s: %w", parentID, err))
			}
		}
	}

	return errors.Join(errs...)
}
