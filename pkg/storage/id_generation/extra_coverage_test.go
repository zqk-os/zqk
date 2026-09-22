package id_generation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestExtraCoverage_UUIDStrategy(t *testing.T) {
	s1 := NewUUIDStrategy()
	if name := s1.Name(); name != "uuid" {
		t.Errorf("expected name uuid, got %s", name)
	}
	if desc := s1.Description(); desc == "" {
		t.Error("expected non-empty description")
	}

	// With params: length
	s2 := NewUUIDStrategyWithParams(map[string]any{"length": 16})
	if s2.length != 16 {
		t.Errorf("expected length 16, got %d", s2.length)
	}
	if desc := s2.Description(); desc == "" {
		t.Error("expected non-empty description for length 16")
	}

	// With params: short
	s3 := NewUUIDStrategyWithParams(map[string]any{"short": true})
	if s3.length != 8 {
		t.Errorf("expected length 8, got %d", s3.length)
	}

	// GenerateNextID error with empty prefix
	ctx := context.Background()
	if _, err := s1.GenerateNextID(ctx, "item", "", nil); err == nil {
		t.Error("expected error for empty prefix")
	}

	// GenerateNextID success
	id, err := s1.GenerateNextID(ctx, "item", "ITEM-", []string{"ITEM-abc12345"})
	if err != nil {
		t.Fatalf("GenerateNextID: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}
}

func TestExtraCoverage_SequentialStrategy(t *testing.T) {
	s1 := NewSequentialStrategy()
	if name := s1.Name(); name != "sequential" {
		t.Errorf("expected name sequential, got %s", name)
	}
	if desc := s1.Description(); desc == "" {
		t.Error("expected non-empty description")
	}

	// With params
	s2 := NewSequentialStrategyWithParams(map[string]any{"min_digits": 4, "start_at": 10})
	if s2.minDigits != 4 || s2.startAt != 10 {
		t.Errorf("expected minDigits=4 startAt=10, got %d, %d", s2.minDigits, s2.startAt)
	}

	ctx := context.Background()
	// Error on empty prefix
	if _, err := s1.GenerateNextID(ctx, "item", "", nil); err == nil {
		t.Error("expected error for empty prefix")
	}

	// Legacy existingIDs fallback
	id, err := s1.GenerateNextID(ctx, "item", "BLI-", []string{"BLI-001", "BLI-002"})
	if err != nil {
		t.Fatalf("GenerateNextID: %v", err)
	}
	if id != "BLI-003" {
		t.Errorf("expected BLI-003, got %s", id)
	}

	// Context with kindDir
	tmpDir := t.TempDir()
	ctxWithDir := context.WithValue(ctx, "kindDir", tmpDir)
	idFromDir, err := s1.GenerateNextID(ctxWithDir, "item", "BLI-", nil)
	if err != nil {
		t.Fatalf("GenerateNextID with kindDir: %v", err)
	}
	if idFromDir == "" {
		t.Error("expected non-empty ID with kindDir")
	}
}

func TestExtraCoverage_StrategyConfigAndRegistry(t *testing.T) {
	cfg := DefaultStrategyConfig()
	if cfg.Strategy != "sequential" {
		t.Errorf("expected sequential strategy, got %s", cfg.Strategy)
	}

	def := GetDefaultStrategy()
	if def == nil || def.Name() != "sequential" {
		t.Errorf("expected default strategy sequential, got %v", def)
	}

	list := ListStrategies()
	if len(list) == 0 {
		t.Error("expected non-empty list of strategies")
	}

	if nonExistent := GetStrategy("nonexistent_strategy"); nonExistent != nil {
		t.Errorf("expected nil for nonexistent strategy, got %v", nonExistent)
	}
}

func TestExtraCoverage_GeneratorPoolAndVolumeTracker(t *testing.T) {
	globalPool := GetGlobalGeneratorPool()
	if globalPool == nil {
		t.Fatal("expected non-nil global pool")
	}

	pool := NewGeneratorPool(5, 20, 100)
	stats := pool.GetPoolStats()
	if stats.MaxPoolSize != 20 || stats.HighVolumeThreshold != 100 {
		t.Errorf("unexpected pool stats: %+v", stats)
	}

	vt := NewVolumeTracker()
	if vol := vt.GetVolume("nonexistent"); vol != 0 {
		t.Errorf("expected volume 0 for nonexistent, got %d", vol)
	}
	vt.Record("key1", 42)
	if vol := vt.GetVolume("key1"); vol != 42 {
		t.Errorf("expected volume 42, got %d", vol)
	}
	// Record exceeding period size to test modulo reset
	vt.Record("key1", 15000)

	ctx := context.Background()
	tmpDir := t.TempDir()
	gen, isDedicated := pool.GetOrCreateGenerator(ctx, tmpDir, "backlog_item", "BLI-", 3, 1)
	if gen == nil {
		t.Fatal("expected non-nil generator")
	}
	_ = isDedicated

	// Record high volume to cross threshold and get dedicated generator
	pool.RecordGeneration(tmpDir, "high_vol", "HV-", 200)
	genDed, isDed := pool.GetOrCreateGenerator(ctx, tmpDir, "high_vol", "HV-", 3, 1)
	if genDed == nil || !isDed {
		t.Errorf("expected dedicated generator for high-volume kind, got %v, isDed=%v", genDed, isDed)
	}

	// Optimal pool size calculations
	if size := CalculateOptimalPoolSize(20, 5, 50); size != 15 {
		t.Errorf("expected 15, got %d", size)
	}
	if size := CalculateOptimalPoolSize(5, 10, 50); size != 10 { // clamps to minSize 10
		t.Errorf("expected minSize 10, got %d", size)
	}
	if size := CalculateOptimalPoolSize(100, 10, 50); size != 50 { // clamps to maxPoolSize 50
		t.Errorf("expected maxPoolSize 50, got %d", size)
	}
}

func TestExtraCoverage_IDQueueAndQueueManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := newQueueManager(ctx, cancel)
	if name := qm.GetName(); name == "" {
		t.Error("expected non-empty queue manager name")
	}
	if qm.IsCritical() {
		t.Error("expected IsCritical false")
	}
	if !qm.IsDrained() {
		t.Error("expected initially drained")
	}
	if count := qm.GetPendingCount(); count != 0 {
		t.Errorf("expected 0 pending, got %d", count)
	}

	tmpDir := t.TempDir()
	q := qm.GetOrCreateQueue(tmpDir, "task", "TSK-", 3, 1, 10)
	if q == nil {
		t.Fatal("expected non-nil queue")
	}

	qStats := q.GetStats()
	if qStats.MaxSize < 10 {
		t.Errorf("expected maxSize >= 10, got %d", qStats.MaxSize)
	}

	_ = qm.hasQueuesNeedingRefill()

	qmStats := qm.GetQueueStats()
	if len(qmStats) == 0 {
		t.Error("expected non-empty queue manager stats")
	}

	// Drain
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer drainCancel()
	_ = qm.Drain(drainCtx)

	_ = qm.InitiateShutdown()

	// Global queue manager
	globalQM := GetGlobalQueueManager(context.Background())
	if globalQM == nil {
		t.Fatal("expected non-nil global queue manager")
	}
}

func TestExtraCoverage_BatchIDGeneratorHelpers(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Direct generator
	gen1 := GetBatchIDGenerator(ctx, tmpDir, "doc", "DOC-", 3, 1)
	if gen1 == nil {
		t.Fatal("expected non-nil generator from GetBatchIDGenerator")
	}
	gen1.SetSequenceFileDir(tmpDir)
	id1, err := gen1.GenerateNextID()
	if err != nil {
		t.Fatalf("GenerateNextID: %v", err)
	}
	if id1 == "" {
		t.Error("expected non-empty ID")
	}
	_ = gen1.GetLastSequence()

	// GenerateBatchIDs with batchSize <= 0 and > 0 (sequence file branch)
	batchZero, err := gen1.GenerateBatchIDs(0)
	if err != nil || len(batchZero) != 1 {
		t.Errorf("expected 1 ID for batchSize 0, got %d, err %v", len(batchZero), err)
	}
	batchThree, err := gen1.GenerateBatchIDs(3)
	if err != nil || len(batchThree) != 3 {
		t.Errorf("expected 3 IDs for batchSize 3, got %d, err %v", len(batchThree), err)
	}

	// GenerateBatchIDs with empty sequenceFileDir (exercises scanDirectory and direct generation)
	genScan := NewBatchIDGenerator(ctx, tmpDir, "scan_kind", "SCN-", 3, 1)
	genScan.SetSequenceFileDir("")
	idsScan, err := genScan.GenerateBatchIDs(2)
	if err != nil || len(idsScan) != 2 {
		t.Errorf("expected 2 scan IDs, got %d, err %v", len(idsScan), err)
	}
	// Call again to exercise cachedMaxSequence branch
	idsScan2, err := genScan.GenerateBatchIDs(2)
	if err != nil || len(idsScan2) != 2 {
		t.Errorf("expected 2 more scan IDs, got %d, err %v", len(idsScan2), err)
	}

	// ensurePattern test
	genNoPattern := &BatchIDGenerator{prefix: "BLI"}
	genNoPattern.ensurePattern()
	if genNoPattern.seqPattern == nil {
		t.Error("expected seqPattern initialized")
	}
	// already initialized fast return
	genNoPattern.ensurePattern()

	// Generator with CAS
	genCAS := GetBatchIDGeneratorWithCAS(ctx, tmpDir, "doc", "DOC-", 3, 1, nil)
	if genCAS == nil {
		t.Fatal("expected non-nil generator from GetBatchIDGeneratorWithCAS")
	}

	// Generator with Buffer
	genBuf := GetBatchIDGeneratorWithBuffer(ctx, tmpDir, "doc_buffered", "DOCB-", 3, 1, 5)
	if genBuf == nil {
		t.Fatal("expected non-nil generator from GetBatchIDGeneratorWithBuffer")
	}
	idBuf, err := genBuf.GenerateNextID()
	if err != nil {
		t.Fatalf("genBuf.GenerateNextID: %v", err)
	}
	if idBuf == "" {
		t.Error("expected non-empty buffered ID")
	}

	// Legacy generator
	genLegacy := GetBatchIDGeneratorLegacy(ctx, tmpDir, "doc_legacy", "DOCL-", 3, 1)
	if genLegacy == nil {
		t.Fatal("expected non-nil generator from GetBatchIDGeneratorLegacy")
	}
}

func TestExtraCoverage_Generator(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bliDir := filepath.Join(tmpDir, paths.ProjectDataDir, "process", "backlog_items")
	_ = fileutil.MkdirAll(bliDir, paths.DirPerm755)

	gen := NewGenerator(validation.NewIDValidator(""), tmpDir)

	// Sequential strategy
	idSeq, err := gen.GenerateNextID(ctx, "backlog_item", StrategyConfig{Strategy: "sequential"})
	if err != nil {
		t.Fatalf("GenerateNextID sequential: %v", err)
	}
	if idSeq == "" {
		t.Error("expected non-empty sequential ID")
	}

	// UUID strategy
	idUUID, err := gen.GenerateNextID(ctx, "backlog_item", StrategyConfig{
		Strategy: "uuid",
		Params:   map[string]any{"length": 8},
	})
	if err != nil {
		t.Fatalf("GenerateNextID uuid: %v", err)
	}
	if idUUID == "" {
		t.Error("expected non-empty UUID ID")
	}

	// Invalid kind
	if _, err := gen.GenerateNextID(ctx, "nonexistent_kind", StrategyConfig{}); err == nil {
		t.Error("expected error for nonexistent kind")
	}

	// Unknown strategy
	if _, err := gen.GenerateNextID(ctx, "backlog_item", StrategyConfig{Strategy: "unknown_strat"}); err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestExtraCoverage_RetryableReadDirError(t *testing.T) {
	if isRetryableReadDirError(nil) {
		t.Error("expected false for nil error")
	}
	if !isRetryableReadDirError(errors.New("i/o timeout")) {
		t.Error("expected true for timeout error")
	}
	if !isRetryableReadDirError(errors.New("context deadline exceeded")) {
		t.Error("expected true for deadline error")
	}
	if !isRetryableReadDirError(errors.New("temporary failure")) {
		t.Error("expected true for temporary error")
	}
	if isRetryableReadDirError(errors.New("permission denied")) {
		t.Error("expected false for permission denied")
	}
	if isRetryableReadDirError(errors.New("file not found")) {
		t.Error("expected false for not found")
	}
	if isRetryableReadDirError(errors.New("generic fatal error")) {
		t.Error("expected false for generic error")
	}
}
