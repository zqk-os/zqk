package lifecycle

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

// ApplyComputeHooks applies any dynamic field computations based on status transitions.
func ApplyComputeHooks(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, req TransitionRequest, updates map[string]any) {
	if req.Kind == objects.KindBacklogItem && req.ToStatus == statusComplete {
		obj, err := provider.Read(ctx, secCtx, req.ID)
		if err == nil {
			var estStr, actStr string
			if v, ok := updates[objects.FieldKeyEstimatedEffort]; ok {
				estStr = fmt.Sprintf("%v", v)
			} else if v, ok := obj[objects.FieldKeyEstimatedEffort]; ok {
				estStr = fmt.Sprintf("%v", v)
			}

			if v, ok := updates[objects.FieldKeyActualEffort]; ok {
				actStr = fmt.Sprintf("%v", v)
			} else if v, ok := obj[objects.FieldKeyActualEffort]; ok {
				actStr = fmt.Sprintf("%v", v)
			}

			// actual_effort is membrane-autofilled at complete (backlog_item_lifecycle).
			// Graph persist can leave a numeric 0 default; treat that as unset, not -100% variance.
			if effortStringUnset(actStr) {
				if estStr != "" {
					actStr = estStr
				} else {
					actStr = "0.0"
				}
				updates[objects.FieldKeyActualEffort] = actStr
			}

			variance := metrics.ComputeEffortVariance(estStr, actStr)
			updates[objects.FieldKeyEffortVariance] = variance
		}
	}

	// Detect unplanned work (backlog items unlinked from milestones/plans) upon lifecycle transitions
	if req.Kind == objects.KindBacklogItem {
		obj, err := provider.Read(ctx, secCtx, req.ID)
		if err == nil {
			hasMilestone := false
			if refsRaw, ok := obj[objects.FieldKeyMilestoneRefs]; ok && refsRaw != nil {
				if refs, ok := refsRaw.([]any); ok && len(refs) > 0 {
					hasMilestone = true
				} else if refsStr, ok := refsRaw.([]string); ok && len(refsStr) > 0 {
					hasMilestone = true
				}
			}

			hasPlan := false
			if planRef, ok := obj[objects.FieldKeyPriorityPlanRef].(string); ok && planRef != "" {
				hasPlan = true
			}

			if !hasMilestone && !hasPlan {
				// Flag as unplanned work via logging and updates
				goroutinelabels.NewGoroutine("unplanned_work_detector", "detect unlinked backlog items").StartSimple(func() {
					logger := logging.NewEventLogger(context.Background())
					logging.FluentEvent(logger).Debug("ZQK Unplanned Work Detection: Unlinked backlog item detected").
						Kind(req.Kind).
						ObjectID(req.ID).
						String("reason", "No milestone or priority plan associated").
						Log()

					// Tag object as unplanned if not already tagged
					var tags []any
					if tagsRaw, ok := obj[objects.FieldKeyTags]; ok && tagsRaw != nil {
						if t, ok := tagsRaw.([]any); ok {
							tags = t
						}
					}

					hasUnplannedTag := false
					for _, tag := range tags {
						if tStr, ok := tag.(string); ok && tStr == "unplanned_work" {
							hasUnplannedTag = true
							break
						}
					}

					if !hasUnplannedTag {
						tags = append(tags, "unplanned_work")
						errUpdate := provider.Update(context.Background(), secCtx, req.ID, map[string]any{
							objects.FieldKeyTags: tags,
						})
						if errUpdate != nil {
							logging.FluentEvent(logger).Debug("ZQK Unplanned Work Detection: failed to add unplanned_work tag").
								WithError(errUpdate).
								Log()
						}
					}
				})
			}
		}
	}

	if req.ToStatus == "validation_requested" {
		goroutinelabels.NewGoroutine("qa_dispatcher", "summon QA auditor").StartSimple(func() {
			logging.FluentEvent(logging.NewEventLogger(context.Background())).Debug("SCH-qa-dispatcher: QA Auditor summoned").
				Kind(req.Kind).
				ObjectID(req.ID).
				String("reason", "validation_requested").
				Log()
		})
	}

	if req.ToStatus == "verification_ready" {
		goroutinelabels.NewGoroutine("verification_daemon", "system verification").StartSimple(func() {
			logger := logging.NewEventLogger(context.Background())
			logging.FluentEvent(logger).Debug("ZQK System Verification Daemon: starting verification").
				Kind(req.Kind).
				ObjectID(req.ID).
				Log()

			if req.Kind == objects.KindTestCase {
				var artifacts []string
				obj, err := provider.Read(context.Background(), secCtx, req.ID)
				if err != nil {
					logging.FluentEvent(logger).Debug("ZQK System Verification Daemon: failed to read object").
						WithError(err).
						Log()
				} else if path, ok := obj[objects.FieldKeyPathOrID].(string); ok && path != "" {
					artifacts = append(artifacts, path)
				}

				err = testkit.SignTestCaseCompletion(context.Background(), req.ID, artifacts)
				if err != nil {
					logging.FluentEvent(logger).Debug("ZQK System Verification Daemon: verification/signing failed").
						WithError(err).
						Log()
					errUpdate := provider.Update(context.Background(), secCtx, req.ID, map[string]any{
						objects.FieldKeyStatus: "error",
					})
					if errUpdate != nil {
						logging.FluentEvent(logger).Debug("ZQK System Verification Daemon: failed to update status to error").
							WithError(errUpdate).
							Log()
					}
				} else {
					logging.FluentEvent(logger).Debug("ZQK System Verification Daemon: cryptographic seal applied").
						Kind(req.Kind).
						ObjectID(req.ID).
						Log()
				}
			} else {
				errUpdate := provider.Update(context.Background(), secCtx, req.ID, map[string]any{
					objects.FieldKeyStatus: "complete",
				})
				if errUpdate != nil {
					logging.FluentEvent(logger).Debug("ZQK System Verification Daemon: failed to update status to complete").
						WithError(errUpdate).
						Log()
				}
			}
		})
	}

	if req.Kind == objects.KindPriorityPlan && req.ToStatus == statusComplete {
		goroutinelabels.NewGoroutine("active_order_promoter", "promote active_order on PRI complete").StartSimple(func() {
			logger := logging.NewEventLogger(context.Background())
			ctx := pkgctx.NewSystemContext() // must use valid system context

			filter := storage.ListFilter{
				Kind: objects.KindPriorityPlan,
				Filters: map[string]any{
					objects.FieldKeyStatus: statusActive,
				},
			}
			result, err := provider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
			if err != nil {
				logging.FluentEvent(logger).Debug("ZQK Active Order Promoter: list failed").WithError(err).Log()
				return
			}
			
			for _, obj := range result.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id == "" {
					continue
				}
				
				var order int64
				switch v := obj["active_order"].(type) {
				case int:
					order = int64(v)
				case int64:
					order = v
				case float64:
					order = int64(v)
				default:
					continue
				}

				if order > 1 {
					errUpdate := provider.Update(ctx, secCtx, id, map[string]any{
						"active_order": order - 1,
					})
					if errUpdate != nil {
						logging.FluentEvent(logger).Debug("ZQK Active Order Promoter: update failed").
							String("target_id", id).
							WithError(errUpdate).Log()
					}
				}
			}
		})
	}
}

// effortStringUnset reports whether actual/estimated effort is missing or a numeric-zero default.
// Persisted 0 must not block membrane autofill at complete (variance would become -100).
func effortStringUnset(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	var n float64
	if _, err := fmt.Sscanf(s, "%f", &n); err != nil {
		return false
	}
	return n == 0
}
