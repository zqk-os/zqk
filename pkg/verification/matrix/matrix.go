package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckRegistry manages registered check definitions.
type CheckRegistry struct {
	mu     sync.RWMutex
	checks map[string]CheckDefinition
}

// NewCheckRegistry initializes an empty check registry.
func NewCheckRegistry() *CheckRegistry {
	return &CheckRegistry{
		checks: make(map[string]CheckDefinition),
	}
}

// Register registers a check definition.
func (r *CheckRegistry) Register(def CheckDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks[def.ID] = def
}

// Get retrieves a check definition by ID.
func (r *CheckRegistry) Get(id string) (CheckDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.checks[id]
	return def, ok
}

// All returns all registered check definitions.
func (r *CheckRegistry) All() []CheckDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]CheckDefinition, 0, len(r.checks))
	for _, c := range r.checks {
		list = append(list, c)
	}
	return list
}

// Engine coordinates the file verification matrix, content caching, and check execution.
type Engine struct {
	mu            sync.RWMutex
	repoRoot      string
	storageDir    string
	ledgerPath    string
	inventoryPath string
	classifier    *Classifier
	registry      *CheckRegistry
	ledger        *MatrixLedger
	inventory     *LiteralInventory
	remediation   RemediationHook
}

func resolveStorageAndLedgerPaths(repoRoot, storageOrLedgerPath string) (string, string) {
	if storageOrLedgerPath == "" {
		dir := filepath.Join(repoRoot, ".matrix")
		return dir, filepath.Join(dir, "verification_matrix.json")
	}
	if strings.HasSuffix(storageOrLedgerPath, ".json") {
		return filepath.Dir(storageOrLedgerPath), storageOrLedgerPath
	}
	return storageOrLedgerPath, filepath.Join(storageOrLedgerPath, "verification_matrix.json")
}

// NewEngine initializes a matrix verification engine.
// storageOrLedgerPath can be a directory (defaults to .matrix) or a path to a ledger JSON file.
func NewEngine(repoRoot string, storageOrLedgerPath string, registry *CheckRegistry) (*Engine, error) {
	if repoRoot == "" {
		repoRoot = "."
	}
	if registry == nil {
		registry = NewCheckRegistry()
	}

	storageDir, ledgerPath := resolveStorageAndLedgerPaths(repoRoot, storageOrLedgerPath)

	classifier := NewClassifier(DefaultProfile())
	invPath := filepath.Join(storageDir, "literal_inventory.json")
	inventory, err := NewLiteralInventory(invPath)
	if err != nil && !fileutil.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load literal inventory: %w", err)
	}

	e := &Engine{
		repoRoot:      repoRoot,
		storageDir:    storageDir,
		ledgerPath:    ledgerPath,
		inventoryPath: invPath,
		classifier:    classifier,
		registry:      registry,
		inventory:     inventory,
		ledger: &MatrixLedger{
			SchemaVersion: "1.0.0",
			UpdatedAt:     time.Now().UTC(),
			Files:         make(map[string]FileEntry),
			Classes:       DefaultClassConfigs(),
		},
	}

	if err := e.loadLedger(); err != nil && !fileutil.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load matrix ledger: %w", err)
	}

	return e, nil
}

// SetRemediationHook registers an external remediation hook for policy failures.
func (e *Engine) SetRemediationHook(hook RemediationHook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.remediation = hook
}

// StorageDir returns the directory where matrix artifacts and scorecards are stored.
func (e *Engine) StorageDir() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.storageDir
}

func (e *Engine) loadLedger() error {
	data, err := fileutil.ReadFile(e.ledgerPath)
	if err != nil {
		return err
	}
	var l MatrixLedger
	if err := json.Unmarshal(data, &l); err != nil {
		return err
	}
	if l.Files == nil {
		l.Files = make(map[string]FileEntry)
	}
	if l.Classes == nil {
		l.Classes = DefaultClassConfigs()
	}
	e.ledger = &l
	return nil
}

// SaveLedger writes the matrix state atomically to disk.
func (e *Engine) SaveLedger() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ledger.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(e.ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ledger: %w", err)
	}

	if err := fileutil.MkdirAll(filepath.Dir(e.ledgerPath), fileutil.StandardDirPerm); err != nil {
		return fmt.Errorf("failed to create ledger directory: %w", err)
	}

	return fileutil.WriteFile(e.ledgerPath, data, fileutil.StandardFilePerm)
}

