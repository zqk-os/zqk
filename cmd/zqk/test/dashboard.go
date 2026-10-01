package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/accumulator"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/walutil"
)

// LineageNode represents a node in the upward/downward traceability chain.
type LineageNode struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Title  string `json:"title,omitempty"`
}

// LineageChain models the lineage from the Root Object to the Test Case.
// Invariant: A traceability chain is considered intact when it displays the full lineage from the test case to the root object.
type LineageChain struct {
	RootObject   *LineageNode   `json:"root_object,omitempty"`
	Requirements []*LineageNode `json:"requirements,omitempty"`
	BacklogItems []*LineageNode `json:"backlog_items,omitempty"`
	Milestones   []*LineageNode `json:"milestones,omitempty"`
	IsIntact     bool           `json:"is_intact"`
	BrokenReason string         `json:"broken_reason,omitempty"`
}

// UnboundCriterionModel represents a test-related criterion not currently linked to any test_case.
type UnboundCriterionModel struct {
	ID               string        `json:"id"`
	Title            string        `json:"title"`
	Status           string        `json:"status"`
	Category         string        `json:"category"`
	ValidationMethod string        `json:"validation_method,omitempty"`
	Lineage          *LineageChain `json:"lineage,omitempty"`
}

// CriterionState tracks in-memory state of a single criterion.
type CriterionState struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	Description   string    `json:"description,omitempty"`
	LastSatisfied time.Time `json:"last_satisfied,omitempty"`
}

// TestCaseModel represents a test case and its linked criteria for the dashboard.
type TestCaseModel struct {
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Status             string            `json:"status"`
	PathOrID           string            `json:"path_or_id,omitempty"`
	Scope              string            `json:"scope,omitempty"`
	Category           string            `json:"category,omitempty"`
	Lineage            *LineageChain     `json:"lineage,omitempty"`
	RequirementRefs    []string          `json:"requirement_refs,omitempty"`
	BacklogItemRefs    []string          `json:"backlog_item_refs,omitempty"`
	GoalRefs           []string          `json:"goal_refs,omitempty"`
	Criteria           []*CriterionState `json:"criteria"`
	RemainingOpenCount int               `json:"remaining_open_count"`
	TotalCriteria      int               `json:"total_criteria"`
	CompletedCriteria  int               `json:"completed_criteria"`
}

