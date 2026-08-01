package qa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
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

func (s *AuditorService) getCursorPath() string {
	return filepath.Join(".zqk", "cursors", "qa_auditor.cursor")
}

func (s *AuditorService) loadCursor() int64 {
	data, err := os.ReadFile(s.getCursorPath())
	if err != nil {
		return 0
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

func (s *AuditorService) saveCursor(seq int64) error {
	path := s.getCursorPath()
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return fileutil.WriteSecureFile(path, []byte(fmt.Sprintf("%d", seq)))
}

// Run monitors the WAL and performs audits on relevant events.
func (s *AuditorService) Run(ctx context.Context) error {
	logger := logging.NewEventLogger(ctx)
	lastSeq := s.loadCursor()

	for {
		err := s.wal.ReplayFrom(lastSeq, func(ev *lifecycle.LifecycleEvent) error {
			// Trigger audit when an object transitions to 'in_progress' or 'completed'
			if ev.EventType == lifecycle.EventTypeStatusTransition && (ev.ToStatus == "in_progress" || ev.ToStatus == "complete" || ev.ToStatus == "completed") {
				// Perform automated audit
				go s.performAudit(ctx, ev.ID, ev.Kind)
			}

			lastSeq = ev.Seq
			_ = s.saveCursor(lastSeq)
			return nil
		})

		if err != nil {
			logging.FluentEvent(logger).Error(ErrMsgServiceReplayFailed, err).Log()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// Poll
		}
	}
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

	// 1.5 Traceability Check
	status, _ := obj[objects.FieldKeyStatus].(string)
	if status == "complete" {
		tracker, hasTracker := obj["tracker"].(string)
		hasTestAsset := false
		if artifacts, ok := obj[objects.FieldKeyArtifacts].([]any); ok {
			for _, art := range artifacts {
				if path, ok := art.(string); ok && strings.HasSuffix(path, "_test.go") {
					hasTestAsset = true
					break
				}
			}
		}
		if !hasTestAsset {
			if testBundles, ok := obj["test_bundle_refs"].([]any); ok && len(testBundles) > 0 {
				hasTestAsset = true
			}
		}

		if !hasTracker || tracker == "" || !hasTestAsset {
			reason := "Missing verified test assets or traceability linkage"
			logging.FluentEvent(logger).Warn(fmt.Sprintf(LogFmtAuditorContentDisparity, id, reason)).Log()
			if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
				logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
			}
			return
		}
	}

	// 2. STRUCTURAL AST AUDIT
	// Scan artifacts listed in the object
	artifacts, ok := obj[objects.FieldKeyArtifacts].([]any)
	if !ok {
		logging.FluentEvent(logger).Info(fmt.Sprintf("Skipping AST audit: no artifacts defined for %s", id)).Log()
	}
	var astViolations []Violation
	for _, art := range artifacts {
		path, ok := art.(string)
		if !ok || !strings.HasSuffix(path, ".go") {
			continue
		}

		logging.FluentEvent(logger).Info(fmt.Sprintf(LogFmtAuditorScanAST, path)).Log()
		violations, err := s.astAuditor.AuditFile(path)
		if err != nil {
			logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorScanFailed, path, err), err).Log()
			continue
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

		if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
			logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
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
		if err := s.emitter.EmitDisparityInterrupt(ctx, id, reason); err != nil {
			logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorEmitHITLFailed, err), err).Log()
		}
		return
	}

	// 4. If PASS: Generate QAReport and emit QASuccess object
	reportData := []byte(id + "success")
	sig, err := s.signer.Sign(reportData)
	if err != nil {
		logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorSignFailed, id, err), err).Log()
		return
	}

	qaSuccess := map[string]any{
		objects.FieldKeyKind:      KindQASuccess,
		"item_id":                 id,
		objects.FieldKeyStatus:    objects.ObjectStatusCompleted,
		objects.FieldKeySignature: sig,
		objects.FieldKeyPublicKey: s.signer.PublicKey(),
	}

	if err := s.storage.Create(ctx, secCtx, qaSuccess); err != nil {
		logging.FluentEvent(logger).Error(fmt.Sprintf(LogFmtAuditorCreateSuccessFailed, id, err), err).Log()
		return
	}

	// Replay-friendly: a clean audit clears the HITL gate for this item without a manual ack loop.
	if s.emitter != nil {
		s.emitter.AckDisparityOnPass(id)
	}

	logging.FluentEvent(logger).Info(fmt.Sprintf(LogFmtAuditorSuccess, id)).Log()
}

func contains(s, substr string) bool {
	// Simple case-insensitive contains for the simulation
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