// GetLedger returns a copy of the current in-memory ledger.
func (e *Engine) GetLedger() MatrixLedger {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return *e.ledger
}

// Summary represents the rollup state of the matrix.
type Summary struct {
	TotalFiles     int                      `json:"total_files"`
	CleanFiles     int                      `json:"clean_files"`
	ViolatingFiles int                      `json:"violating_files"`
	PendingFiles   int                      `json:"pending_files"`
	CacheHits      int                      `json:"cache_hits"`
	Evaluated      int                      `json:"evaluated"`
	ByClass        map[FileClass]ClassStats `json:"by_class"`
}

// ClassStats holds aggregate metrics for a specific file class.
type ClassStats struct {
	Total     int `json:"total"`
	Clean     int `json:"clean"`
	Violating int `json:"violating"`
	Pending   int `json:"pending"`
}

func (e *Engine) resolveTargetFile(relPath string) (fs.FileInfo, string, error) {
	fullPath := filepath.Join(e.repoRoot, relPath)
	info, err := fileutil.Stat(fullPath)
	if err != nil {
		return nil, "", fmt.Errorf("file not found: %w", err)
	}
	hash, err := ComputeFileHash(fullPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to compute file hash: %w", err)
	}
	return info, hash, nil
}

func (e *Engine) initFileEntry(relPath string, info fs.FileInfo, hash string) FileEntry {
	return FileEntry{
		Path:        relPath,
		ContentHash: hash,
		Class:       e.classifier.Classify(relPath),
		Size:        info.Size(),
		ModTime:     info.ModTime().UTC(),
		Checks:      make(map[string]CheckResult),
	}
}

func (e *Engine) isCacheSatisfied(existing FileEntry, hash string, classCfg ClassConfig, force bool) bool {
	if force || existing.ContentHash != hash {
		return false
	}
	for _, reqCheck := range classCfg.RequiredChecks {
		res, ok := existing.Checks[reqCheck]
		if !ok || res.Status != CheckStatusPassed {
			return false
		}
	}
	return true
}

func (e *Engine) executeRequiredChecks(ctx context.Context, entry *FileEntry, classCfg ClassConfig) {
	for _, checkID := range classCfg.RequiredChecks {
		checkDef, exists := e.registry.Get(checkID)
		if !exists || checkDef.Runner == nil {
			if prevRes, ok := entry.Checks[checkID]; ok && prevRes.Status == CheckStatusPassed {
				continue
			}
			entry.Checks[checkID] = CheckResult{
				CheckID:     checkID,
				Status:      CheckStatusPending,
				Evaluator:   "unassigned",
				EvaluatedAt: time.Now().UTC(),
				Feedback:    "Awaiting evaluation pass",
			}
			continue
		}

		res, err := checkDef.Runner.Run(ctx, e.repoRoot, entry)
		if err != nil {
			res = CheckResult{
				CheckID:     checkID,
				Status:      CheckStatusFailed,
				Evaluator:   "runner_error",
				EvaluatedAt: time.Now().UTC(),
				Feedback:    fmt.Sprintf("Runner execution failed: %v", err),
			}
		}
		entry.Checks[checkID] = res
	}
}

func (e *Engine) applyScorecardEvaluation(ctx context.Context, entry *FileEntry, relPath string, hash string, class FileClass) {
	allFindings := make([]Finding, 0)
	for _, res := range entry.Checks {
		allFindings = append(allFindings, res.Findings...)
	}

	sc := &FileScorecard{
		SchemaVersion: "1.0.0",
		FilePath:      relPath,
		ContentHash:   hash,
		FileClass:     class,
		EvaluatedAt:   time.Now().UTC(),
		Evaluator:     "PER-HARDCODING-ERADICATION-CZAR",
		Dimension:     DimensionHCODE,
		Findings:      allFindings,
	}

	if priorSc, loadErr := e.LoadScorecard(relPath); loadErr == nil && priorSc != nil {
		sc.PolicyEvaluation.Remediation = priorSc.PolicyEvaluation.Remediation
	}

	if eval, evalErr := e.EvaluateScorecard(ctx, sc, false); evalErr == nil && eval != nil {
		if !eval.Passed && eval.Remediation != nil {
			for k, v := range entry.Checks {
				if v.Status == CheckStatusFailed {
					v.Feedback = fmt.Sprintf("%s | Remediation: %s (Fixer: %s)", v.Feedback, eval.Remediation.TechnicalDebtID, eval.Remediation.BacklogItemID)
					entry.Checks[k] = v
				}
			}
		}
	}
}

