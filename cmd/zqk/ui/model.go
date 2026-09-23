package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/swarm"
)

// Tab identifier constants.
const (
	TabSeismograph = 0
	TabSwarm       = 1
	TabObjects     = 2
	TabScheduler   = 3
	TotalTabs      = 4
)

// SchedulerJobRow captures a job's operational state for display.
type SchedulerJobRow struct {
	ID        string
	Schedule  string
	LastRunAt string
	NextRunAt string
	Status    string
}

// UIModel encapsulates the dynamic state of the Mission Control TUI.
type UIModel struct {
	ProjectRoot   string
	ActiveTab     int
	ScrollOffset  int // 0 means bottom / newest (or top depending on view)
	AutoScroll    bool
	Width         int
	Height        int
	Mutations     []state.JournalMutation
	SwarmData     map[string]any
	SchedulerJobs []SchedulerJobRow
	ObjectCounts  map[string]int
	LastUpdated   time.Time
	StatusMessage string
}

// NewUIModel constructs an initialized UIModel.
func NewUIModel(projectRoot string, initialTab string) *UIModel {
	tab := TabSeismograph
	switch strings.ToLower(initialTab) {
	case "swarm", "agent", "agents":
		tab = TabSwarm
	case "objects", "object", "backlog":
		tab = TabObjects
	case "scheduler", "jobs", "job":
		tab = TabScheduler
	}

	return &UIModel{
		ProjectRoot:  projectRoot,
		ActiveTab:    tab,
		AutoScroll:   true,
		ObjectCounts: make(map[string]int),
		LastUpdated:  time.Now(),
	}
}

// RefreshMutations re-reads recent events from the kernel streams.
func (m *UIModel) RefreshMutations() {
	if m.ProjectRoot == "" {
		return
	}
	muts := state.ReadRecentJournalMutations(m.ProjectRoot, 200)
	// Order chronologically: oldest first, newest at the end
	for i, j := 0, len(muts)-1; i < j; i, j = i+1, j-1 {
		muts[i], muts[j] = muts[j], muts[i]
	}
	m.Mutations = muts
	m.LastUpdated = time.Now()
}

// RefreshSwarm queries the storage provider for swarm metrics.
func (m *UIModel) RefreshSwarm(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if sp == nil {
		return
	}
	data, err := swarm.BuildSwarmStatus(ctx, sp, sec)
	if err == nil && data != nil {
		if sw, ok := data["swarm"].(map[string]any); ok {
			m.SwarmData = sw
		}
	}
}

// RefreshObjects gathers object counts directly from process directories.
func (m *UIModel) RefreshObjects() {
	if m.ProjectRoot == "" {
		return
	}
	procDir := filepath.Join(m.ProjectRoot, paths.ProcessDir)
	entries, err := fileutil.ReadDir(procDir)
	if err != nil {
		return
	}

	counts := make(map[string]int)
	for _, e := range entries {
		if e.IsDir() {
			kDir := filepath.Join(procDir, e.Name())
			files, fErr := fileutil.ReadDir(kDir)
			if fErr == nil {
				c := 0
				for _, f := range files {
					if !f.IsDir() && !strings.HasPrefix(f.Name(), ".") &&
						(strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml") || strings.HasSuffix(f.Name(), ".json")) {
						c++
					}
				}
				if c > 0 {
					counts[e.Name()] = c
				}
			}
		}
	}
	m.ObjectCounts = counts
}

// RefreshScheduler queries scheduler jobs.
func (m *UIModel) RefreshScheduler(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if sp == nil {
		return
	}
	res, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind: objects.KindSchedulerJob,
	})
	if err != nil || res == nil {
		return
	}

	jobs := make([]SchedulerJobRow, 0, len(res.Objects))
	for _, obj := range res.Objects {
		id := fmt.Sprintf("%v", obj["id"])
		sch := fmt.Sprintf("%v", obj["schedule"])
		if sch == "" || sch == "<nil>" {
			sch = fmt.Sprintf("%v", obj["interval"])
		}
		if sch == "" || sch == "<nil>" {
			sch = "--"
		}

		lastRun := fmt.Sprintf("%v", obj["last_run_at"])
		if lastRun == "<nil>" || lastRun == "" {
			lastRun = "--"
		} else if len(lastRun) > 19 {
			lastRun = lastRun[:19]
		}

		nextRun := fmt.Sprintf("%v", obj["next_run_at"])
		if nextRun == "<nil>" || nextRun == "" {
			nextRun = "--"
		} else if len(nextRun) > 19 {
			nextRun = nextRun[:19]
		}

		status := fmt.Sprintf("%v", obj["last_status"])
		if status == "<nil>" || status == "" {
			status = fmt.Sprintf("%v", obj[objects.FieldKeyStatus])
		}
		if status == "" || status == "<nil>" {
			status = "active"
		}

		jobs = append(jobs, SchedulerJobRow{
			ID:        id,
			Schedule:  sch,
			LastRunAt: lastRun,
			NextRunAt: nextRun,
			Status:    status,
		})
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].ID < jobs[j].ID
	})
	m.SchedulerJobs = jobs
}
