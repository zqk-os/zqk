package agentfeed

import (
	"fmt"
	"strconv"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/nodesovereignty"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type DoctorResult struct {
	FeedHealth      string                 `json:"feed_health"`
	MCPSubscribers  int                    `json:"mcp_subscribers"`
	PeerWakeLive    bool                   `json:"peer_wake_live"`
	PeerSeatsLive   int                    `json:"peer_seats_live"`
	ContractPathsOK bool                   `json:"contract_paths_ok"`
	Issues          []string               `json:"issues,omitempty"`
	NodeSovereignty nodesovereignty.Report `json:"node_sovereignty"`
}

type DoctorOptions struct {
	ProjectRoot    string
	MCPSubscribers int
	MCPQueryErr    error
	// VendorPolicy overrides the process-wide VendorPathPolicy when non-nil.
	VendorPolicy *VendorPathPolicy
}

// InspectFeed examines the feed health, peer wake capabilities, and path hygiene.
func InspectFeed(opts DoctorOptions) DoctorResult {
	result := DoctorResult{
		FeedHealth:      "unknown",
		MCPSubscribers:  -1,
		PeerWakeLive:    false,
		ContractPathsOK: true,
	}

	// 1. Check MCP Subscribers (passed in from caller)
	if opts.MCPQueryErr != nil {
		result.Issues = append(result.Issues, "mcp_query_failed: "+opts.MCPQueryErr.Error())
	} else {
		result.MCPSubscribers = opts.MCPSubscribers
		if opts.MCPSubscribers > 0 {
			result.PeerWakeLive = true
		}
	}

	// 2. Check Feed Health (path-cache / datacell membrane)
	feedPath := datacell.AgentChatChannelEventsJSONLPath(opts.ProjectRoot)
	if size, ok := fileutil.FileSize(feedPath); ok && size > 0 {
		result.FeedHealth = "healthy"
	} else {
		result.FeedHealth = "degraded"
		result.Issues = append(result.Issues, "feed_file_missing_or_empty: "+feedPath)
	}

	// 2b. Inspect unacked / stalled message backlog (TDE-F-OBS-001)
	if f, err := loadPeerAckAwaitFile(opts.ProjectRoot); err == nil {
		stalled := 0
		now := time.Now().UTC()
		for _, a := range f.Awaits {
			if a.Status == AwaitStatusOpen {
				if t, parseErr := time.Parse(time.RFC3339, a.CreatedAt); parseErr == nil && now.Sub(t) > 10*time.Minute {
					stalled++
				}
			}
		}
		if stalled > 0 {
			result.FeedHealth = "degraded"
			result.Issues = append(result.Issues, fmt.Sprintf("feed_unacked_awaits_stalled: %d open awaits exceeding 10m threshold", stalled))
		}
	}

	// 3. Enforce project-data contract paths via VendorPathPolicy (not hardcoded vendor names).
	policy := CurrentVendorPathPolicy()
	if opts.VendorPolicy != nil {
		policy = *opts.VendorPolicy
	}
	litePolicyPath := datacell.AgentChatChannelConfigPath(opts.ProjectRoot)
	liteBody := ""
	if content, err := fileutil.ReadFile(litePolicyPath); err == nil {
		liteBody = string(content)
	}
	if leaks := policy.LeakIssues(opts.ProjectRoot, feedPath, liteBody); len(leaks) > 0 {
		result.ContractPathsOK = false
		result.Issues = append(result.Issues, leaks...)
	}

	// 4. Peer seat PID hygiene (mesh notify targets).
	// Seat PIDs must track seat-worker heartbeats.
	inspectPeerSeatPIDHygiene(opts.ProjectRoot, &result)

	// 4b. PrivilegedWriter project socket (CAS ops fail-closed without it).
	// Ops must use project-scoped PrivilegedWriter socket.
	inspectPrivilegedWriterSocket(opts.ProjectRoot, &result)

	// 5. Node sovereignty (multi-root LaunchAgent / global MCP pin).
	result.NodeSovereignty = nodesovereignty.Assess(opts.ProjectRoot)
	if result.NodeSovereignty.Severity != nodesovereignty.SeverityOK {
		result.Issues = append(result.Issues, result.NodeSovereignty.ATTN)
		for _, c := range result.NodeSovereignty.Conflicts {
			result.Issues = append(result.Issues, "node_sovereignty:"+c.Code+": "+c.Detail)
		}
		if result.FeedHealth == "healthy" {
			result.FeedHealth = "degraded"
		}
	}

	return result
}

// inspectPeerSeatPIDHygiene counts live peer seats, reconciling peer_seats.json from
// seat-worker heartbeats when seating PIDs are dead or lag the heartbeat SSOT.
func inspectPeerSeatPIDHygiene(projectRoot string, result *DoctorResult) {
	if result == nil {
		return
	}
	seats, err := LoadPeerSeats(projectRoot)
	if err != nil {
		result.Issues = append(result.Issues, "peer_seats_read_failed: "+err.Error())
		return
	}
	aliveBySeat, hbErr := listSeatWorkerAlivePIDsFn(projectRoot)
	if hbErr != nil {
		result.Issues = append(result.Issues, "seat_worker_heartbeats_read_failed: "+hbErr.Error())
	}
	needsRefresh := false
	for id, rec := range seats.Seats {
		if hb, ok := aliveBySeat[id]; ok && hb > 0 && IsPIDAlive(hb) {
			if rec.PID != hb {
				needsRefresh = true
			}
			continue
		}
		if rec.PID > 0 && !IsPIDAlive(rec.PID) {
			needsRefresh = true
		}
	}
	if needsRefresh {
		if _, rerr := RefreshPeerSeatsFromLivePIDs(projectRoot, false); rerr != nil {
			result.Issues = append(result.Issues, "peer_seats_heartbeat_reconcile_failed: "+rerr.Error())
		} else if reloaded, lerr := LoadPeerSeats(projectRoot); lerr == nil {
			seats = reloaded
		}
	}
	live := 0
	for id, rec := range seats.Seats {
		if rec.PID <= 0 {
			continue
		}
		if IsPIDAlive(rec.PID) {
			live++
			continue
		}
		result.Issues = append(result.Issues, "peer_seat_stale_pid: "+id+" pid="+itoa(rec.PID))
	}
	result.PeerSeatsLive = live
	if live > 0 {
		result.PeerWakeLive = true
	} else if len(seats.Seats) > 0 {
		result.Issues = append(result.Issues, "peer_seats_present_but_no_live_pids: run feed doctor --refresh-seats")
	}
}

// inspectPrivilegedWriterSocket reports when the PrivilegedWriter UNIX socket used by
// CAS (env override or project-scoped default) is unreachable.
func inspectPrivilegedWriterSocket(projectRoot string, result *DoctorResult) {
	if result == nil || projectRoot == "" {
		return
	}
	if storage.PrivilegedWriterSocketReachable(projectRoot) {
		return
	}
	sock := storage.PrivilegedWriterSocketPathForProject(projectRoot)
	// Issue only — do not flip feed_health (correspondence plane ≠ CAS membrane).
	result.Issues = append(result.Issues, "privileged_writer_socket_missing: "+sock)
}

func init() {
	nodesovereignty.SetFeedNotifier(func(projectRoot, actor, msg string) {
		_, _ = AppendEvent(AppendEventInput{
			ProjectRoot: projectRoot,
			Message:     msg,
			EventType:   FeedEventTypeNodeHop,
			AgentID:     actor,
			Sender:      FeedSenderCoordinator,
		})
	})
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
