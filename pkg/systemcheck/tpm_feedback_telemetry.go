package systemcheck

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// TPMFeedbackTelemetry aggregates kernel hygiene anomalies, unverified criteria,
// and process gaps upward into actionable TPM refinement initiatives.
type TPMFeedbackTelemetry struct {
	MeasuredAt           time.Time          `json:"measured_at"`
	UnverifiedCriteria   int                `json:"unverified_criteria"`
	StalledBacklogItems  int                `json:"stalled_backlog_items"`
	HygieneAnomalies     int                `json:"hygiene_anomalies"`
	RefinementActions    []string           `json:"refinement_actions"`
	RankedActionClusters []TPMActionCluster `json:"ranked_action_clusters"`
}

// TPMActionCluster records an aggregated action category and its occurrence count.
type TPMActionCluster struct {
	Action string `json:"action"`
	Count  int    `json:"count"`
}

// CollectTPMFeedbackTelemetry scans storage for unverified criteria and process gaps
// and compiles them into a structured telemetry report for metrics_rollup.
func CollectTPMFeedbackTelemetry(ctx context.Context, sp storagepkg.ObjectStorageProvider) (*TPMFeedbackTelemetry, error) {
	if sp == nil {
		return nil, fmt.Errorf("storage provider is required")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	telemetry := &TPMFeedbackTelemetry{
		MeasuredAt:           time.Now().UTC(),
		RefinementActions:    make([]string, 0),
		RankedActionClusters: make([]TPMActionCluster, 0),
	}

	// 1. Scan criteria
	critRes, err := sp.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindCriteria,
	})
	if err == nil && critRes != nil {
		for _, obj := range critRes.Objects {
			status, _ := obj[objects.FieldKeyStatus].(string)
			if status != objects.ObjectStatusComplete && status != "archived" {
				telemetry.UnverifiedCriteria++
			}
		}
	}

	// 2. Scan backlog items
	bliRes, err := sp.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if err == nil && bliRes != nil {
		for _, obj := range bliRes.Objects {
			status, _ := obj[objects.FieldKeyStatus].(string)
			if status == objects.ObjectStatusInProgress {
				telemetry.StalledBacklogItems++
			}
			desc, _ := obj[objects.FieldKeyDescription].(string)
			if desc == "" {
				telemetry.HygieneAnomalies++
			}
		}
	}

	// 3. Compile refinement initiatives
	if telemetry.UnverifiedCriteria > 0 {
		action := fmt.Sprintf("tpm-refine-unverified-criteria: %d criteria awaiting verification or test runner catalyst", telemetry.UnverifiedCriteria)
		telemetry.RefinementActions = append(telemetry.RefinementActions, action)
		telemetry.RankedActionClusters = append(telemetry.RankedActionClusters, TPMActionCluster{
			Action: "unverified_criteria",
			Count:  telemetry.UnverifiedCriteria,
		})
	}

	if telemetry.StalledBacklogItems > 0 {
		action := fmt.Sprintf("tpm-sweep-in-progress-blis: %d in_progress items needing test verification or convergence", telemetry.StalledBacklogItems)
		telemetry.RefinementActions = append(telemetry.RefinementActions, action)
		telemetry.RankedActionClusters = append(telemetry.RankedActionClusters, TPMActionCluster{
			Action: "stalled_backlog_items",
			Count:  telemetry.StalledBacklogItems,
		})
	}

	if telemetry.HygieneAnomalies > 0 {
		action := fmt.Sprintf("tpm-remediate-hygiene-anomalies: %d items with incomplete description/metadata", telemetry.HygieneAnomalies)
		telemetry.RefinementActions = append(telemetry.RefinementActions, action)
		telemetry.RankedActionClusters = append(telemetry.RankedActionClusters, TPMActionCluster{
			Action: "hygiene_anomalies",
			Count:  telemetry.HygieneAnomalies,
		})
	}

	return telemetry, nil
}
