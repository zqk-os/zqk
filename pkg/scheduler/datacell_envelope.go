package scheduler

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// DataCellEnvelopeDryRunReport is the result of DryRunDataCellEnvelopePolicy — proves the policy engine
// accepts jobs tagged with CategoryDataCellEnvelope without persisting a scheduler_job object.
type DataCellEnvelopeDryRunReport struct {
	SchedulerCategory string `json:"scheduler_category"`
	StorageProfile    string `json:"storage_profile"`
	PolicyAction      string `json:"policy_action"`
	PolicyReason      string `json:"policy_reason"`
	EnvelopeSummary   string `json:"envelope_compact_summary"`
}

// DryRunDataCellEnvelopePolicy runs PolicyEngine.Evaluate on a synthetic job that uses CategoryDataCellEnvelope.
// v1 wiring proof for data-cell operational envelope; does not enqueue work.
func DryRunDataCellEnvelopePolicy(projectRoot string, profile datacell.StorageProfile) (*DataCellEnvelopeDryRunReport, error) {
	env, ok := datacell.OperationalEnvelopeForProfile(profile)
	if !ok {
		return nil, errfmt.Errorf("operational envelope: unknown storage profile %q", profile)
	}
	reg := NewJobStateRegistry(projectRoot)
	pe := NewPolicyEngine(reg, projectRoot, nil)
	const jobID = "SCH-envelope-policy-dryrun"
	job := &ScheduledJob{
		ID:       jobID,
		JobType:  JobTypeRunWrapper,
		Category: CategoryDataCellEnvelope,
	}
	dec, err := pe.Evaluate(jobID, job)
	if err != nil {
		return nil, err
	}
	if dec == nil {
		return nil, errfmt.Errorf("policy engine returned nil decision")
	}
	return &DataCellEnvelopeDryRunReport{
		SchedulerCategory: CategoryDataCellEnvelope,
		StorageProfile:    string(profile),
		PolicyAction:      dec.Action,
		PolicyReason:      dec.Reason,
		EnvelopeSummary:   env.CompactSummary(),
	}, nil
}

// DryRunDataCellEnvelopePoliciesForKnownProfiles runs DryRunDataCellEnvelopePolicy for each
// entry in datacell.KnownStorageProfiles (stable order: cas_entity, light_file, stream).
func DryRunDataCellEnvelopePoliciesForKnownProfiles(projectRoot string) ([]*DataCellEnvelopeDryRunReport, error) {
	out := make([]*DataCellEnvelopeDryRunReport, 0, len(datacell.KnownStorageProfiles))
	for _, p := range datacell.KnownStorageProfiles {
		rep, err := DryRunDataCellEnvelopePolicy(projectRoot, p)
		if err != nil {
			return out, err
		}
		out = append(out, rep)
	}
	return out, nil
}