// EvaluateFile evaluates a single file, leveraging cached results if the SHA-256 hash matches.
func (e *Engine) EvaluateFile(ctx context.Context, relPath string, force bool) (*FileEntry, bool, error) {
	info, hash, err := e.resolveTargetFile(relPath)
	if err != nil {
		return nil, false, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	class := e.classifier.Classify(relPath)
	classCfg := e.ledger.Classes[class]

	existing, found := e.ledger.Files[relPath]
	if found && e.isCacheSatisfied(existing, hash, classCfg, force) {
		return &existing, true, nil // Cache Hit!
	}

	// Cache Miss or Hash Changed: Evaluate checks
	entry := e.initFileEntry(relPath, info, hash)
	if found && existing.ContentHash == hash {
		for k, v := range existing.Checks {
			entry.Checks[k] = v
		}
	}

	e.executeRequiredChecks(ctx, &entry, classCfg)
	e.applyScorecardEvaluation(ctx, &entry, relPath, hash, class)

	e.ledger.Files[relPath] = entry
	return &entry, false, nil
}

// RecordAgentCheck allows an adversarial persona or manual reviewer to stamp an evaluation result on a file.
func (e *Engine) RecordAgentCheck(relPath string, checkID string, status CheckStatus, agentRole string, feedback string, findings []Finding) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	info, hash, err := e.resolveTargetFile(relPath)
	if err != nil {
		return err
	}

	entry, ok := e.ledger.Files[relPath]
	if !ok || entry.ContentHash != hash {
		entry = e.initFileEntry(relPath, info, hash)
	}

	if entry.Checks == nil {
		entry.Checks = make(map[string]CheckResult)
	}

	entry.Checks[checkID] = CheckResult{
		CheckID:     checkID,
		Status:      status,
		Evaluator:   agentRole,
		EvaluatedAt: time.Now().UTC(),
		Feedback:    feedback,
		Findings:    findings,
	}

	e.ledger.Files[relPath] = entry
	return nil
}

// EvaluateAll walks the repository, classifies every file, and verifies against class invariants.
func (e *Engine) EvaluateAll(ctx context.Context, force bool) (*Summary, error) {
	summary := &Summary{
		ByClass: make(map[FileClass]ClassStats),
	}

	ignoredDirs := map[string]bool{
		".git":               true,
		".matrix":            true,
		paths.ProjectDataDir: true,
		".gemini":            true,
		"vendor":             true,
		"bin":                true,
		"node_modules":       true,
		"build":              true,
		"dist":               true,
		".cache":             true,
		".venv":              true,
		"__pycache__":        true,
	}

	err := filepath.WalkDir(e.repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if ignoredDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(e.repoRoot, path)
		if err != nil {
			return err
		}

		entry, cacheHit, evalErr := e.EvaluateFile(ctx, rel, force)
		if evalErr != nil {
			return nil // Skip unreadable files or proceed
		}

		summary.TotalFiles++
		if cacheHit {
			summary.CacheHits++
		} else {
			summary.Evaluated++
		}

		// Calculate file status
		hasFailed := false
		hasPending := false
		classCfg := e.ledger.Classes[entry.Class]

		for _, reqCheck := range classCfg.RequiredChecks {
			res, ok := entry.Checks[reqCheck]
			if !ok || res.Status == CheckStatusPending {
				hasPending = true
			} else if res.Status == CheckStatusFailed {
				hasFailed = true
			}
		}

		stats := summary.ByClass[entry.Class]
		stats.Total++
		recordSummaryStatus(summary, &stats, hasFailed, hasPending)
		summary.ByClass[entry.Class] = stats

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory failed: %w", err)
	}

	return summary, nil
}

func recordSummaryStatus(summary *Summary, stats *ClassStats, hasFailed, hasPending bool) {
	switch {
	case hasFailed:
		summary.ViolatingFiles++
		stats.Violating++
	case hasPending:
		summary.PendingFiles++
		stats.Pending++
	default:
		summary.CleanFiles++
		stats.Clean++
	}
}

