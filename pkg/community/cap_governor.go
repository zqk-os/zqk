// Copyright (c) 2026 ZQK Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package community

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// CAPLoopMarkerFileName is the canonical active indicator for autonomous loops.
	CAPLoopMarkerFileName = "cap_loop_active"

	// DefaultMarkerStaleThreshold defines when an unrefreshed marker without active plans
	// is considered abandoned or stale.
	DefaultMarkerStaleThreshold = 24 * time.Hour
)

// CAPGovernorOptions configures the CAP loop lifecycle governor.
type CAPGovernorOptions struct {
	ProjectRoot string
	AgentID     string
	Vendor      string
}

// CAPGovernor enforces fail-closed state markers, anti-idle continuation hook execution,
// and zero-idle loop discipline.
type CAPGovernor struct {
	projectRoot string
	agentID     string
	vendor      string
	markerPath  string
}

// NewCAPGovernor initializes a new lifecycle governor instance.
func NewCAPGovernor(opts CAPGovernorOptions) (*CAPGovernor, error) {
	if strings.TrimSpace(opts.ProjectRoot) == "" {
		return nil, errors.New("project root is required for CAP governor")
	}
	marker := filepath.Join(opts.ProjectRoot, paths.ProjectDataDir, "state", CAPLoopMarkerFileName)
	vendor := strings.TrimSpace(opts.Vendor)
	if vendor == "" {
		vendor = "antigravity"
	}
	return &CAPGovernor{
		projectRoot: opts.ProjectRoot,
		agentID:     strings.TrimSpace(opts.AgentID),
		vendor:      vendor,
		markerPath:  marker,
	}, nil
}

// MarkerPath returns the canonical path to the marker file.
func (g *CAPGovernor) MarkerPath() string {
	return g.markerPath
}

// Activate ensures the cap_loop_active marker is present with valid permissions.
func (g *CAPGovernor) Activate() error {
	dir := filepath.Dir(g.markerPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}
	payload := fmt.Sprintf("active:pid=%d:agent=%s:ts=%s\n", os.Getpid(), g.agentID, time.Now().UTC().Format(time.RFC3339))
	return fileutil.WriteFile(g.markerPath, []byte(payload), paths.FilePerm644)
}

// Deactivate removes the cap_loop_active marker.
func (g *CAPGovernor) Deactivate() error {
	if err := os.Remove(g.markerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cap marker: %w", err)
	}
	return nil
}

// IsActive returns whether the marker file exists on disk.
func (g *CAPGovernor) IsActive() bool {
	info, err := os.Stat(g.markerPath)
	return err == nil && !info.IsDir()
}

// GovernorStatus represents the operational state evaluated by the governor.
type GovernorStatus struct {
	MarkerPresent    bool      `json:"marker_present"`
	MarkerTimestamp  time.Time `json:"marker_timestamp,omitempty"`
	ActivePlanID     string    `json:"active_plan_id,omitempty"`
	ActivePlanTitle  string    `json:"active_plan_title,omitempty"`
	ShouldContinue   bool      `json:"should_continue"`
	DecisionReason   string    `json:"decision_reason"`
	StaleMarkerClean bool      `json:"stale_marker_cleaned"`
}

// Inspect performs an operational audit of marker validity and kernel priority plans.
func (g *CAPGovernor) Inspect(ctx context.Context, sec *pkgctx.SecurityContext, sp storage.ObjectStorageProvider) (*GovernorStatus, error) {
	status := &GovernorStatus{
		MarkerPresent: g.IsActive(),
	}

	if status.MarkerPresent {
		if fi, err := os.Stat(g.markerPath); err == nil {
			status.MarkerTimestamp = fi.ModTime().UTC()
		}
	}

	// Check kernel storage for execution-facing priority plans
	if sp != nil {
		for _, planStatus := range []string{objects.ObjectStatusInProgress, objects.ObjectStatusActive} {
			filter := storage.ListFilter{
				Kind: objects.KindPriorityPlan,
				Filters: map[string]any{
					objects.FieldKeyStatus: planStatus,
				},
			}
			res, err := sp.List(ctx, sec, nil, filter)
			if err != nil || len(res.Objects) == 0 {
				continue
			}
			first := res.Objects[0]
			pID, _ := first[objects.FieldKeyID].(string)
			title, _ := first[objects.FieldKeyTitle].(string)
			pID = strings.TrimSpace(pID)
			title = strings.TrimSpace(title)
			if pID == "" {
				continue
			}
			status.ActivePlanID = pID
			status.ActivePlanTitle = title
			break
		}
	}

	// Decision Logic:
	// If an active priority plan is in flight, continuation is mandatory regardless of marker glitch
	if status.ActivePlanID != "" {
		status.ShouldContinue = true
		status.DecisionReason = fmt.Sprintf("Active priority plan in progress: %s (%s)", status.ActivePlanTitle, status.ActivePlanID)
		if !status.MarkerPresent {
			// Auto-heal missing marker when plan is in progress
			if actErr := g.Activate(); actErr != nil {
				return nil, fmt.Errorf("failed to auto-heal cap marker: %w", actErr)
			}
			status.MarkerPresent = true
		}
		return status, nil
	}

	// If marker is present but NO plan is active/in_progress, inspect age
	if status.MarkerPresent {
		age := time.Since(status.MarkerTimestamp)
		if age > DefaultMarkerStaleThreshold {
			// Stale marker quarantine / cleanup
			if deactErr := g.Deactivate(); deactErr != nil {
				return nil, fmt.Errorf("failed to quarantine stale cap marker: %w", deactErr)
			}
			status.MarkerPresent = false
			status.StaleMarkerClean = true
			status.ShouldContinue = false
			status.DecisionReason = "Stale CAP loop marker quarantined; no active priority plans detected"
			return status, nil
		}
		status.ShouldContinue = true
		status.DecisionReason = "CAP loop marker active; awaiting work allocation"
		return status, nil
	}

	status.ShouldContinue = false
	status.DecisionReason = "No active priority plan or CAP marker present; idle allowable"
	return status, nil
}
