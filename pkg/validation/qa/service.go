package qa

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/walutil"
)

// Cap concurrent performAudit workers. Catch-up from a reset IDE watermark previously
// spawned one unbounded `go` per historical in_progress/complete transition; audits now run on
// a bounded, labeled [goroutinelabels.Pool] (no raw goroutines — POL-CODE concurrency policy).
const (
	maxConcurrentAuditorAudits = 8
	auditorPoolName            = "qa_auditor_audit"
	auditorPoolPurpose         = "performing QA audits for lifecycle status transitions"
	// Queue depth so WAL replay can hand off a burst of historical transitions without blocking.
	auditorPoolQueueSize = 64
)

// AuditorService monitors the lifecycle WAL and performs automated QA audits.
type AuditorService struct {
	astAuditor *ASTAuditor
	storage    storage.ObjectStorageProvider
	wal        *lifecycle.LifecycleEventWAL
	signer     Signer
	emitter    *InterruptEmitter
	engine     *GuidanceEngine
	gate       Gate
}

func NewAuditorService(wal *lifecycle.LifecycleEventWAL, s storage.ObjectStorageProvider, signer Signer, emitter *InterruptEmitter, engine *GuidanceEngine, gate Gate) *AuditorService {
	return &AuditorService{
		astAuditor: NewASTAuditor(),
		storage:    s,
		wal:        wal,
		signer:     signer,
		emitter:    emitter,
		engine:     engine,
		gate:       gate,
	}
}

func (s *AuditorService) projectRoot() string {
	if s.emitter != nil && s.emitter.ProjectRoot() != "" {
		return s.emitter.ProjectRoot()
	}
	if g, ok := s.gate.(*AuditorGate); ok && g.projectRoot != "" {
		return g.projectRoot
	}
	return ""
}

func (s *AuditorService) getIDEPath() string {
	return filepath.Join(paths.ProjectDataDir, paths.IdesSubdir, "qa_auditor.ide")
}

func (s *AuditorService) loadIDE() walutil.ReplayCursor {
	data, err := fileutil.ReadFile(s.getIDEPath())
	if err != nil {
		return walutil.ReplayCursor{}
	}
	cursor, err := walutil.ParseReplayCursorCheckpoint(data)
	if err != nil {
		return walutil.ReplayCursor{}
	}
	return cursor
}

func (s *AuditorService) saveIDE(cursor walutil.ReplayCursor) error {
	path := s.getIDEPath()
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return fileutil.WriteSecureFile(path, walutil.FormatReplayCursorCheckpoint(cursor))
}

var auditTriggeringStatuses = map[string]bool{
	"in_progress": true,
	"complete":    true,
	"completed":   true,
}

var auditTriggeringKinds = map[string]bool{
	objects.KindBacklogItem: true,
	objects.KindAgentTask:   true,
	objects.KindRequirement: true,
	objects.KindCriteria:    true,
}

// isAuditTriggeringEvent reports whether a WAL event is a status transition into a state
// that should schedule a QA audit.
func isAuditTriggeringEvent(ev *lifecycle.LifecycleEvent) bool {
	return ev != nil &&
		ev.EventType == lifecycle.EventTypeStatusTransition &&
		auditTriggeringStatuses[ev.ToStatus] &&
		(auditTriggeringKinds[ev.Kind] || ev.Kind == "")
}