// LifecycleEventSummary records recent criteria satisfied events for the live ticker.
type LifecycleEventSummary struct {
	Timestamp   time.Time         `json:"timestamp"`
	EventType   string            `json:"event_type"`
	CriterionID string            `json:"criterion_id,omitempty"`
	ObjectID    string            `json:"object_id,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Scope       map[string]string `json:"scope,omitempty"`
	Message     string            `json:"message"`
}

// DashboardState holds the complete state of the test dashboard.
type DashboardState struct {
	mu                  sync.RWMutex
	projectRoot         string
	TestCases           map[string]*TestCaseModel
	TestCaseOrder       []string
	CriteriaIndex       map[string][]*TestCaseModel // maps criterionID -> referencing test cases
	UnboundTestCriteria []*UnboundCriterionModel
	RecentEvents        []LifecycleEventSummary
	LastUpdated         time.Time
	engine              *accumulator.Engine[*DashboardLitePayload]
}

// NewDashboardState initializes an empty DashboardState.
func NewDashboardState() *DashboardState {
	return &DashboardState{
		TestCases:           make(map[string]*TestCaseModel),
		CriteriaIndex:       make(map[string][]*TestCaseModel),
		UnboundTestCriteria: make([]*UnboundCriterionModel, 0),
		RecentEvents:        make([]LifecycleEventSummary, 0, 20),
		LastUpdated:         time.Now(),
	}
}

// NewDashboardStateWithProjectRoot initializes a DashboardState attached to a project root and accumulator engine.
func NewDashboardStateWithProjectRoot(projectRoot string) *DashboardState {
	state := NewDashboardState()
	state.projectRoot = projectRoot
	state.EnsureEngine(projectRoot)
	if projectRoot != "" {
		_, _ = state.LoadFromLiteFile(projectRoot)
	}
	return state
}

// StartBackgroundWALSubscriber helper to launch SubscribeWAL with proper goroutine tracking.
func (s *DashboardState) StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{}) {
	s.mu.Lock()
	projectRoot := s.projectRoot
	s.mu.Unlock()
	if projectRoot != "" {
		s.EnsureEngine(projectRoot)
		s.mu.RLock()
		empty := len(s.TestCases) == 0
		s.mu.RUnlock()
		if empty {
			loaded, _ := s.LoadFromLiteFile(projectRoot)
			if !loaded {
				if factory, err := storage.NewStorageFactory(ctx, projectRoot); err == nil && factory != nil {
					_ = s.ScanFromStorage(ctx, factory.GetStorage())
				}
			}
		}
	}
	s.mu.Lock()
	eng := s.engine
	s.mu.Unlock()
	if eng != nil {
		eng.StartBackgroundWALSubscriber(ctx, updateCh)
		return
	}

	goroutinelabels.NewGoroutine("test-dashboard-wal-subscriber", "subscribing to test dashboard lifecycle events").
		StartSimple(func() {
			s.SubscribeWAL(ctx, projectRoot, updateCh)
		})
}

func init() {
	accumulator.RegisterSubscriber("test_dashboard", func(projectRoot string) accumulator.WALSubscriber {
		return NewDashboardStateWithProjectRoot(projectRoot)
	})
}

// EnsureEngine initializes or returns the attached generic accumulator engine.
func (s *DashboardState) EnsureEngine(projectRoot string) *accumulator.Engine[*DashboardLitePayload] {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		s.projectRoot = projectRoot
		spec := accumulator.AccumulatorSpec{
			Name:               "test_dashboard",
			SchemaVersion:      "1.0.0",
			ProjectRoot:        projectRoot,
			StoragePath:        LiteFilePath(projectRoot),
			StalenessTolerance: 2 * time.Minute,
		}
		s.engine, _ = accumulator.NewEngine[*DashboardLitePayload](spec, s)
	}
	return s.engine
}

// Name satisfies accumulator.Accumulator interface.
func (s *DashboardState) Name() string {
	return "test_dashboard"
}

// DefaultPayload satisfies accumulator.Accumulator interface.
func (s *DashboardState) DefaultPayload() *DashboardLitePayload {
	return &DashboardLitePayload{
		SchemaVersion:       "1.0.0",
		MaterializedAt:      time.Now().UTC(),
		TestCases:           make(map[string]*TestCaseModel),
		TestCaseOrder:       make([]string, 0),
		UnboundTestCriteria: make([]*UnboundCriterionModel, 0),
		RecentEvents:        make([]LifecycleEventSummary, 0),
	}
}

// ApplyEvent satisfies accumulator.Accumulator interface.
func (s *DashboardState) ApplyEvent(ev *lifecycle.LifecycleEvent) bool {
	s.HandleLifecycleEvent(ev)
	return true
}

// BuildPayload satisfies accumulator.Accumulator interface.
func (s *DashboardState) BuildPayload() *DashboardLitePayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buildPayloadLocked()
}

// ScanFromStorage satisfies accumulator.Accumulator interface.
func (s *DashboardState) ScanFromStorage(ctx context.Context, sp storage.ObjectStorageProvider) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	return s.ScanFromStorageWithSecurity(ctx, sp, secCtx)
}

// ScanFromStorageWithSecurity performs a full load from storage using the provided security context.
func (s *DashboardState) ScanFromStorageWithSecurity(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *storage.SecurityContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	return s.scanFromStorageLocked(ctx, sp, secCtx, s.projectRoot, true, "", "")
}

// WarmLiteFile builds and persists the materialized test dashboard lite file from storage.
func WarmLiteFile(ctx context.Context, projectRoot string, sp storage.ObjectStorageProvider) error {
	state := NewDashboardStateWithProjectRoot(projectRoot)
	if err := state.ScanFromStorage(ctx, sp); err != nil {
		return err
	}
	return state.SaveToLiteFile(projectRoot)
}

// Engine returns the attached accumulator engine if initialized.
func (s *DashboardState) Engine() *accumulator.Engine[*DashboardLitePayload] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine
}

func (s *DashboardState) applyPayloadLocked(payload *DashboardLitePayload) {
	if payload == nil {
		return
	}
	s.TestCases = payload.TestCases
	if s.TestCases == nil {
		s.TestCases = make(map[string]*TestCaseModel)
	}
	s.TestCaseOrder = payload.TestCaseOrder
	if s.TestCaseOrder == nil {
		s.TestCaseOrder = make([]string, 0)
	}
	// Guarantee every test case in s.TestCases is in s.TestCaseOrder
	orderSet := make(map[string]bool, len(s.TestCaseOrder))
	for _, id := range s.TestCaseOrder {
		orderSet[id] = true
	}
	for id := range s.TestCases {
		if !orderSet[id] {
			s.TestCaseOrder = append(s.TestCaseOrder, id)
			orderSet[id] = true
		}
	}
	s.UnboundTestCriteria = payload.UnboundTestCriteria
	if s.UnboundTestCriteria == nil {
		s.UnboundTestCriteria = make([]*UnboundCriterionModel, 0)
	}
	s.RecentEvents = payload.RecentEvents
	s.LastUpdated = payload.MaterializedAt

	// Rebuild in-memory lookup index
	s.CriteriaIndex = make(map[string][]*TestCaseModel)
	for _, tc := range s.TestCases {
		for _, cr := range tc.Criteria {
			s.CriteriaIndex[cr.ID] = append(s.CriteriaIndex[cr.ID], tc)
		}
	}
}

// DashboardLitePayload defines the materialized, zero-cost JSON projection for the test dashboard.
type DashboardLitePayload struct {
	SchemaVersion       string                    `json:"schema_version"`
	MaterializedAt      time.Time                 `json:"materialized_at"`
	TotalTestCases      int                       `json:"total_test_cases"`
	InFlightCount       int                       `json:"in_flight_count"`
	RegressionCount     int                       `json:"regression_count"`
	SatisfiedCriteria   int                       `json:"satisfied_criteria"`
	TotalCriteria       int                       `json:"total_criteria"`
	IntactChains        int                       `json:"intact_chains"`
	TestCases           map[string]*TestCaseModel `json:"test_cases"`
	TestCaseOrder       []string                  `json:"test_case_order"`
	UnboundTestCriteria []*UnboundCriterionModel  `json:"unbound_test_criteria,omitempty"`
	RecentEvents        []LifecycleEventSummary   `json:"recent_events,omitempty"`
}

// LiteFilePath returns the canonical on-disk path for the materialized test dashboard lite-file.
func LiteFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "test_dashboard_lite.json")
}

// LoadFromLiteFile loads the materialized dashboard projection from disk without hitting the storage layer.
func (s *DashboardState) LoadFromLiteFile(projectRoot string) (bool, error) {
	eng := s.EnsureEngine(projectRoot)
	env, err := eng.LoadFromLiteFile()
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if env == nil || env.Payload == nil {
		return false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyPayloadLocked(env.Payload)
	return true, nil
}

func (s *DashboardState) buildPayloadLocked() *DashboardLitePayload {
	// Guarantee all test cases in s.TestCases are in s.TestCaseOrder
	orderSet := make(map[string]bool, len(s.TestCaseOrder))
	for _, id := range s.TestCaseOrder {
		orderSet[id] = true
	}
	for id := range s.TestCases {
		if !orderSet[id] {
			s.TestCaseOrder = append(s.TestCaseOrder, id)
			orderSet[id] = true
		}
	}

	payload := &DashboardLitePayload{
		SchemaVersion:       "1.0.0",
		MaterializedAt:      s.LastUpdated,
		TestCases:           s.TestCases,
		TestCaseOrder:       s.TestCaseOrder,
		UnboundTestCriteria: s.UnboundTestCriteria,
		RecentEvents:        s.RecentEvents,
	}

	intactCount := 0
	regressionCount := 0
	satCrit := 0
	totCrit := 0
	for _, tcID := range s.TestCaseOrder {
		tc := s.TestCases[tcID]
		if tc == nil {
			continue
		}
		if tc.Lineage != nil && tc.Lineage.IsIntact {
			intactCount++
		}
		if tc.Status == objects.ObjectStatusComplete && tc.RemainingOpenCount == 0 && (tc.Lineage == nil || tc.Lineage.IsIntact) {
			regressionCount++
		}
		totCrit += tc.TotalCriteria
		satCrit += tc.CompletedCriteria
	}
	payload.TotalTestCases = len(s.TestCaseOrder)
	payload.InFlightCount = payload.TotalTestCases - regressionCount
	payload.RegressionCount = regressionCount
	payload.IntactChains = intactCount
	payload.TotalCriteria = totCrit
	payload.SatisfiedCriteria = satCrit
	return payload
}

// SaveToLiteFile atomically persists the current dashboard state as a materialized lite-file via accumulator Engine.
func (s *DashboardState) SaveToLiteFile(projectRoot string) error {
	eng := s.EnsureEngine(projectRoot)
	eng.SetLastUpdated(s.LastUpdated)
	return eng.SaveToLiteFile()
}

// ValidateTPMDefinitionOfDone verifies that all non-preliminary, active test cases have unbroken lineage to root objects
// and that zero unbound active test criteria exist in the test matrix.
func (s *DashboardState) ValidateTPMDefinitionOfDone(out, errOut ioWriter) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sc := objects.GetGlobalStatusChecker()
	var brokenList []string
	checkedCount := 0
	intactCount := 0

	for _, tcID := range s.TestCaseOrder {
		tc := s.TestCases[tcID]
		if tc == nil || tc.Status == objects.ObjectStatusArchived || tc.Status == objects.ObjectStatusConceptual || tc.Status == objects.ObjectStatusDraft || sc.IsPreliminary(objects.KindTestCase, tc.Status) {
			continue
		}
		if tc.Lineage != nil && tc.Lineage.IsIntact {
			checkedCount++
			intactCount++
		} else {
			// If test case appears broken in lite-file projection, verify if it actually exists in storage.
			// Orphaned test cases deleted from storage or ghost events must not fail the active DoD gate.
			if s.projectRoot != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if factory, err := storage.NewStorageFactory(ctx, s.projectRoot); err == nil && factory != nil {
					sp := factory.GetStorage()
					secCtx := pkgctx.NewSystemSecurityContext()
					if obj, err := sp.Read(ctx, secCtx, tc.ID); err != nil || obj == nil {
						cancel()
						continue
					}
				}
				cancel()
			}
			checkedCount++
			reason := "incomplete lineage"
			if tc.Lineage != nil && tc.Lineage.BrokenReason != "" {
				reason = tc.Lineage.BrokenReason
			}
			brokenList = append(brokenList, fmt.Sprintf("%s [%s] (%s): %s", tc.ID, tc.Status, tc.Title, reason))
		}
	}

	var unboundList []string
	for _, uc := range s.UnboundTestCriteria {
		if uc == nil || uc.Status == objects.ObjectStatusArchived || uc.Status == objects.ObjectStatusConceptual || uc.Status == objects.ObjectStatusDraft || sc.IsPreliminary(objects.KindCriteria, uc.Status) {
			continue
		}
		unboundList = append(unboundList, fmt.Sprintf("%s [%s] (%s): category=%s, method=%s", uc.ID, uc.Status, uc.Title, uc.Category, uc.ValidationMethod))
	}

	if len(brokenList) > 0 || len(unboundList) > 0 {
		if len(brokenList) > 0 {
			_, _ = fmt.Fprintf(errOut, "❌ TPM Definition of Done FAILED: %d of %d test case chain(s) are incomplete or missing root objects:\n", len(brokenList), checkedCount)
			for _, b := range brokenList {
				_, _ = fmt.Fprintf(errOut, "   - %s\n", b)
			}
		}
		if len(unboundList) > 0 {
			_, _ = fmt.Fprintf(errOut, "❌ TPM Definition of Done FAILED: %d unbound test criteria detected (missing test_case binding):\n", len(unboundList))
			for _, u := range unboundList {
				_, _ = fmt.Fprintf(errOut, "   - %s\n", u)
			}
		}
		_, _ = fmt.Fprintf(errOut, "%s", paths.RewriteCanonicalCLIInvocations("\nTip: Run 'zqk test bind' to see recommended parent bindings and repair broken chains.\n"))
		return fmt.Errorf("traceability DoD check failed: %d broken lineage chains, %d unbound test criteria", len(brokenList), len(unboundList))
	}

	_, _ = fmt.Fprintf(out, "✓ TPM Definition of Done SATISFIED: All %d test case chains have intact, unbroken lineage to root objects and 0 unbound test criteria!\n", checkedCount)
	return nil
}

// NewDashboardCmd creates the `zqk test dashboard` command.
func NewDashboardCmd() *cobra.Command {
	cmd := bldr.NewTestDashboardCommandBuilder()
	if cmd.Flags().Lookup("view") == nil {
		cmd.Flags().String("view", "active", "Dashboard view mode: 'active' (lean in-flight working set), 'regression' (completed verification chains), 'all' (both)")
	}
	if cmd.Flags().Lookup("refresh") == nil {
		cmd.Flags().BoolP("refresh", "r", false, "Force full re-scan from storage and refresh the lite-file cache")
	}
	if cmd.Flags().Lookup("check-dod") == nil {
		cmd.Flags().Bool("check-dod", false, "TPM Definition of Done check: verify that 100% of active test case lineages are intact up to root")
	}
	if cmd.Flags().Lookup("color") == nil {
		cmd.Flags().Bool("color", false, "Force colored output even when stdout is not a TTY (useful when piping to 'less -R')")
	}
	if cmd.Flags().Lookup("pager") == nil {
		cmd.Flags().BoolP("pager", "p", false, "Pipe output through a pager ($PAGER or less -R) with full color support")
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			watchStr, _ := cmd.Flags().GetString("watch")
			showAll, _ := cmd.Flags().GetBool("all")
			statusFilter, _ := cmd.Flags().GetString("status")
			tcFilter, _ := cmd.Flags().GetString("test-case")
			viewMode, _ := cmd.Flags().GetString("view")
			refresh, _ := cmd.Flags().GetBool("refresh")
			checkDoD, _ := cmd.Flags().GetBool("check-dod")
			forceColor, _ := cmd.Flags().GetBool("color")
			usePager, _ := cmd.Flags().GetBool("pager")

			if forceColor || usePager || ((os.Getenv("CLICOLOR_FORCE") == "1" || os.Getenv("FORCE_COLOR") == "1") && os.Getenv("NO_COLOR") == "") {
				color.NoColor = false
			}

			if showAll {
				viewMode = "all"
			}
			if viewMode == "" {
				viewMode = "active"
			}

			var watchInterval time.Duration
			if watchStr != "0" && watchStr != "" {
				var err error
				watchInterval, err = time.ParseDuration(watchStr)
				if err != nil {
					watchInterval = 2 * time.Second
				}
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			state := NewDashboardState()
			projectRoot := proc.ProjectRoot()

			// Fast-path: load materialized lite-file unless refresh, check-dod, or specific filters requested
			useLite := !refresh && !checkDoD && !showAll && statusFilter == "" && tcFilter == ""
			loaded := false
			if useLite {
				eng := state.EnsureEngine(projectRoot)
				env, err := eng.GetOrRecoverPayload(ctx, proc.Storage())
				if err == nil && env != nil && env.Payload != nil {
					// In ephemeral CLI execution (non-watch), if the projection is stale, recovering,
					// or empty, do not exit with an empty skeleton; fall back to synchronous storage load.
					if watchInterval <= 0 && (env.Stale || env.Recovering || len(env.Payload.TestCases) == 0) {
						loaded = false
					} else {
						state.mu.Lock()
						state.applyPayloadLocked(env.Payload)
						state.LastUpdated = env.MaterializedAt
						state.mu.Unlock()
						loaded = true
					}
				}
			}

			if !loaded {
				// Full scan from storage
				if err := state.LoadFromStorage(ctx, proc, showAll, statusFilter, tcFilter); err != nil {
					return err
				}
				// Materialize lite-file for instant subsequent loads
				if !showAll && statusFilter == "" && tcFilter == "" {
					_ = state.SaveToLiteFile(projectRoot)
				}
			}

			// Validate Definition of Done if requested
			if checkDoD {
				return state.ValidateTPMDefinitionOfDone(cmd.OutOrStdout(), cmd.ErrOrStderr())
			}

			// Render once
			if watchInterval <= 0 {
				if usePager {
					return runWithPager(ctx, func(w ioWriter) error {
						return state.Render(w, false, viewMode)
					})
				}
				return state.Render(cmd.OutOrStdout(), false, viewMode)
			}

			// Watch mode: setup WAL subscription and refresh loop
			updateCh := make(chan struct{}, 10)
			subCtx, cancelSub := context.WithCancel(ctx)
			defer cancelSub()

			// Listen for SIGINT / SIGTERM for clean shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			// Start background WAL subscriber
			goroutinelabels.NewGoroutine("test-dashboard-wal-subscriber", "subscribing to criteria satisfied events").
				StartSimple(func() {
					state.SubscribeWAL(subCtx, projectRoot, updateCh)
				})

			// Initial screen clear
			_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[2J")

			ticker := time.NewTicker(watchInterval)
			defer ticker.Stop()

			// Render initial frame
			_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[H")
			_ = state.Render(cmd.OutOrStdout(), true, viewMode)
			_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[J")

			for {
				select {
				case <-sigCh:
					return nil
				case <-ctx.Done():
					return nil
				case <-updateCh:
					// Re-render immediately on event
					_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[H")
					_ = state.Render(cmd.OutOrStdout(), true, viewMode)
					_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[J")
					_ = state.SaveToLiteFile(projectRoot)
				case <-ticker.C:
					// Periodic refresh (also catches any un-indexed storage changes)
					_ = state.LoadFromStorage(ctx, proc, showAll, statusFilter, tcFilter)
					_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[H")
					_ = state.Render(cmd.OutOrStdout(), true, viewMode)
					_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[J")
					_ = state.SaveToLiteFile(projectRoot)
				}
			}
		})(cmd, args)
	}

	return cmd
}

// LoadFromStorage reads test cases, criteria, and their full lineage from the Knowledge Kernel.
func (s *DashboardState) LoadFromStorage(
	ctx context.Context,
	proc *cli.Processor,
	showAll bool,
	statusFilter string,
	tcFilter string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.scanFromStorageLocked(ctx, proc.Storage(), proc.SecurityContext(), proc.ProjectRoot(), showAll, statusFilter, tcFilter)
}

func (s *DashboardState) scanFromStorageLocked(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *storage.SecurityContext,
	projectRoot string,
	showAll bool,
	statusFilter string,
	tcFilter string,
) error {
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	// In-memory cache for fast lineage resolution across the scan
	objCache := make(map[string]map[string]any)
	readWithCache := func(id string) map[string]any {
		if id == "" {
			return nil
		}
		if cached, ok := objCache[id]; ok {
			return cached
		}
		obj, err := sp.Read(ctx, secCtx, id)
		if err != nil || obj == nil {
			return nil
		}
		objCache[id] = obj
		return obj
	}

	tcFilterMap := make(map[string]any)
	if tcFilter != "" {
		tcFilterMap[objects.FieldKeyID] = tcFilter
	}
	if statusFilter != "" {
		tcFilterMap[objects.FieldKeyStatus] = statusFilter
	} else if !showAll {
		tcFilterMap[objects.FieldKeyStatus] = map[string]any{"$ne": objects.ObjectStatusArchived}
	}

	tcFields := []string{
		objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyTitle,
		objects.FieldKeyPathOrID, "scope", "category",
		objects.FieldKeyCriteriaRefs, objects.FieldKeyRequirementRefs,
		objects.FieldKeyBacklogItemRefs, objects.FieldKeyGoalRefs,
		objects.FieldKeyRemainingOpenCount,
	}

	listRes, err := sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    objects.KindTestCase,
		Filters: tcFilterMap,
		Fields:  tcFields,
	})
	if err != nil {
		return fmt.Errorf("failed to list test cases: %w", err)
	}

	// Filter and sort test cases
	s.TestCases = make(map[string]*TestCaseModel)
	s.CriteriaIndex = make(map[string][]*TestCaseModel)
	s.UnboundTestCriteria = make([]*UnboundCriterionModel, 0)
	var orderedIDs []string

	for _, tcObj := range listRes.Objects {
		id, _ := tcObj[objects.FieldKeyID].(string)
		status, _ := tcObj[objects.FieldKeyStatus].(string)
		title, _ := tcObj[objects.FieldKeyTitle].(string)
		pathOrID, _ := tcObj[objects.FieldKeyPathOrID].(string)
		scope, _ := tcObj["scope"].(string)
		category, _ := tcObj["category"].(string)

		if tcFilter != "" && !strings.EqualFold(id, tcFilter) {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(status, statusFilter) {
			continue
		}
		if !showAll && (status == objects.ObjectStatusArchived) {
			continue
		}

		critRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyCriteriaRefs])
		reqRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyRequirementRefs])
		bliRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyBacklogItemRefs])
		goalRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyGoalRefs])

		// Full lineage resolution up to root object
		lineage := resolveLineageChain(readWithCache, reqRefs, bliRefs, goalRefs)

		criteriaList := make([]*CriterionState, 0, len(critRefs))
		completedCount := 0

		for _, crID := range critRefs {
			cState := &CriterionState{
				ID:     crID,
				Status: objects.ObjectStatusOriginated,
			}
			if crObj := readWithCache(crID); crObj != nil {
				if st, ok := crObj[objects.FieldKeyStatus].(string); ok && st != "" {
					cState.Status = st
				}
				if desc, ok := crObj[objects.FieldKeyDescription].(string); ok {
					cState.Description = desc
				}
			}
			if isCriterionComplete(cState.Status) {
				completedCount++
			}
			criteriaList = append(criteriaList, cState)
		}

		remOpen := len(criteriaList) - completedCount
		if remOpen < 0 {
			remOpen = 0
		}
		if len(criteriaList) == 0 {
			if remVal, ok := tcObj[objects.FieldKeyRemainingOpenCount].(int); ok && remVal >= 0 {
				remOpen = remVal
			}
		}

		model := &TestCaseModel{
			ID:                 id,
			Title:              title,
			Status:             status,
			PathOrID:           pathOrID,
			Scope:              scope,
			Category:           category,
			Lineage:            lineage,
			RequirementRefs:    reqRefs,
			BacklogItemRefs:    bliRefs,
			GoalRefs:           goalRefs,
			Criteria:           criteriaList,
			RemainingOpenCount: remOpen,
			TotalCriteria:      len(criteriaList),
			CompletedCriteria:  completedCount,
		}

		s.TestCases[id] = model
		orderedIDs = append(orderedIDs, id)

		for _, cr := range criteriaList {
			s.CriteriaIndex[cr.ID] = append(s.CriteriaIndex[cr.ID], model)
		}
	}

	critFilterMap := make(map[string]any)
	if !showAll {
		critFilterMap[objects.FieldKeyStatus] = map[string]any{"$ne": objects.ObjectStatusArchived}
	}
	critFields := []string{
		objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus,
		"category", "validation_method", objects.FieldKeyGoalRefs,
	}

	critListRes, err := sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    objects.KindCriteria,
		Filters: critFilterMap,
		Fields:  critFields,
	})
	if err == nil {
		for _, crObj := range critListRes.Objects {
			crID, _ := crObj[objects.FieldKeyID].(string)
			if len(s.CriteriaIndex[crID]) > 0 {
				continue
			}
			cat, _ := crObj["category"].(string)
			vMethod, _ := crObj["validation_method"].(string)
			status, _ := crObj[objects.FieldKeyStatus].(string)
			title, _ := crObj[objects.FieldKeyTitle].(string)

			if !strings.EqualFold(cat, "test") && !strings.EqualFold(vMethod, "automated_test") {
				continue
			}
			if !showAll && (status == objects.ObjectStatusArchived) {
				continue
			}

			cGoalRefs := lifecycle.StringRefsFromAny(crObj[objects.FieldKeyGoalRefs])
			cLineage := resolveLineageChain(readWithCache, nil, nil, cGoalRefs)

			s.UnboundTestCriteria = append(s.UnboundTestCriteria, &UnboundCriterionModel{
				ID:               crID,
				Title:            title,
				Status:           status,
				Category:         cat,
				ValidationMethod: vMethod,
				Lineage:          cLineage,
			})
		}
	}

	sort.Slice(orderedIDs, func(i, j int) bool {
		tcA := s.TestCases[orderedIDs[i]]
		tcB := s.TestCases[orderedIDs[j]]
		// Priority: active first, then originated, then complete, then archived
		prio := func(st string) int {
			switch st {
			case objects.ObjectStatusActive:
				return 1
			case "metrics_captured", "draft", "originated":
				return 2
			case objects.ObjectStatusComplete:
				return 3
			case objects.ObjectStatusArchived:
				return 4
			default:
				return 5
			}
		}
		if prio(tcA.Status) != prio(tcB.Status) {
			return prio(tcA.Status) < prio(tcB.Status)
		}
		return tcA.ID < tcB.ID
	})

	s.TestCaseOrder = orderedIDs
	if projectRoot == "" {
		projectRoot = s.projectRoot
	}
	if projectRoot != "" {
		s.seedRecentEventsFromWAL(projectRoot)
	}
	s.LastUpdated = time.Now()
	return nil
}

func (s *DashboardState) seedRecentEventsFromWAL(projectRoot string) {
	wal, err := lifecycle.GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		return
	}
	_ = wal.ReplayFrom(0, func(ev *lifecycle.LifecycleEvent) error {
		if ev == nil {
			return nil
		}
		if ev.EventType == lifecycle.EventTypeCriterionSatisfied ||
			(ev.EventType == lifecycle.EventTypeStatusTransition && (strings.EqualFold(ev.Kind, objects.KindCriteria) || strings.EqualFold(ev.Kind, objects.KindTestCase))) {
			s.handleLifecycleEventLocked(ev)
		}
		return nil
	})
}

// SubscribeWAL monitors the lifecycle WAL for criteria satisfaction and status transitions via accumulator Engine.
func (s *DashboardState) SubscribeWAL(ctx context.Context, projectRoot string, updateCh chan<- struct{}) {
	eng := s.EnsureEngine(projectRoot)
	if eng != nil {
		eng.SubscribeWAL(ctx, updateCh)
		return
	}

	wal, err := lifecycle.GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		return
	}

	var cursor walutil.ReplayCursor
	pollInterval := 200 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		newEvents := 0
		newCursor, err := wal.ReplayFromCursor(cursor, func(ev *lifecycle.LifecycleEvent) error {
			if ev == nil {
				return nil
			}
			newEvents++
			s.HandleLifecycleEvent(ev)
			return nil
		})

		if err == nil {
			cursor = newCursor
		}

		if newEvents > 0 {
			select {
			case updateCh <- struct{}{}:
			default:
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

// HandleLifecycleEvent updates internal state based on WAL events.
func (s *DashboardState) HandleLifecycleEvent(ev *lifecycle.LifecycleEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handleLifecycleEventLocked(ev)
}

func (s *DashboardState) handleLifecycleEventLocked(ev *lifecycle.LifecycleEvent) {

	ts := ev.Ts
	if ts.IsZero() {
		ts = time.Now()
	}

	switch ev.EventType {
	case lifecycle.EventTypeCriterionSatisfied:
		critID := ev.CriterionID
		scopeDesc := ""
		if len(ev.Scope) > 0 {
			var parts []string
			for k, v := range ev.Scope {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			scopeDesc = strings.Join(parts, ", ")
		}

		// Update linked criteria
		if tcs, ok := s.CriteriaIndex[critID]; ok {
			for _, tc := range tcs {
				for _, cr := range tc.Criteria {
					if cr.ID == critID {
						cr.Status = objects.ObjectStatusComplete
						cr.LastSatisfied = ts
					}
				}
				// Recount completed criteria
				completed := 0
				for _, cr := range tc.Criteria {
					if isCriterionComplete(cr.Status) {
						completed++
					}
				}
				prevStatus := tc.Status
				tc.CompletedCriteria = completed
				tc.RemainingOpenCount = tc.TotalCriteria - completed
				if tc.RemainingOpenCount <= 0 && tc.TotalCriteria > 0 {
					tc.Status = objects.ObjectStatusComplete
					if prevStatus != objects.ObjectStatusComplete {
						s.appendRecentEvent(LifecycleEventSummary{
							Timestamp:   ts,
							EventType:   string(lifecycle.EventTypeStatusTransition),
							Kind:        objects.KindTestCase,
							ObjectID:    tc.ID,
							CriterionID: critID,
							Message:     fmt.Sprintf("TRACEABILITY CHAIN GRADUATED: %s (100%% criteria green) ➔ Moved to Regression Suite", tc.ID),
						})
					}
				}
			}
		}

		msg := fmt.Sprintf("CRITERION SATISFIED: %s", critID)
		if scopeDesc != "" {
			msg += fmt.Sprintf(" (%s)", scopeDesc)
		}
		s.appendRecentEvent(LifecycleEventSummary{
			Timestamp:   ts,
			EventType:   string(ev.EventType),
			CriterionID: critID,
			Scope:       ev.Scope,
			Message:     msg,
		})

	case lifecycle.EventTypeStatusTransition:
		if strings.EqualFold(ev.Kind, objects.KindCriteria) || strings.EqualFold(ev.Kind, "criterion") {
			critID := ev.ID
			if tcs, ok := s.CriteriaIndex[critID]; ok {
				for _, tc := range tcs {
					for _, cr := range tc.Criteria {
						if cr.ID == critID {
							cr.Status = ev.ToStatus
							if isCriterionComplete(ev.ToStatus) {
								cr.LastSatisfied = ts
							}
						}
					}
					completed := 0
					for _, cr := range tc.Criteria {
						if isCriterionComplete(cr.Status) {
							completed++
						}
					}
					prevStatus := tc.Status
					tc.CompletedCriteria = completed
					tc.RemainingOpenCount = tc.TotalCriteria - completed
					if tc.RemainingOpenCount <= 0 && tc.TotalCriteria > 0 {
						tc.Status = objects.ObjectStatusComplete
						if prevStatus != objects.ObjectStatusComplete {
							s.appendRecentEvent(LifecycleEventSummary{
								Timestamp:   ts,
								EventType:   string(lifecycle.EventTypeStatusTransition),
								Kind:        objects.KindTestCase,
								ObjectID:    tc.ID,
								CriterionID: critID,
								Message:     fmt.Sprintf("TRACEABILITY CHAIN GRADUATED: %s (100%% criteria green) ➔ Moved to Regression Suite", tc.ID),
							})
						}
					}
				}
			}
			s.appendRecentEvent(LifecycleEventSummary{
				Timestamp:   ts,
				EventType:   string(ev.EventType),
				Kind:        ev.Kind,
				CriterionID: critID,
				Message:     fmt.Sprintf("CRITERION %s: %s -> %s", critID, ev.FromStatus, ev.ToStatus),
			})
		} else if strings.EqualFold(ev.Kind, objects.KindTestCase) {
			if tc, ok := s.TestCases[ev.ID]; ok {
				tc.Status = ev.ToStatus
			} else {
				tc = s.hydrateTestCaseFromStorageLocked(ev.ID)
				if tc != nil {
					tc.Status = ev.ToStatus
					s.TestCases[ev.ID] = tc
					s.ensureTestCaseInOrderLocked(ev.ID)
				}
			}
			s.appendRecentEvent(LifecycleEventSummary{
				Timestamp: ts,
				EventType: string(ev.EventType),
				Kind:      ev.Kind,
				ObjectID:  ev.ID,
				Message:   fmt.Sprintf("TEST CASE %s: %s -> %s", ev.ID, ev.FromStatus, ev.ToStatus),
			})
		}
	case lifecycle.EventTypeReferenceLinked:
		s.handleReferenceLinkedLocked(ev, ts)
	}

	s.LastUpdated = time.Now()
}

func (s *DashboardState) handleReferenceLinkedLocked(ev *lifecycle.LifecycleEvent, ts time.Time) {
	if ev.Kind == objects.KindTestCase && ev.TargetKind == objects.KindCriteria {
		tcID := ev.ID
		critID := ev.TargetID
		tc, ok := s.TestCases[tcID]
		if !ok {
			tc = s.hydrateTestCaseFromStorageLocked(tcID)
			if tc == nil {
				return
			}
			s.TestCases[tcID] = tc
		} else if tc.Title == "" || tc.Lineage == nil {
			s.enrichTestCaseFromStorageLocked(tc)
		}
		s.ensureTestCaseInOrderLocked(tcID)

		// Ensure criterion is in tc.Criteria
		found := false
		for _, cr := range tc.Criteria {
			if cr.ID == critID {
				found = true
				break
			}
		}
		if !found {
			tc.Criteria = append(tc.Criteria, &CriterionState{
				ID:     critID,
				Status: objects.ObjectStatusOriginated,
			})
			tc.TotalCriteria = len(tc.Criteria)
			tc.RemainingOpenCount = tc.TotalCriteria - tc.CompletedCriteria
		}

		// Update CriteriaIndex
		s.CriteriaIndex[critID] = append(s.CriteriaIndex[critID], tc)

		s.appendRecentEvent(LifecycleEventSummary{
			Timestamp:   ts,
			EventType:   string(ev.EventType),
			Kind:        ev.Kind,
			ObjectID:    tcID,
			CriterionID: critID,
			Message:     fmt.Sprintf("LINKAGE SHOCKWAVE: Test Case %s ➔ Criterion %s", tcID, critID),
		})
	} else if ev.Kind == objects.KindTestCase && ev.TargetKind == objects.KindBacklogItem {
		tcID := ev.ID
		bliID := ev.TargetID
		tc, ok := s.TestCases[tcID]
		if !ok {
			tc = s.hydrateTestCaseFromStorageLocked(tcID)
			if tc == nil {
				return
			}
			s.TestCases[tcID] = tc
		} else if tc.Title == "" || tc.Lineage == nil {
			s.enrichTestCaseFromStorageLocked(tc)
		}
		s.ensureTestCaseInOrderLocked(tcID)
		already := false
		for _, r := range tc.BacklogItemRefs {
			if r == bliID {
				already = true
				break
			}
		}
		if !already {
			tc.BacklogItemRefs = append(tc.BacklogItemRefs, bliID)
		}
		s.refreshLineageLocked(tc)
	} else if ev.Kind == objects.KindTestCase && ev.TargetKind == objects.KindRequirement {
		tcID := ev.ID
		reqID := ev.TargetID
		tc, ok := s.TestCases[tcID]
		if !ok {
			tc = s.hydrateTestCaseFromStorageLocked(tcID)
			if tc == nil {
				return
			}
			s.TestCases[tcID] = tc
		} else if tc.Title == "" || tc.Lineage == nil {
			s.enrichTestCaseFromStorageLocked(tc)
		}
		s.ensureTestCaseInOrderLocked(tcID)
		already := false
		for _, r := range tc.RequirementRefs {
			if r == reqID {
				already = true
				break
			}
		}
		if !already {
			tc.RequirementRefs = append(tc.RequirementRefs, reqID)
		}
		s.refreshLineageLocked(tc)
	}
}

func (s *DashboardState) ensureTestCaseInOrderLocked(tcID string) {
	for _, id := range s.TestCaseOrder {
		if id == tcID {
			return
		}
	}
	s.TestCaseOrder = append(s.TestCaseOrder, tcID)
}

func (s *DashboardState) hydrateTestCaseFromStorageLocked(tcID string) *TestCaseModel {
	if s.projectRoot == "" || tcID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	factory, err := storage.NewStorageFactory(ctx, s.projectRoot)
	if err != nil || factory == nil {
		return nil
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()
	tcObj, err := sp.Read(ctx, secCtx, tcID)
	if err != nil || tcObj == nil {
		return nil
	}

	title, _ := tcObj[objects.FieldKeyTitle].(string)
	status, _ := tcObj[objects.FieldKeyStatus].(string)
	if status == "" {
		status = objects.ObjectStatusActive
	}
	pathOrID, _ := tcObj[objects.FieldKeyPathOrID].(string)
	scope, _ := tcObj["scope"].(string)
	category, _ := tcObj["category"].(string)

	critRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyCriteriaRefs])
	reqRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyRequirementRefs])
	bliRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyBacklogItemRefs])
	goalRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyGoalRefs])

	readWithCache := func(id string) map[string]any {
		if id == "" {
			return nil
		}
		obj, _ := sp.Read(ctx, secCtx, id)
		return obj
	}
	lineage := resolveLineageChain(readWithCache, reqRefs, bliRefs, goalRefs)

	criteriaList := make([]*CriterionState, 0, len(critRefs))
	completedCount := 0
	for _, crID := range critRefs {
		cState := &CriterionState{
			ID:     crID,
			Status: objects.ObjectStatusOriginated,
		}
		if crObj := readWithCache(crID); crObj != nil {
			if st, ok := crObj[objects.FieldKeyStatus].(string); ok && st != "" {
				cState.Status = st
			}
			if desc, ok := crObj[objects.FieldKeyDescription].(string); ok {
				cState.Description = desc
			}
		}
		if isCriterionComplete(cState.Status) {
			completedCount++
		}
		criteriaList = append(criteriaList, cState)
	}

	remOpen := len(criteriaList) - completedCount
	if remOpen < 0 {
		remOpen = 0
	}

	return &TestCaseModel{
		ID:                 tcID,
		Title:              title,
		Status:             status,
		PathOrID:           pathOrID,
		Scope:              scope,
		Category:           category,
		Lineage:            lineage,
		RequirementRefs:    reqRefs,
		BacklogItemRefs:    bliRefs,
		GoalRefs:           goalRefs,
		Criteria:           criteriaList,
		RemainingOpenCount: remOpen,
		TotalCriteria:      len(criteriaList),
		CompletedCriteria:  completedCount,
	}
}

func (s *DashboardState) enrichTestCaseFromStorageLocked(tc *TestCaseModel) {
	if tc == nil || s.projectRoot == "" {
		return
	}
	hydrated := s.hydrateTestCaseFromStorageLocked(tc.ID)
	if hydrated == nil {
		return
	}
	if tc.Title == "" {
		tc.Title = hydrated.Title
	}
	if tc.PathOrID == "" {
		tc.PathOrID = hydrated.PathOrID
	}
	if tc.Scope == "" {
		tc.Scope = hydrated.Scope
	}
	if tc.Category == "" {
		tc.Category = hydrated.Category
	}
	if len(tc.RequirementRefs) == 0 {
		tc.RequirementRefs = hydrated.RequirementRefs
	}
	if len(tc.BacklogItemRefs) == 0 {
		tc.BacklogItemRefs = hydrated.BacklogItemRefs
	}
	if len(tc.GoalRefs) == 0 {
		tc.GoalRefs = hydrated.GoalRefs
	}
	if tc.Lineage == nil {
		tc.Lineage = hydrated.Lineage
	}
	if len(tc.Criteria) == 0 && len(hydrated.Criteria) > 0 {
		tc.Criteria = hydrated.Criteria
		tc.TotalCriteria = hydrated.TotalCriteria
		tc.CompletedCriteria = hydrated.CompletedCriteria
		tc.RemainingOpenCount = hydrated.RemainingOpenCount
	}
}

func (s *DashboardState) refreshLineageLocked(tc *TestCaseModel) {
	if tc == nil || s.projectRoot == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	factory, err := storage.NewStorageFactory(ctx, s.projectRoot)
	if err != nil || factory == nil {
		return
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()
	readWithCache := func(id string) map[string]any {
		if id == "" {
			return nil
		}
		obj, _ := sp.Read(ctx, secCtx, id)
		return obj
	}
	tc.Lineage = resolveLineageChain(readWithCache, tc.RequirementRefs, tc.BacklogItemRefs, tc.GoalRefs)
}

func (s *DashboardState) appendRecentEvent(ev LifecycleEventSummary) {
	s.RecentEvents = append(s.RecentEvents, ev)
	if len(s.RecentEvents) > 10 {
		s.RecentEvents = s.RecentEvents[len(s.RecentEvents)-10:]
	}
}

// Render writes the formatted dashboard to the destination writer.
// Invariant: Traceability operates as a state machine: when the last chain item goes green,
// a completion event fires that graduates the test case chain into the regression view,
// keeping the active working set lean, high-signal, and manageable.
func (s *DashboardState) Render(w ioWriter, isWatch bool, viewMode string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	white := color.New(color.FgWhite).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	var buf strings.Builder

	// Partition test cases into Active Working Set vs Regression Suite
	var activeTCsList []string
	var regressionTCsList []string

	for _, tcID := range s.TestCaseOrder {
		tc := s.TestCases[tcID]
		isRegression := tc.Status == objects.ObjectStatusComplete && tc.RemainingOpenCount == 0 && (tc.Lineage == nil || tc.Lineage.IsIntact)
		if isRegression {
			regressionTCsList = append(regressionTCsList, tcID)
		} else {
			activeTCsList = append(activeTCsList, tcID)
		}
	}

	modeBadge := "ACTIVE WORKING SET"
	if viewMode == "regression" {
		modeBadge = "REGRESSION SUITE"
	} else if viewMode == "all" {
		modeBadge = "FULL MATRIX"
	}
	if isWatch {
		modeBadge += " | LIVE STREAM"
	}

	buf.WriteString(fmt.Sprintf("\n%s\n", bold(fmt.Sprintf("⚡ ZQK TEST MATRIX & CRITERIA RADAR | [%s]", modeBadge))))
	buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("═", 78)))

	// Aggregate Metrics
	totalTCs := len(s.TestCaseOrder)
	activeTCs := len(activeTCsList)
	completeTCs := len(regressionTCsList)
	totalCriteria := 0
	completeCriteria := 0

	for _, tcID := range s.TestCaseOrder {
		tc := s.TestCases[tcID]
		totalCriteria += tc.TotalCriteria
		completeCriteria += tc.CompletedCriteria
	}

	critPct := 0.0
	if totalCriteria > 0 {
		critPct = (float64(completeCriteria) / float64(totalCriteria)) * 100
	}

	intactLineageCount := 0
	for _, tcID := range s.TestCaseOrder {
		tc := s.TestCases[tcID]
		if tc.Lineage != nil && tc.Lineage.IsIntact {
			intactLineageCount++
		}
	}

	buf.WriteString(fmt.Sprintf("  Test Cases : %s total | %s in-flight working set | %s regression pool\n",
		bold(fmt.Sprintf("%d", totalTCs)),
		yellow(fmt.Sprintf("%d", activeTCs)),
		green(fmt.Sprintf("%d", completeTCs)),
	))
	buf.WriteString(fmt.Sprintf("  Criteria   : %s/%s satisfied (%s)\n",
		green(fmt.Sprintf("%d", completeCriteria)),
		bold(fmt.Sprintf("%d", totalCriteria)),
		bold(fmt.Sprintf("%.1f%%", critPct)),
	))
	lineageColor := green
	if intactLineageCount < totalTCs {
		lineageColor = yellow
	}
	buf.WriteString(fmt.Sprintf("  Lineage    : %s\n",
		lineageColor(fmt.Sprintf("%d/%d intact chains (full lineage to root object)", intactLineageCount, totalTCs)),
	))
	buf.WriteString(fmt.Sprintf("  Updated    : %s\n", dim(s.LastUpdated.Format("15:04:05.000 MST"))))
	buf.WriteString(fmt.Sprintf("%s\n\n", strings.Repeat("─", 78)))

	// Legend
	buf.WriteString(fmt.Sprintf("  LEGEND: 🟢 Satisfied/Complete  🟡 In-Progress/Verifying  ⚪ Pending/Draft  🔴 Failed  📁 Archived\n\n"))

	renderTestCase := func(tcID string) {
		tc := s.TestCases[tcID]

		statusColor := white
		switch tc.Status {
		case objects.ObjectStatusComplete:
			statusColor = green
		case objects.ObjectStatusActive:
			statusColor = yellow
		case objects.ObjectStatusArchived:
			statusColor = dim
		}
		badge := fmt.Sprintf("[%s]", strings.ToUpper(tc.Status))

		buf.WriteString(fmt.Sprintf("▶ %s  %s  %s\n",
			bold(cyan(tc.ID)),
			statusColor(badge),
			bold(tc.Title),
		))

		// Full lineage chain up to Root Object
		buf.WriteString(fmt.Sprintf("    Lineage  : %s\n", formatLineageStrip(tc.Lineage, tc.ID, tc.Status)))

		// Target and scope
		if tc.PathOrID != "" {
			targetStr := tc.PathOrID
			if tc.Scope != "" {
				targetStr += fmt.Sprintf(" (%s)", tc.Scope)
			}
			buf.WriteString(fmt.Sprintf("    Target   : %s\n", dim(targetStr)))
		}

		// Criteria Icon Strip
		var iconPills []string
		for _, cr := range tc.Criteria {
			icon := "⚪"
			cColor := dim
			switch {
			case isCriterionComplete(cr.Status):
				icon = "🟢"
				cColor = green
			case cr.Status == "in_progress" || cr.Status == "testing" || cr.Status == "verifying" || cr.Status == "awaiting_verification":
				icon = "🟡"
				cColor = yellow
			case cr.Status == "error" || cr.Status == "failed" || cr.Status == "rejected":
				icon = "🔴"
				cColor = red
			}
			pill := fmt.Sprintf("[%s %s]", icon, cColor(cr.ID))
			iconPills = append(iconPills, pill)
		}

		// Render Icon Pills (wrap nicely if long)
		if len(iconPills) > 0 {
			buf.WriteString("    Criteria : ")
			for i, pill := range iconPills {
				if i > 0 && i%3 == 0 {
					buf.WriteString("\n               ")
				}
				buf.WriteString(pill + " ")
			}
			buf.WriteString("\n")
		} else {
			buf.WriteString(fmt.Sprintf("    Criteria : %s\n", dim("(none linked)")))
		}

		// Progress bar
		pct := 0.0
		if tc.TotalCriteria > 0 {
			pct = (float64(tc.CompletedCriteria) / float64(tc.TotalCriteria)) * 100
		}
		progressBar := renderProgressBar(pct, 20)
		buf.WriteString(fmt.Sprintf("    Progress : %s %3.0f%% (%d/%d satisfied, %d open)\n\n",
			progressBar, pct, tc.CompletedCriteria, tc.TotalCriteria, tc.RemainingOpenCount,
		))
	}

	switch viewMode {
	case "regression":
		buf.WriteString(fmt.Sprintf("%s\n", bold(fmt.Sprintf("🛡️  REGRESSION TESTING SUITE (%d VERIFIED & INTACT CHAINS)", len(regressionTCsList)))))
		buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("─", 78)))
		if len(regressionTCsList) == 0 {
			buf.WriteString(fmt.Sprintf("  %s No completed regression test cases found.\n\n", dim("ℹ")))
		} else {
			for _, tcID := range regressionTCsList {
				renderTestCase(tcID)
			}
		}
		if len(activeTCsList) > 0 {
			buf.WriteString(fmt.Sprintf("  %s %s\n\n",
				yellow("🔥 ACTIVE WORKING SET:"),
				bold(paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("%d in-flight / unverified chains remaining [switch view with: zqk test dashboard --view active]", len(activeTCsList)))),
			))
		}

	case "all":
		if len(activeTCsList) > 0 {
			buf.WriteString(fmt.Sprintf("%s\n", bold(fmt.Sprintf("🔥 ACTIVE WORKING SET (%d IN-FLIGHT / UNVERIFIED)", len(activeTCsList)))))
			buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("─", 78)))
			for _, tcID := range activeTCsList {
				renderTestCase(tcID)
			}
		}
		if len(regressionTCsList) > 0 {
			buf.WriteString(fmt.Sprintf("%s\n", bold(fmt.Sprintf("🛡️  REGRESSION SUITE (%d VERIFIED & INTACT CHAINS)", len(regressionTCsList)))))
			buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("─", 78)))
			for _, tcID := range regressionTCsList {
				renderTestCase(tcID)
			}
		}

	default: // "active"
		buf.WriteString(fmt.Sprintf("%s\n", bold(fmt.Sprintf("🔥 ACTIVE WORKING SET (%d IN-FLIGHT / UNVERIFIED CHAINS)", len(activeTCsList)))))
		buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("─", 78)))
		if len(activeTCsList) == 0 {
			buf.WriteString(fmt.Sprintf("  %s All test case chains verified! Working set is clean.\n\n", green("✓")))
		} else {
			for _, tcID := range activeTCsList {
				renderTestCase(tcID)
			}
		}
		if len(regressionTCsList) > 0 {
			buf.WriteString(fmt.Sprintf("  %s %s\n\n",
				green("🛡️  REGRESSION TESTING POOL:"),
				bold(paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("%d verified chains green & passing [pruned from active view — inspect with: zqk test dashboard --view regression or --all]", len(regressionTCsList)))),
			))
		}
	}

	// Unbound / Standalone Test Criteria Section
	if len(s.UnboundTestCriteria) > 0 {
		buf.WriteString(fmt.Sprintf("%s\n", bold(fmt.Sprintf("🧪 UNBOUND & STANDALONE TEST CRITERIA (%d items)", len(s.UnboundTestCriteria)))))
		buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("─", 78)))
		for _, unc := range s.UnboundTestCriteria {
			uColor := white
			switch unc.Status {
			case objects.ObjectStatusComplete, "validated", "verified":
				uColor = green
			case objects.ObjectStatusActive, "in_progress", "awaiting_verification":
				uColor = yellow
			}
			uBadge := fmt.Sprintf("[%s]", strings.ToUpper(unc.Status))
			buf.WriteString(fmt.Sprintf("  ▶ %s  %s  %s\n",
				bold(cyan(unc.ID)),
				uColor(uBadge),
				bold(unc.Title),
			))
			buf.WriteString(fmt.Sprintf("    Lineage  : %s\n", formatLineageStrip(unc.Lineage, unc.ID, unc.Status)))
			detail := fmt.Sprintf("category: %s", unc.Category)
			if unc.ValidationMethod != "" {
				detail += fmt.Sprintf(" | method: %s", unc.ValidationMethod)
			}
			buf.WriteString(fmt.Sprintf("    Details  : %s\n", dim(detail)))
			buf.WriteString(fmt.Sprintf("    Notice   : %s\n\n", yellow("⚠️ Test criterion has no test_case object bound!")))
		}
	}

	// Live WAL Event Ticker
	buf.WriteString(fmt.Sprintf("%s\n", bold("📡 RECENT CRITERIA SATISFACTION & SHOCKWAVE EVENTS")))
	buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("─", 78)))
	if len(s.RecentEvents) == 0 {
		buf.WriteString(fmt.Sprintf("  %s\n", dim("Waiting for incoming criteria_satisfied events on lifecycle WAL...")))
	} else {
		for _, rev := range s.RecentEvents {
			tsStr := rev.Timestamp.Format("15:04:05")
			buf.WriteString(fmt.Sprintf("  ⚡ [%s] %s\n", dim(tsStr), green(rev.Message)))
		}
	}
	buf.WriteString("\n")

	_, err := fmt.Fprint(w, buf.String())
	return err
}

// ioWriter interface for testable stdout writing
type ioWriter interface {
	Write(p []byte) (n int, err error)
}

func isCriterionComplete(st string) bool {
	switch strings.ToLower(strings.TrimSpace(st)) {
	case objects.ObjectStatusComplete, "validated", "verified", "passed", "success", objects.ObjectStatusArchived:
		return true
	default:
		return false
	}
}

func renderProgressBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int((pct / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	unfilled := width - filled

	green := color.New(color.FgGreen).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	return fmt.Sprintf("[%s%s]",
		green(strings.Repeat("█", filled)),
		dim(strings.Repeat("░", unfilled)),
	)
}

// resolveLineageChain builds the full lineage chain up to the root object.
// Invariant: A traceability chain is considered intact when it displays the full lineage from the test case to the root object.
func resolveLineageChain(
	readObj func(id string) map[string]any,
	reqRefs []string,
	bliRefs []string,
	directGoalRefs []string,
) *LineageChain {
	chain := &LineageChain{}

	// 1. Resolve requirements
	for _, reqID := range reqRefs {
		reqObj := readObj(reqID)
		node := &LineageNode{ID: reqID, Kind: objects.KindRequirement, Status: objects.ObjectStatusNonexistent}
		if reqObj != nil {
			if st, ok := reqObj[objects.FieldKeyStatus].(string); ok && st != "" {
				node.Status = st
			}
			if title, ok := reqObj[objects.FieldKeyTitle].(string); ok {
				node.Title = title
			}
		}
		chain.Requirements = append(chain.Requirements, node)
	}

	// 2. Resolve backlog items
	for _, bliID := range bliRefs {
		bliObj := readObj(bliID)
		node := &LineageNode{ID: bliID, Kind: objects.KindBacklogItem, Status: objects.ObjectStatusNonexistent}
		if bliObj != nil {
			if st, ok := bliObj[objects.FieldKeyStatus].(string); ok && st != "" {
				node.Status = st
			}
			if title, ok := bliObj[objects.FieldKeyTitle].(string); ok {
				node.Title = title
			}
		}
		chain.BacklogItems = append(chain.BacklogItems, node)
	}

	// 3. Find root object candidate
	var candidateRootID string
	var candidateKind string

	for _, gID := range directGoalRefs {
		if strings.HasPrefix(gID, "GOAL-") {
			candidateRootID = gID
			candidateKind = objects.KindGoal
			break
		}
	}

	if candidateRootID == "" {
		for _, reqID := range reqRefs {
			reqObj := readObj(reqID)
			if reqObj != nil {
				gRefs := lifecycle.StringRefsFromAny(reqObj[objects.FieldKeyGoalRefs])
				for _, gID := range gRefs {
					if strings.HasPrefix(gID, "GOAL-") {
						candidateRootID = gID
						candidateKind = objects.KindGoal
						break
					}
				}
				if candidateRootID != "" {
					break
				}
				mRefs := lifecycle.StringRefsFromAny(reqObj[objects.FieldKeyMilestoneRefs])
				for _, mID := range mRefs {
					if strings.HasPrefix(mID, "MIL-") {
						candidateRootID = mID
						candidateKind = objects.KindMilestone
						break
					}
				}
				if candidateRootID != "" {
					break
				}
			}
		}
	}

	if candidateRootID == "" {
		for _, bliID := range bliRefs {
			bliObj := readObj(bliID)
			if bliObj != nil {
				if planRef, ok := bliObj[objects.FieldKeyPriorityPlanRef].(string); ok && planRef != "" {
					candidateRootID = planRef
					candidateKind = objects.KindPriorityPlan
					break
				}
				gRefs := lifecycle.StringRefsFromAny(bliObj[objects.FieldKeyGoalRefs])
				for _, gID := range gRefs {
					if strings.HasPrefix(gID, "GOAL-") {
						candidateRootID = gID
						candidateKind = objects.KindGoal
						break
					}
				}
				if candidateRootID != "" {
					break
				}
			}
		}
	}

	if candidateRootID != "" {
		rootObj := readObj(candidateRootID)
		node := &LineageNode{ID: candidateRootID, Kind: candidateKind, Status: objects.ObjectStatusNonexistent}
		if rootObj != nil {
			if st, ok := rootObj[objects.FieldKeyStatus].(string); ok && st != "" {
				node.Status = st
			}
			if title, ok := rootObj[objects.FieldKeyTitle].(string); ok {
				node.Title = title
			}
		}
		chain.RootObject = node
	}

	// Evaluate if chain is intact:
	// Invariant: Intact requires displaying the full lineage from the test case to the root object.
	hasRoot := chain.RootObject != nil
	hasIntermediate := len(chain.Requirements) > 0 || len(chain.BacklogItems) > 0

	if hasRoot && hasIntermediate {
		chain.IsIntact = true
	} else if hasRoot && len(directGoalRefs) > 0 {
		chain.IsIntact = true
	} else if !hasRoot && !hasIntermediate {
		chain.IsIntact = false
		chain.BrokenReason = "missing root object and requirement/backlog item bindings"
	} else if !hasRoot {
		chain.IsIntact = false
		chain.BrokenReason = "missing root object binding"
	} else {
		chain.IsIntact = false
		chain.BrokenReason = "missing requirement or backlog item intermediate lineage"
	}

	return chain
}

// statusIcon returns the canonical status emoji for system objects.
func statusIcon(st string) string {
	switch strings.ToLower(strings.TrimSpace(st)) {
	case objects.ObjectStatusComplete, "validated", "verified", "passed", "success":
		return "🟢"
	case objects.ObjectStatusActive, "in_progress", "metrics_captured", "testing", "verifying", "awaiting_verification":
		return "🟡"
	case objects.ObjectStatusOriginated, "draft", "proposed", "conceptual", "planned":
		return "⚪"
	case "failed", "error", "rejected":
		return "🔴"
	case objects.ObjectStatusArchived, "deferred", "parked":
		return "📁"
	default:
		return "⚪"
	}
}

// formatLineageStrip formats the lineage chain as a visual pipeline with status icons.
func formatLineageStrip(chain *LineageChain, currentID string, currentStatus string) string {
	if chain == nil {
		return fmt.Sprintf("[⚠️ NO LINEAGE] ➔ [%s %s]  %s", statusIcon(currentStatus), currentID, color.YellowString("✗ Incomplete Traceability"))
	}

	var parts []string

	if chain.RootObject != nil {
		parts = append(parts, fmt.Sprintf("[%s %s]", statusIcon(chain.RootObject.Status), chain.RootObject.ID))
	} else {
		parts = append(parts, "[⚠️ UNBOUND ROOT]")
	}

	if len(chain.Requirements) > 0 {
		var reqPills []string
		for _, r := range chain.Requirements {
			reqPills = append(reqPills, fmt.Sprintf("[%s %s]", statusIcon(r.Status), r.ID))
		}
		parts = append(parts, strings.Join(reqPills, ","))
	} else if len(chain.BacklogItems) == 0 && chain.RootObject == nil {
		parts = append(parts, "[⚠️ UNBOUND REQ]")
	}

	if len(chain.BacklogItems) > 0 {
		var bliPills []string
		for _, b := range chain.BacklogItems {
			bliPills = append(bliPills, fmt.Sprintf("[%s %s]", statusIcon(b.Status), b.ID))
		}
		parts = append(parts, strings.Join(bliPills, ","))
	}

	parts = append(parts, fmt.Sprintf("[%s %s]", statusIcon(currentStatus), currentID))

	arrow := " ➔ "
	lineageStr := strings.Join(parts, arrow)

	if chain.IsIntact {
		lineageStr += "  " + color.GreenString("✓ Chain Intact")
	} else {
		lineageStr += "  " + color.YellowString("✗ Incomplete (%s)", chain.BrokenReason)
	}

	return lineageStr
}

// runWithPager pipes rendered output through $PAGER or less -R, preserving color and vi navigation.
// It temporarily disconnects the command timeout monitor and idle watchdog while the user interacts
// with the pager, and reconnects them and runs registered callbacks upon pager exit.
func runWithPager(ctx context.Context, renderFn func(w ioWriter) error) error {
	reconnect := cli.DisconnectTimeoutMonitor(func() {
		cli.TouchMeaningfulActivity()
	})
	defer reconnect()

	pagerCmdStr := strings.TrimSpace(os.Getenv("PAGER"))
	if pagerCmdStr == "" {
		pagerCmdStr = "less -R"
	} else if strings.HasPrefix(pagerCmdStr, "less") && !strings.Contains(pagerCmdStr, "-R") && !strings.Contains(pagerCmdStr, "-r") {
		pagerCmdStr += " -R"
	}

	parts := strings.Fields(pagerCmdStr)
	pagerExe := parts[0]
	var pagerArgs []string
	if len(parts) > 1 {
		pagerArgs = parts[1:]
	}

	// Avoid killing pager on command timeout while user is actively reading or searching.
	pagerCtx, cancelPager := context.WithCancel(context.Background())
	defer cancelPager()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	pagerCmd := exec.CommandContext(pagerCtx, pagerExe, pagerArgs...)
	pagerCmd.Stdout = os.Stdout
	pagerCmd.Stderr = os.Stderr

	pipeIn, err := pagerCmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := pagerCmd.Start(); err != nil {
		_ = pipeIn.Close()
		return renderFn(os.Stdout)
	}

	renderErr := renderFn(pipeIn)
	_ = pipeIn.Close()

	waitErr := pagerCmd.Wait()
	if renderErr != nil && !errors.Is(renderErr, syscall.EPIPE) && !strings.Contains(renderErr.Error(), "broken pipe") {
		return renderErr
	}
	return waitErr
}