// Inventory returns the active literal inventory.
func (e *Engine) Inventory() *LiteralInventory {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.inventory
}

// ComputeDiamondScore calculates an objective 1-5 diamond rating from structured findings.
func ComputeDiamondScore(findings []Finding) DiamondScore {
	if len(findings) == 0 {
		return ScoreFlawless
	}

	criticals := 0
	errors := 0
	warnings := 0

	for _, f := range findings {
		sev := strings.ToLower(f.Severity)
		switch sev {
		case "critical":
			criticals++
		case "error", "high":
			errors++
		case "warning", "medium":
			warnings++
		}
	}

	if criticals > 0 {
		return ScoreCritical
	}
	if errors > 0 {
		return ScoreFailing
	}
	if warnings > 2 {
		return ScoreBacklog
	}
	return ScoreMinor
}

// StampFileCheck updates or records an evaluation check result for a file.
// Enforces that the content hash matches the actual file on disk.
func (e *Engine) StampFileCheck(ctx context.Context, req StampRequest) (*CheckResult, error) {
	if req.Path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}
	if req.CheckID == "" && req.Dimension == "" {
		return nil, fmt.Errorf("either check_id or dimension must be specified")
	}
	checkID := req.CheckID
	if checkID == "" {
		checkID = string(req.Dimension)
	}

	fullPath := filepath.Join(e.repoRoot, req.Path)
	info, err := fileutil.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("file does not exist on disk: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("target path is a directory, expected file")
	}

	currentHash, err := ComputeFileHash(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash file: %w", err)
	}

	if req.ContentHash != "" && req.ContentHash != currentHash {
		return nil, fmt.Errorf("evidence hash mismatch for %s: expected %s, file has %s", req.Path, req.ContentHash, currentHash)
	}

	// Compute DiamondScore if not provided
	diamondScore := req.DiamondScore
	if diamondScore == ScoreUnrated {
		diamondScore = ComputeDiamondScore(req.Findings)
	}

	status := req.Status
	if status == "" {
		if diamondScore >= ScoreMinor {
			status = CheckStatusPassed
		} else {
			status = CheckStatusFailed
		}
	}

	// Record any extracted string literals in the global inventory
	if e.inventory != nil {
		for _, f := range req.Findings {
			if f.LiteralValue != "" {
				count, isDup := e.inventory.Record(f.LiteralValue, req.Path, f.Line)
				if isDup && (req.Dimension == DimensionHCODE || checkID == "hardcoded-logic") {
					status = CheckStatusFailed
					diamondScore = ScoreFailing
				}
				_ = count
			}
		}
		if invErr := e.inventory.Save(); invErr != nil {
			return nil, fmt.Errorf("failed to save literal inventory: %w", invErr)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	entry, exists := e.ledger.Files[req.Path]
	if !exists || entry.ContentHash != currentHash {
		entry = e.initFileEntry(req.Path, info, currentHash)
	}
	if entry.Checks == nil {
		entry.Checks = make(map[string]CheckResult)
	}

	result := CheckResult{
		CheckID:      checkID,
		Dimension:    req.Dimension,
		Status:       status,
		DiamondScore: diamondScore,
		Evaluator:    req.Evaluator,
		EvaluatedAt:  time.Now().UTC(),
		Feedback:     req.Feedback,
		Findings:     req.Findings,
	}
	entry.Checks[checkID] = result
	e.ledger.Files[req.Path] = entry

	return &result, nil
}

// SetClassifier sets a custom classifier on the engine.
func (e *Engine) SetClassifier(c *Classifier) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.classifier = c
	if c != nil && c.Profile() != nil {
		e.ledger.Classes = c.Profile().Classes
	}
}

// Classifier returns the engine's active classifier.
func (e *Engine) Classifier() *Classifier {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.classifier
}

// Records returns all content-addressed ledger records tracking (file_path, sha256, dimension, status, stamped_by).
func (e *Engine) Records() []LedgerRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()

	records := make([]LedgerRecord, 0)
	for path, file := range e.ledger.Files {
		for _, check := range file.Checks {
			dim := check.Dimension
			if dim == "" {
				dim = DimensionCode(check.CheckID)
			}
			records = append(records, LedgerRecord{
				FilePath:  path,
				SHA256:    file.ContentHash,
				Dimension: dim,
				Status:    check.Status,
				StampedBy: check.Evaluator,
			})
		}
	}
	return records
}