// Run monitors the WAL and performs audits on relevant events.
func (s *AuditorService) Run(ctx context.Context) error {
	logger := logging.NewEventLogger(ctx)
	cursor := s.loadIDE()

	auditPool := goroutinelabels.NewPool(
		goroutinelabels.DefaultBudget(),
		auditorPoolName, auditorPoolPurpose,
		maxConcurrentAuditorAudits, auditorPoolQueueSize,
	)
	auditPool.Start(ctx)
	defer auditPool.Stop()

	poll := lifecycle.WALIdlePollMin
	for {
		var delivered int
		next, err := s.wal.ReplayFromCursor(cursor, func(ev *lifecycle.LifecycleEvent) error {
			delivered++
			if isAuditTriggeringEvent(ev) {
				id, kind := ev.ID, ev.Kind
				if submitErr := auditPool.Submit(ctx, func(auditCtx context.Context) error {
					s.performAudit(auditCtx, id, kind)
					return nil
				}); submitErr != nil {
					return submitErr
				}
			}
			return nil
		})

		if err != nil {
			logging.FluentEvent(logger).Error(ErrMsgServiceReplayFailed, err).Log()
		} else {
			cursor = next
			_ = s.saveIDE(cursor)
		}

		if delivered > 0 {
			poll = lifecycle.WALIdlePollMin
		} else {
			poll = lifecycle.NextWALIdlePoll(poll)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

// AuditNow runs one QA audit for id (same path as the WAL watcher).
func (s *AuditorService) AuditNow(ctx context.Context, id, kind string) {
	s.performAudit(ctx, id, kind)
}

func (s *AuditorService) performAudit(ctx context.Context, id string, kind string) {
	logger := logging.NewEventLogger(ctx)
	logging.FluentEvent(logger).Info(fmt.Sprintf(LogFmtAuditorStart, kind, id)).Log()

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Read the object
	obj, err := s.storage.Read(ctx, secCtx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no such file") {
			logging.FluentEvent(logger).Info(fmt.Sprintf("Skipping audit for deleted object %s", id)).Log()
			return
		}
		logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorReadFailed, id, err), err).Log()
		return
	}

	status, _ := obj[objects.FieldKeyStatus].(string)
	isComplete := status == objects.ObjectStatusComplete || status == objects.ObjectStatusCompleted || status == "complete" || status == "completed"
	artifactPaths := ExtractObjectArtifacts(obj)

	// 1.3 Requirement Criteria Verification
	if kind == objects.KindRequirement && isComplete {
		critRefs := extractArtifactPaths(obj[objects.FieldKeyCriteriaRefs])
		if len(critRefs) == 0 {
			reason := ReasonMissingCriteria
			logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
			if s.emitter != nil {
				if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
					logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
				}
			}
			return
		}
		for _, cID := range critRefs {
			criterion, err := s.storage.Read(ctx, secCtx, cID)
			if err != nil {
				reason := fmt.Sprintf("Referenced criterion %s not found: %v", cID, err)
				logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
				if s.emitter != nil {
					if emitErr := s.emitter.EmitDisparityInterrupt(ctx, id, reason); emitErr != nil {
						logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, emitErr), emitErr).Log()
					}
				}
				return
			}
			cStatus, _ := criterion[objects.FieldKeyStatus].(string)
			if cStatus != objects.ObjectStatusComplete && cStatus != objects.ObjectStatusCompleted && cStatus != "complete" && cStatus != "completed" {
				reason := fmt.Sprintf("Referenced criterion %s is not complete (status=%s)", cID, cStatus)
				logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
				if s.emitter != nil {
					if emitErr := s.emitter.EmitDisparityInterrupt(ctx, id, reason); emitErr != nil {
						logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, emitErr), emitErr).Log()
					}
				}
				return
			}
		}
		logging.FluentEvent(logger).Info(fmt.Sprintf("🔍 [QA-AUDITOR] Verified all %d referenced criteria for requirement %s", len(critRefs), id)).Log()
	}

	// 1.4 Criteria Test Proof Verification
	if kind == objects.KindCriteria && isComplete {
		hasTestProof := false
		testRefs := extractArtifactPaths(obj[objects.FieldKeyTestCaseRefs])
		for _, tID := range testRefs {
			tc, err := s.storage.Read(ctx, secCtx, tID)
			if err == nil && tc != nil {
				tStatus, _ := tc[objects.FieldKeyStatus].(string)
				remOpen, _ := tc["remaining_open_count"].(int)
				if (tStatus == objects.ObjectStatusComplete || tStatus == objects.ObjectStatusCompleted || tStatus == "complete" || tStatus == "completed") && remOpen == 0 {
					hasTestProof = true
					break
				}
			}
		}
		if !hasTestProof {
			// Also inspect test_cases referencing this criteria in storage
			filter := storage.ListFilter{
				Kind: objects.KindTestCase,
			}
			if listRes, err := s.storage.List(ctx, secCtx, nil, filter); err == nil && listRes != nil {
				for _, tc := range listRes.Objects {
					tcCritRefs := extractArtifactPaths(tc[objects.FieldKeyCriteriaRefs])
					match := false
					for _, r := range tcCritRefs {
						if r == id {
							match = true
							break
						}
					}
					if match {
						tStatus, _ := tc[objects.FieldKeyStatus].(string)
						remOpen, _ := tc["remaining_open_count"].(int)
						if (tStatus == objects.ObjectStatusComplete || tStatus == objects.ObjectStatusCompleted || tStatus == "complete" || tStatus == "completed") && remOpen == 0 {
							hasTestProof = true
							break
						}
					}
				}
			}
		}
		if !hasTestProof {
			reason := ReasonMissingTestProof
			logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
			if s.emitter != nil {
				if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
					logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
				}
			}
			return
		}
		logging.FluentEvent(logger).Info(fmt.Sprintf("🔍 [QA-AUDITOR] Verified passing test case proof for criterion %s", id)).Log()
	}

	// 1.5 Traceability Check
	if kind == objects.KindBacklogItem && isComplete {
		hasTraceability := hasStringEvidence(obj[objects.FieldKeyCommitHashes])
		hasTestAsset := false
		for _, path := range artifactPaths {
			if strings.HasSuffix(path, "_test.go") {
				hasTestAsset = true
				break
			}
		}
		if !hasTestAsset {
			hasTestAsset = s.hasCompletedCriterion(ctx, secCtx, obj[objects.FieldKeyCriteriaRefs])
		}

		if !hasTraceability && !hasTestAsset {
			reason := "Missing verified test assets or traceability linkage"
			logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
			if s.emitter != nil {
				if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
					logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
				}
			}
			return
		}
	}

	// 1.6 Deliverable Artifacts Validation
	// Fail-closed: completed objects must pass unified deliverable artifact validation.
	if isComplete {
		verified, err := ValidateDeliverableArtifacts(obj, s.projectRoot())
		if err != nil {
			reason := err.Error()
			logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
			if s.emitter != nil {
				if emitErr := s.emitter.EmitDisparityInterrupt(ctx, id, reason); emitErr != nil {
					logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, emitErr), emitErr).Log()
				}
			}
			return
		}
		if len(verified) > 0 {
			artifactPaths = verified
		}
	}

	// 2. STRUCTURAL AST AUDIT
	if len(artifactPaths) == 0 {
		if kind == objects.KindRequirement || kind == objects.KindCriteria {
			// Requirements and criteria have ontological verification above; AST file scan is not applicable.
		} else {
			// Fail-closed invariant: NO DATA != NO FAILURES.
			// If an object has no artifacts to AST-audit, and is not a requirement or criteria verified by ontology,
			// the auditor cannot attest to its conformance and MUST NOT issue a vacuous QASuccess token.
			logging.FluentEvent(logger).Info(fmt.Sprintf("Skipping QASuccess issuance: no verifiable artifacts or ontological criteria defined for %s (%s)", id, kind)).Log()
			return
		}
	}
	var astViolations []Violation
	for _, path := range artifactPaths {
		if !strings.HasSuffix(path, ".go") {
			continue
		}

		logging.FluentEvent(logger).Info(fmt.Sprintf(LogFmtAuditorScanAST, path)).Log()
		violations, err := s.astAuditor.AuditFile(path)
		if err != nil {
			logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorScanFailed, path, err), err).Log()
			reason := fmt.Sprintf("AST scan failed for %s: %v", path, err)
			logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorASTDisparity, id, reason)).Log()
			if s.emitter != nil {
				if emitErr := s.emitter.EmitDisparityInterrupt(ctx, id, reason); emitErr != nil {
					logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, emitErr), emitErr).Log()
				}
			}
			return
		}
		astViolations = append(astViolations, violations...)
	}

	if len(astViolations) > 0 {
		reason := fmt.Sprintf(LogFmtAuditorASTViolation, len(astViolations))
		logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorASTDisparity, id, reason)).Log()

		// Provide guidance for the first violation
		v := astViolations[0]
		g := s.engine.Recommend(v.Type, fmt.Sprintf("%s:%d:%d - %s", v.Pos.Filename, v.Pos.Line, v.Pos.Column, v.Message))
		logging.FluentEvent(logger).Info(fmt.Sprintf(LogFmtAuditorGuidance, g.Summary)).Log()
		for _, step := range g.Steps {
			logging.FluentEvent(logger).Info(fmt.Sprintf("   - %s\n", step)).Log()
		}

		if s.emitter != nil {
			if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
				logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
			}
		}
		return
	}

	// 3. SMOKE-AND-MIRRORS DETECTION (Existing logic)
	title, ok := obj[objects.FieldKeyTitle].(string)
	if !ok {
		logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorMissingTitle, id)).Log()
	}
	description, ok := obj[objects.FieldKeyDescription].(string)
	if !ok {
		logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorMissingDesc, id)).Log()
	}

	hasSmoke := (title != "" && (contains(title, "smoke") || contains(title, "mirrors")))
	hasTODO := (description != "" && contains(description, "TODO"))

	if hasSmoke || hasTODO {
		reason := LogFmtAuditorSmokeAndMirrors
		logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
		if s.emitter != nil {
			if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
				logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
			}
		}
		return
	}

	// Invariant: QASuccess is never minted during in_progress lifecycle status transitions.
	if status == objects.ObjectStatusInProgress || status == "in_progress" {
		logging.FluentEvent(logger).Info(fmt.Sprintf("Skipping QASuccess issuance: status is in_progress for %s", id)).Log()
		return
	}

	// 4. If PASS: Generate QAReport and emit QASuccess object
	reportData := []byte(id + "success")
	sig, err := s.signer.Sign(reportData)
	if err != nil {
		logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorSignFailed, id, err), err).Log()
		return
	}

	qaSuccess, err := buildQASuccessObject(id, sig, s.signer.PublicKey())
	if err != nil {
		logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorBuildSuccessFailed, id, err), err).Log()
		return
	}

	if err := s.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, qaSuccess); err != nil {
		logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorCreateSuccessFailed, id, err), err).Log()
		return
	}

	// Replay-friendly: a clean audit clears the HITL gate for this item without a manual ack loop.
	if s.emitter != nil {
		s.emitter.AckDisparityOnPass(id)
	}

	logging.FluentEvent(logger).Info(fmt.Sprintf(LogFmtAuditorSuccess, id)).Log()
}



func hasStringEvidence(value any) bool {
	switch refs := value.(type) {
	case []any:
		for _, ref := range refs {
			if text, ok := ref.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	case []string:
		for _, ref := range refs {
			if strings.TrimSpace(ref) != "" {
				return true
			}
		}
	}
	return false
}

func (s *AuditorService) hasCompletedCriterion(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	value any,
) bool {
	var refs []string
	switch values := value.(type) {
	case []any:
		for _, candidate := range values {
			if ref, ok := candidate.(string); ok && strings.TrimSpace(ref) != "" {
				refs = append(refs, ref)
			}
		}
	case []string:
		refs = values
	}

	for _, ref := range refs {
		criterion, err := s.storage.Read(ctx, secCtx, ref)
		if err != nil {
			continue
		}
		criterionStatus, _ := criterion[objects.FieldKeyStatus].(string)
		if criterionStatus == objects.ObjectStatusComplete || criterionStatus == objects.ObjectStatusCompleted {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	// Simple case-insensitive contains for the simulation
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
