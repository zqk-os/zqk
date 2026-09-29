package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Server provides an embedded local HTTP server for interactive web visual studio.
type Server struct {
	mu          sync.RWMutex
	httpServer  *http.Server
	listener    net.Listener
	projectRoot string
	indexer     *storage.PureGoIndexer
	logger      logging.Logger
	started     bool
}

// ServerOption configures the UI server.
type ServerOption func(*Server)

// WithLogger sets a custom logger on the UI server.
func WithLogger(logger logging.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithIndexer attaches an existing PureGoIndexer instance to the server.
func WithIndexer(indexer *storage.PureGoIndexer) ServerOption {
	return func(s *Server) {
		s.indexer = indexer
	}
}

// NewServer constructs an embedded Web Studio UI server.
func NewServer(projectRoot string, opts ...ServerOption) *Server {
	s := &Server{
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(""),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.indexer == nil {
		s.indexer = storage.NewPureGoIndexer()
		// Eagerly index the project root if available
		if projectRoot != "" {
			_, _ = s.indexer.IndexProjectDir(context.Background(), projectRoot)
		}
	}
	return s
}

// Start launches the HTTP server on addr (e.g. "127.0.0.1:0" for dynamic port or "127.0.0.1:8080").
func (s *Server) Start(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("studio: server already started")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("studio: failed to listen on %s: %w", addr, err)
	}
	s.listener = ln

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/objects", s.handleObjects)
	mux.HandleFunc("/api/objects/", s.handleObjectByID)

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.started = true

	goroutinelabels.NewGoroutine("studio_ui_http_serve", "serving studio UI HTTP traffic").
		AsControlPlane().
		StartSimple(func() {
			_ = s.httpServer.Serve(ln)
		})

	return nil
}

// Addr returns the resolved network address (host:port) that the server is listening on.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.httpServer == nil {
		return nil
	}

	err := s.httpServer.Shutdown(ctx)
	s.started = false
	return err
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "ok",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"projectRoot": s.projectRoot,
		"nodeCount":   s.indexer.NodeCount(),
	})
}

// GraphPayload defines the JSON response structure for graph visualization.
type GraphPayload struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode represents an entity in the visual graph.
type GraphNode struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	Status         string              `json:"status"`
	Title          string              `json:"title"`
	CreatedAt      string              `json:"createdAt,omitempty"`
	UpdatedAt      string              `json:"updatedAt,omitempty"`
	References     map[string][]string `json:"references,omitempty"`
	WorkstreamRefs []string            `json:"workstreamRefs,omitempty"`
}

// GraphEdge represents a directed relationship in the visual graph.
type GraphEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	kinds := []string{
		"mission", "vision", "workstream", "goal", "roadmap", "milestone",
		"priority_plan", "backlog_item", "requirement", "criteria", "test_case",
		"decision", "agent_task",
	}

	nodes := make([]GraphNode, 0)
	edges := make([]GraphEdge, 0)
	seenEdges := make(map[string]bool)

	for _, kind := range kinds {
		for _, node := range s.indexer.GetNodesByKind(kind) {
			var ca, ua string
			if !node.CreatedAt.IsZero() {
				ca = node.CreatedAt.UTC().Format(time.RFC3339)
			}
			if !node.UpdatedAt.IsZero() {
				ua = node.UpdatedAt.UTC().Format(time.RFC3339)
			}
			var wsRefs []string
			if refs, ok := node.References["workstream_refs"]; ok {
				wsRefs = refs
			}
			nodes = append(nodes, GraphNode{
				ID:             node.ID,
				Kind:           node.Kind,
				Status:         node.Status,
				Title:          node.Title,
				CreatedAt:      ca,
				UpdatedAt:      ua,
				References:     node.References,
				WorkstreamRefs: wsRefs,
			})

			for rel, targets := range node.References {
				for _, target := range targets {
					edgeKey := fmt.Sprintf("%s->%s:%s", node.ID, target, rel)
					if !seenEdges[edgeKey] {
						seenEdges[edgeKey] = true
						edges = append(edges, GraphEdge{
							Source:   node.ID,
							Target:   target,
							Relation: rel,
						})
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(GraphPayload{
		Nodes: nodes,
		Edges: edges,
	})
}

func (s *Server) handleObjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	kindFilter := r.URL.Query().Get("kind")
	results := make([]*storage.IndexedNode, 0)

	if kindFilter != "" {
		results = s.indexer.GetNodesByKind(kindFilter)
	} else {
		kinds := []string{
			"mission", "vision", "workstream", "goal", "roadmap", "milestone",
			"priority_plan", "backlog_item", "requirement", "criteria", "test_case",
			"decision", "agent_task",
		}
		for _, k := range kinds {
			results = append(results, s.indexer.GetNodesByKind(k)...)
		}
	}

	if results == nil {
		results = make([]*storage.IndexedNode, 0)
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(results)
}

func (s *Server) handleObjectByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := strings.TrimPrefix(r.URL.Path, "/api/objects/")
	if id == "" {
		http.Error(w, `{"error": "missing object id"}`, http.StatusBadRequest)
		return
	}

	node, exists := s.indexer.GetNode(id)
	if !exists {
		http.Error(w, `{"error": "object not found"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(node)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(embeddedDashboardHTML))
}

const embeddedDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>ZQK Knowledge Kernel Visual Studio</title>
  <style>
    :root {
      --bg: #0d1117;
      --card-bg: #161b22;
      --card-hover: #21262d;
      --border: #30363d;
      --border-bright: #484f58;
      --text: #c9d1d9;
      --text-muted: #8b949e;
      --text-bright: #f0f6fc;
      --accent: #58a6ff;
      --accent-glow: rgba(88, 166, 255, 0.25);
      --success: #3fb950;
      --warning: #d29922;
      --purple: #a371f7;
      --teal: #39c5bb;
      --pink: #db61a2;
      --orange: #f0883e;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background-color: var(--bg);
      color: var(--text);
      display: flex;
      flex-direction: column;
      height: 100vh;
      overflow: hidden;
      user-select: none;
    }
    header {
      padding: 10px 18px;
      background: var(--card-bg);
      border-bottom: 1px solid var(--border);
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 14px;
      flex-shrink: 0;
      z-index: 20;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .brand-icon { font-size: 18px; }
    h1 {
      margin: 0;
      font-size: 15px;
      font-weight: 600;
      color: var(--text-bright);
      white-space: nowrap;
    }
    .view-switcher {
      display: flex;
      background: #0d1117;
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 2px;
      gap: 2px;
    }
    .view-tab {
      padding: 5px 12px;
      font-size: 12px;
      font-weight: 600;
      color: var(--text-muted);
      border-radius: 4px;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 6px;
      transition: all 0.15s;
    }
    .view-tab:hover { color: var(--text-bright); }
    .view-tab.active {
      background: #21262d;
      color: var(--accent);
      box-shadow: 0 1px 3px rgba(0,0,0,0.3);
    }
    .header-controls {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-wrap: wrap;
    }
    .select-input {
      background: #0d1117;
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--text-bright);
      padding: 5px 10px;
      font-size: 12px;
      font-weight: 500;
      outline: none;
      cursor: pointer;
    }
    .select-input:focus { border-color: var(--accent); }
    .search-input {
      background: #0d1117;
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--text-bright);
      padding: 5px 10px 5px 28px;
      font-size: 12px;
      width: 180px;
      outline: none;
      transition: all 0.2s;
    }
    .search-input:focus {
      border-color: var(--accent);
      width: 240px;
    }
    .search-box { position: relative; }
    .search-icon {
      position: absolute;
      left: 8px;
      top: 50%;
      transform: translateY(-50%);
      color: var(--text-muted);
      font-size: 11px;
      pointer-events: none;
    }
    .status-pill {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      background: rgba(63, 185, 80, 0.12);
      color: var(--success);
      padding: 4px 8px;
      border-radius: 12px;
      font-size: 11px;
      font-weight: 500;
    }
    .pulse-dot {
      width: 6px;
      height: 6px;
      background: var(--success);
      border-radius: 50%;
      box-shadow: 0 0 6px var(--success);
      animation: pulse 2s infinite;
    }
    @keyframes pulse {
      0%, 100% { opacity: 1; transform: scale(1); }
      50% { opacity: 0.4; transform: scale(0.85); }
    }
    .btn {
      background: #21262d;
      color: var(--text-bright);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 5px 10px;
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 5px;
      transition: background 0.15s, border-color 0.15s;
    }
    .btn:hover { background: #30363d; border-color: var(--border-bright); }
    .btn-sm { padding: 3px 8px; font-size: 11px; }
    .btn-accent { background: var(--accent); color: #fff; border-color: var(--accent); }
    .btn-accent:hover { background: #4090ed; }

    main {
      flex: 1;
      display: grid;
      grid-template-columns: 1fr 380px;
      overflow: hidden;
      position: relative;
    }
    .content-view {
      position: relative;
      width: 100%;
      height: 100%;
      overflow: hidden;
      display: none;
    }
    .content-view.active {
      display: flex;
      flex-direction: column;
    }

    /* DAG Graph View */
    .panel-graph {
      background: radial-gradient(circle at 50% 50%, #161b22 0%, #0d1117 100%);
      flex: 1;
      position: relative;
      overflow: hidden;
    }
    .graph-toolbar {
      position: absolute;
      top: 14px;
      left: 14px;
      z-index: 10;
      display: flex;
      gap: 6px;
      background: rgba(22, 27, 34, 0.92);
      backdrop-filter: blur(8px);
      padding: 4px;
      border-radius: 8px;
      border: 1px solid var(--border);
    }
    .filter-bar {
      position: absolute;
      top: 14px;
      right: 14px;
      z-index: 10;
      display: flex;
      gap: 4px;
      background: rgba(22, 27, 34, 0.92);
      backdrop-filter: blur(8px);
      padding: 4px;
      border-radius: 8px;
      border: 1px solid var(--border);
      max-width: 60%;
      flex-wrap: wrap;
    }
    .filter-chip {
      padding: 3px 8px;
      font-size: 11px;
      border-radius: 4px;
      cursor: pointer;
      color: var(--text-muted);
      transition: all 0.15s;
    }
    .filter-chip.active, .filter-chip:hover {
      background: var(--accent);
      color: #fff;
    }
    .focus-banner {
      position: absolute;
      top: 56px;
      left: 50%;
      transform: translateX(-50%);
      z-index: 12;
      background: rgba(33, 38, 45, 0.95);
      border: 1px solid var(--accent);
      box-shadow: 0 4px 16px rgba(0,0,0,0.5), 0 0 12px var(--accent-glow);
      padding: 6px 14px;
      border-radius: 20px;
      display: flex;
      align-items: center;
      gap: 10px;
      font-size: 12px;
      color: var(--text-bright);
    }
    .graph-canvas {
      flex: 1;
      width: 100%;
      height: 100%;
      cursor: grab;
    }
    .graph-canvas.grabbing { cursor: grabbing; }

    /* SVG graph nodes & edges */
    .edge-line {
      fill: none;
      stroke: #30363d;
      stroke-width: 1.5;
      transition: stroke 0.2s, stroke-width 0.2s;
    }
    .edge-line.highlight {
      stroke: var(--accent);
      stroke-width: 2.5;
    }
    .node-group { cursor: pointer; }
    .node-bg {
      fill: #161b22;
      stroke: #30363d;
      stroke-width: 1.5;
      rx: 8;
      ry: 8;
      transition: all 0.15s;
    }
    .node-group:hover .node-bg {
      fill: #21262d;
      stroke: var(--border-bright);
    }
    .node-group.selected .node-bg {
      stroke: var(--accent);
      stroke-width: 2.5;
      filter: drop-shadow(0 0 10px var(--accent-glow));
    }
    .node-text-id {
      fill: var(--text-bright);
      font-size: 11px;
      font-weight: 600;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    .node-text-title {
      fill: var(--text);
      font-size: 11px;
    }
    .node-badge {
      font-size: 9px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }

    /* Gantt / Timeline View */
    .gantt-container {
      flex: 1;
      display: flex;
      flex-direction: column;
      background: var(--bg);
      overflow: hidden;
    }
    .gantt-header-row {
      display: grid;
      grid-template-columns: 360px 1fr;
      background: var(--card-bg);
      border-bottom: 1px solid var(--border);
      height: 44px;
      flex-shrink: 0;
    }
    .gantt-header-title {
      padding: 12px 16px;
      font-size: 12px;
      font-weight: 600;
      color: var(--text-bright);
      border-right: 1px solid var(--border);
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .gantt-timeline-ticks {
      display: flex;
      align-items: center;
      overflow: hidden;
      padding: 0 12px;
      position: relative;
    }
    .gantt-tick {
      flex: 1;
      text-align: center;
      font-size: 11px;
      color: var(--text-muted);
      border-left: 1px dashed var(--border);
      height: 100%;
      display: flex;
      align-items: center;
      justify-content: center;
    }
    .gantt-body {
      flex: 1;
      overflow-y: auto;
      overflow-x: hidden;
    }
    .gantt-section-header {
      background: #11151c;
      padding: 8px 16px;
      font-size: 12px;
      font-weight: 700;
      color: var(--accent);
      border-bottom: 1px solid var(--border);
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .gantt-row {
      display: grid;
      grid-template-columns: 360px 1fr;
      border-bottom: 1px solid rgba(48, 54, 61, 0.4);
      min-height: 40px;
      align-items: center;
      transition: background 0.1s;
      cursor: pointer;
    }
    .gantt-row:hover { background: rgba(88, 166, 255, 0.04); }
    .gantt-row.selected { background: rgba(88, 166, 255, 0.08); }
    .gantt-row-info {
      padding: 6px 16px;
      border-right: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      gap: 2px;
      overflow: hidden;
    }
    .gantt-row-title-line {
      display: flex;
      align-items: center;
      gap: 6px;
      overflow: hidden;
    }
    .gantt-row-id {
      font-size: 11px;
      font-weight: 600;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      color: var(--text-bright);
    }
    .gantt-row-title {
      font-size: 11px;
      color: var(--text-muted);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .gantt-bar-cell {
      padding: 6px 14px;
      position: relative;
      height: 100%;
      display: flex;
      align-items: center;
    }
    .gantt-bar {
      height: 22px;
      border-radius: 4px;
      display: flex;
      align-items: center;
      padding: 0 8px;
      font-size: 10px;
      font-weight: 600;
      color: #fff;
      position: absolute;
      transition: transform 0.15s, box-shadow 0.15s;
    }
    .gantt-bar:hover {
      transform: scaleY(1.15);
      box-shadow: 0 0 8px rgba(0,0,0,0.5);
      z-index: 5;
    }
    .gantt-bar-complete {
      background: linear-gradient(90deg, #238636 0%, #2ea043 100%);
      border: 1px solid #3fb950;
    }
    .gantt-bar-inprogress {
      background: linear-gradient(90deg, #1f6feb 0%, #388bfd 100%);
      border: 1px solid #58a6ff;
    }
    .gantt-bar-planned {
      background: #21262d;
      border: 1px solid var(--border-bright);
      color: var(--text-muted);
    }
    .gantt-bar-testing {
      background: linear-gradient(90deg, #8957e5 0%, #a371f7 100%);
      border: 1px solid #bc8cff;
    }

    /* Right Side Panel */
    .sidebar-panel {
      background: var(--card-bg);
      border-left: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
    .tab-bar {
      display: flex;
      border-bottom: 1px solid var(--border);
      background: #11151c;
      flex-shrink: 0;
    }
    .tab-btn {
      flex: 1;
      padding: 10px 14px;
      font-size: 12px;
      font-weight: 600;
      text-align: center;
      color: var(--text-muted);
      cursor: pointer;
      border-bottom: 2px solid transparent;
      transition: color 0.15s, border-color 0.15s;
    }
    .tab-btn.active {
      color: var(--text-bright);
      border-bottom-color: var(--accent);
      background: var(--card-bg);
    }
    .tab-content {
      flex: 1;
      overflow-y: auto;
      padding: 14px;
      display: none;
    }
    .tab-content.active { display: block; }

    /* Object Card List */
    .node-card {
      padding: 10px 12px;
      margin-bottom: 8px;
      background: #1c2128;
      border: 1px solid var(--border);
      border-radius: 6px;
      cursor: pointer;
      transition: all 0.12s;
    }
    .node-card:hover {
      background: var(--card-hover);
      border-color: var(--border-bright);
      transform: translateX(2px);
    }
    .node-card.selected {
      border-color: var(--accent);
      background: rgba(88, 166, 255, 0.08);
    }
    .node-card-top {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 4px;
    }
    .kind-pill {
      font-size: 10px;
      font-weight: 600;
      text-transform: uppercase;
      padding: 2px 6px;
      border-radius: 4px;
    }
    .kind-mission, .kind-vision { background: rgba(163, 113, 247, 0.2); color: var(--purple); }
    .kind-workstream { background: rgba(240, 136, 62, 0.2); color: var(--orange); }
    .kind-goal, .kind-roadmap { background: rgba(63, 185, 80, 0.2); color: var(--success); }
    .kind-priority_plan, .kind-milestone { background: rgba(88, 166, 255, 0.2); color: var(--accent); }
    .kind-backlog_item { background: rgba(57, 197, 187, 0.2); color: var(--teal); }
    .kind-requirement { background: rgba(210, 153, 34, 0.2); color: var(--warning); }
    .kind-criteria { background: rgba(63, 185, 80, 0.2); color: var(--success); }
    .kind-test_case { background: rgba(219, 97, 162, 0.2); color: var(--pink); }
    .kind-default { background: #30363d; color: var(--text-muted); }
    .status-pill-mini { font-size: 10px; color: var(--text-muted); }

    /* Inspector */
    .inspector-header {
      padding-bottom: 12px;
      border-bottom: 1px solid var(--border);
      margin-bottom: 14px;
    }
    .inspector-id-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-top: 8px;
    }
    .inspector-id {
      font-size: 13px;
      font-weight: 700;
      color: var(--text-bright);
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      word-break: break-all;
    }
    .inspector-title {
      font-size: 13px;
      font-weight: 500;
      color: var(--text);
      margin-top: 6px;
      line-height: 1.4;
    }
    .inspector-actions {
      display: flex;
      gap: 6px;
      margin-top: 10px;
    }
    .section-title {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.5px;
      color: var(--text-muted);
      margin: 14px 0 6px 0;
    }
    .ref-chip {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      font-size: 11px;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      background: #21262d;
      border: 1px solid var(--border);
      padding: 3px 8px;
      border-radius: 4px;
      color: var(--accent);
      cursor: pointer;
      margin: 2px 4px 2px 0;
      transition: all 0.15s;
    }
    .ref-chip:hover {
      background: #30363d;
      border-color: var(--accent);
      color: #fff;
    }
    .code-box {
      background: #0d1117;
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 10px;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 11px;
      color: var(--text);
      overflow-x: auto;
      max-height: 240px;
      white-space: pre-wrap;
      word-break: break-all;
    }
    .empty-state {
      padding: 32px 16px;
      text-align: center;
      color: var(--text-muted);
    }
    .empty-state-icon { font-size: 32px; margin-bottom: 8px; }
    .empty-state-title {
      font-size: 14px;
      font-weight: 600;
      color: var(--text-bright);
      margin-bottom: 4px;
    }
  </style>
</head>
<body>
  <header>
    <div class="brand">
      <span class="brand-icon">⚡</span>
      <h1>ZQK Knowledge Kernel Visual Studio</h1>
    </div>

    <!-- View Mode Switcher -->
    <div class="view-switcher">
      <div class="view-tab active" id="tab-nav-dag" onclick="switchMainView('dag')">☊ DAG Graph</div>
      <div class="view-tab" id="tab-nav-gantt" onclick="switchMainView('gantt')">▤ Timeline & Gantt</div>
    </div>

    <div class="header-controls">
      <!-- Workstream Filter Dropdown -->
      <select id="workstream-filter" class="select-input" onchange="onWorkstreamChange()" title="Filter by Workstream">
        <option value="all">🌐 All Workstreams</option>
      </select>

      <!-- Density Filter -->
      <select id="density-filter" class="select-input" onchange="onDensityChange()" title="Control Graph Density">
        <option value="backbone">Backbone (Plans & Milestones)</option>
        <option value="execution" selected>Execution (+ Backlog Items)</option>
        <option value="all">Full Mesh (All Objects)</option>
      </select>

      <div class="search-box">
        <span class="search-icon">🔍</span>
        <input type="text" id="global-search" class="search-input" placeholder="Search (press /) ...">
      </div>

      <div class="status-pill" id="live-indicator">
        <span class="pulse-dot"></span>
        <span>Connected</span>
      </div>

      <button class="btn" id="btn-refresh" onclick="loadDAG()">⟳ Refresh</button>
    </div>
  </header>

  <main>
    <!-- View 1: Interactive DAG Graph -->
    <div class="content-view active" id="view-dag">
      <div class="panel-graph" id="graph-panel">
        <div class="panel-header" style="display:none;" id="dag-panel-title">Interactive Knowledge Graph DAG</div>

        <!-- Zoom Controls -->
        <div class="graph-toolbar">
          <button class="btn" onclick="zoomIn()" title="Zoom In">+</button>
          <button class="btn" onclick="zoomOut()" title="Zoom Out">-</button>
          <button class="btn" onclick="resetZoom()" title="Reset Zoom">⟲</button>
          <button class="btn" onclick="fitGraph()" title="Fit to Screen">⛶</button>
        </div>

        <!-- Focus Subgraph Banner -->
        <div id="focus-banner" class="focus-banner" style="display:none;">
          <span id="focus-banner-text">🎯 Focused Subgraph: </span>
          <button class="btn btn-sm btn-accent" onclick="clearFocus()">✕ Show All</button>
        </div>

        <!-- Kind Filter Chips -->
        <div class="filter-bar" id="kind-filters">
          <span class="filter-chip active" data-kind="all" onclick="filterKind('all')">All</span>
          <span class="filter-chip" data-kind="workstream" onclick="filterKind('workstream')">Workstreams</span>
          <span class="filter-chip" data-kind="goal" onclick="filterKind('goal')">Goals</span>
          <span class="filter-chip" data-kind="priority_plan" onclick="filterKind('priority_plan')">Plans</span>
          <span class="filter-chip" data-kind="milestone" onclick="filterKind('milestone')">Milestones</span>
          <span class="filter-chip" data-kind="backlog_item" onclick="filterKind('backlog_item')">BLIs</span>
        </div>

        <svg id="dag-svg" class="graph-canvas">
          <defs>
            <marker id="arrow" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 1 L 10 5 L 0 9 z" fill="#484f58" />
            </marker>
            <marker id="arrow-highlight" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 1 L 10 5 L 0 9 z" fill="#58a6ff" />
            </marker>
          </defs>
          <g id="viewport">
            <g id="edges-layer"></g>
            <g id="nodes-layer"></g>
          </g>
        </svg>
      </div>
    </div>

    <!-- View 2: Timeline & Gantt View -->
    <div class="content-view" id="view-gantt">
      <div class="gantt-container">
        <div class="gantt-header-row">
          <div class="gantt-header-title">
            <span>Workstream & Execution Plan</span>
            <span id="gantt-task-count" style="font-size: 11px; color: var(--text-muted);">0 items</span>
          </div>
          <div class="gantt-timeline-ticks" id="gantt-timeline-ticks">
            <!-- Dynamic date ticks -->
          </div>
        </div>
        <div class="gantt-body" id="gantt-body">
          <!-- Dynamic grouped rows -->
        </div>
      </div>
    </div>

    <!-- Right: Multi-Tab Sidebar (Shared) -->
    <div class="sidebar-panel">
      <div class="tab-bar">
        <div class="tab-btn active" id="tab-btn-inspector" onclick="switchTab('inspector')">Inspector</div>
        <div class="tab-btn" id="tab-btn-objects" onclick="switchTab('objects')">Kernel Objects (<span id="objects-count">0</span>)</div>
      </div>

      <!-- Tab 1: Selected Object Inspector -->
      <div class="tab-content active" id="tab-inspector">
        <div id="inspector-content">
          <div class="empty-state">
            <div class="empty-state-icon">🎯</div>
            <div class="empty-state-title">No Object Selected</div>
            <div style="font-size: 12px; margin-top: 4px;">Click any item in the DAG or Gantt timeline to inspect its causal dependencies.</div>
          </div>
        </div>
      </div>

      <!-- Tab 2: Kernel Objects List -->
      <div class="tab-content" id="tab-objects">
        <div id="objects-list-container">
          <div style="padding: 16px; color: var(--text-muted);">Loading objects...</div>
        </div>
      </div>
    </div>
  </main>

  <script>
    // State
    let graphData = { nodes: [], edges: [] };
    let filteredNodes = [];
    let selectedNodeId = null;
    let focusedNodeId = null; // Causal subgraph focus
    let activeKindFilter = 'all';
    let activeWorkstreamFilter = 'all';
    let activeDensity = 'execution'; // 'backbone', 'execution', 'all'
    let currentMainView = 'dag'; // 'dag' or 'gantt'
    let searchQuery = '';

    // Transform state
    let scale = 1.0;
    let translateX = 40;
    let translateY = 40;
    let isPanning = false;
    let panStartX = 0;
    let panStartY = 0;

    let nodePositions = new Map();
    const svg = document.getElementById('dag-svg');
    const viewport = document.getElementById('viewport');
    const nodesLayer = document.getElementById('nodes-layer');
    const edgesLayer = document.getElementById('edges-layer');

    function updateTransform() {
      viewport.setAttribute('transform', 'translate(' + translateX + ',' + translateY + ') scale(' + scale + ')');
    }

    function zoomIn() { scale = Math.min(scale * 1.25, 3.5); updateTransform(); }
    function zoomOut() { scale = Math.max(scale / 1.25, 0.15); updateTransform(); }
    function resetZoom() { scale = 1.0; translateX = 40; translateY = 40; updateTransform(); }

    function fitGraph() {
      if (nodePositions.size === 0) return;
      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      nodePositions.forEach(p => {
        minX = Math.min(minX, p.x);
        minY = Math.min(minY, p.y);
        maxX = Math.max(maxX, p.x + p.width);
        maxY = Math.max(maxY, p.y + p.height);
      });
      const rect = svg.getBoundingClientRect();
      if (!rect || rect.width <= 0 || rect.height <= 0) return;
      const contentW = maxX - minX + 80;
      const contentH = maxY - minY + 80;
      scale = Math.min(rect.width / contentW, rect.height / contentH, 1.2);
      if (!isFinite(scale) || scale <= 0) scale = 1.0;
      translateX = (rect.width - contentW * scale) / 2 - minX * scale + 40 * scale;
      translateY = (rect.height - contentH * scale) / 2 - minY * scale + 40 * scale;
      if (!isFinite(translateX)) translateX = 40;
      if (!isFinite(translateY)) translateY = 40;
      updateTransform();
    }

    // Pan interaction
    svg.addEventListener('mousedown', (e) => {
      if (e.target.closest('.node-group')) return;
      isPanning = true;
      panStartX = e.clientX - translateX;
      panStartY = e.clientY - translateY;
      svg.classList.add('grabbing');
    });

    svg.addEventListener('dblclick', (e) => {
      if (e.target.closest('.node-group')) return;
      clearFocus();
      resetZoom();
    });

    window.addEventListener('mousemove', (e) => {
      if (!isPanning) return;
      translateX = e.clientX - panStartX;
      translateY = e.clientY - panStartY;
      updateTransform();
    });

    window.addEventListener('mouseup', () => {
      isPanning = false;
      svg.classList.remove('grabbing');
    });

    svg.addEventListener('wheel', (e) => {
      e.preventDefault();
      const zoomFactor = e.deltaY < 0 ? 1.1 : 0.9;
      const rect = svg.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;
      const mouseY = e.clientY - rect.top;
      const newScale = Math.min(Math.max(scale * zoomFactor, 0.1), 4.0);
      translateX = mouseX - (mouseX - translateX) * (newScale / scale);
      translateY = mouseY - (mouseY - translateY) * (newScale / scale);
      scale = newScale;
      updateTransform();
    }, { passive: false });

    // Keyboard shortcut / for search
    window.addEventListener('keydown', (e) => {
      if (e.key === '/' && document.activeElement.tagName !== 'INPUT') {
        e.preventDefault();
        document.getElementById('global-search').focus();
      }
      if (e.key === 'Escape') {
        clearFocus();
      }
    });

    document.getElementById('global-search').addEventListener('input', (e) => {
      searchQuery = e.target.value.toLowerCase().trim();
      applyFilters();
    });

    function switchMainView(view) {
      currentMainView = view;
      document.getElementById('tab-nav-dag').classList.toggle('active', view === 'dag');
      document.getElementById('tab-nav-gantt').classList.toggle('active', view === 'gantt');
      document.getElementById('view-dag').classList.toggle('active', view === 'dag');
      document.getElementById('view-gantt').classList.toggle('active', view === 'gantt');
      if (view === 'gantt') {
        renderGantt();
      } else {
        fitGraph();
      }
    }

    function onWorkstreamChange() {
      activeWorkstreamFilter = document.getElementById('workstream-filter').value;
      applyFilters();
    }

    function onDensityChange() {
      activeDensity = document.getElementById('density-filter').value;
      applyFilters();
    }

    function filterKind(k) {
      activeKindFilter = k;
      document.querySelectorAll('#kind-filters .filter-chip').forEach(el => {
        el.classList.toggle('active', el.getAttribute('data-kind') === k);
      });
      applyFilters();
    }

    function switchTab(tab) {
      document.getElementById('tab-btn-inspector').classList.toggle('active', tab === 'inspector');
      document.getElementById('tab-btn-objects').classList.toggle('active', tab === 'objects');
      document.getElementById('tab-inspector').classList.toggle('active', tab === 'inspector');
      document.getElementById('tab-objects').classList.toggle('active', tab === 'objects');
    }

    function getKindColor(kind) {
      switch(kind) {
        case 'mission': case 'vision': return 'var(--purple)';
        case 'workstream': return 'var(--orange)';
        case 'goal': case 'roadmap': return 'var(--success)';
        case 'priority_plan': case 'milestone': return 'var(--accent)';
        case 'backlog_item': return 'var(--teal)';
        case 'requirement': return 'var(--warning)';
        case 'criteria': return 'var(--success)';
        case 'test_case': return 'var(--pink)';
        default: return 'var(--text-muted)';
      }
    }

    function getKindTier(kind) {
      switch(kind) {
        case 'mission': case 'vision': return 0;
        case 'workstream': return 1;
        case 'goal': case 'roadmap': return 2;
        case 'milestone': case 'priority_plan': return 3;
        case 'backlog_item': return 4;
        case 'requirement': return 5;
        case 'criteria': return 6;
        case 'test_case': return 7;
        default: return 4;
      }
    }

    // Causal Subgraph Isolation
    function focusNode(nodeId) {
      focusedNodeId = nodeId;
      const banner = document.getElementById('focus-banner');
      const text = document.getElementById('focus-banner-text');
      banner.style.display = 'flex';
      text.textContent = '🎯 Focused Subgraph: ' + nodeId;
      applyFilters();
      fitGraph();
    }

    function clearFocus() {
      focusedNodeId = null;
      document.getElementById('focus-banner').style.display = 'none';
      applyFilters();
    }

    function getCausalSubtreeNodeIds(centerId) {
      const activeIds = new Set();
      activeIds.add(centerId);

      // Build adjacency maps
      const outgoing = new Map(); // id -> set of target ids
      const incoming = new Map(); // id -> set of source ids

      graphData.edges.forEach(e => {
        if (!outgoing.has(e.source)) outgoing.set(e.source, new Set());
        outgoing.get(e.source).add(e.target);

        if (!incoming.has(e.target)) incoming.set(e.target, new Set());
        incoming.get(e.target).add(e.source);
      });

      // BFS upstream (ancestors)
      let queue = [centerId];
      while (queue.length > 0) {
        const cur = queue.shift();
        const parents = incoming.get(cur);
        if (parents) {
          parents.forEach(p => {
            if (!activeIds.has(p)) {
              activeIds.add(p);
              queue.push(p);
            }
          });
        }
      }

      // BFS downstream (descendants)
      queue = [centerId];
      while (queue.length > 0) {
        const cur = queue.shift();
        const children = outgoing.get(cur);
        if (children) {
          children.forEach(c => {
            if (!activeIds.has(c)) {
              activeIds.add(c);
              queue.push(c);
            }
          });
        }
      }

      return activeIds;
    }

    async function loadDAG() {
      try {
        const res = await fetch('/api/graph');
        const data = await res.json();
        graphData = data || { nodes: [], edges: [] };
        if (!graphData.nodes) graphData.nodes = [];
        if (!graphData.edges) graphData.edges = [];

        // Propagate workstream associations downstream along DAG edges
        const wsMap = new Map();
        graphData.nodes.forEach(n => {
          if (n.kind === 'workstream') {
            if (!wsMap.has(n.id)) wsMap.set(n.id, new Set());
            wsMap.get(n.id).add(n.id);
          }
          if (n.workstreamRefs) {
            if (!wsMap.has(n.id)) wsMap.set(n.id, new Set());
            n.workstreamRefs.forEach(w => wsMap.get(n.id).add(w));
          }
        });
        const outgoing = new Map();
        graphData.edges.forEach(e => {
          if (!outgoing.has(e.source)) outgoing.set(e.source, []);
          outgoing.get(e.source).push(e.target);
        });
        let changed = true;
        let iters = 0;
        while (changed && iters < 12) {
          changed = false;
          iters++;
          wsMap.forEach((wsSet, sourceId) => {
            const targets = outgoing.get(sourceId) || [];
            targets.forEach(targetId => {
              if (!wsMap.has(targetId)) wsMap.set(targetId, new Set());
              const targetSet = wsMap.get(targetId);
              const prevSize = targetSet.size;
              wsSet.forEach(w => targetSet.add(w));
              if (targetSet.size > prevSize) changed = true;
            });
          });
        }
        graphData.nodes.forEach(n => {
          if (wsMap.has(n.id) && wsMap.get(n.id).size > 0) {
            n.workstreamRefs = Array.from(wsMap.get(n.id));
          }
        });

        populateWorkstreamsDropdown();
        applyFilters();

        if (graphData.nodes.length > 0 && !selectedNodeId) {
          selectNode(graphData.nodes[0].id, false);
        }
        setTimeout(() => fitGraph(), 100);
      } catch (err) {
        console.error('Failed to load DAG', err);
      }
    }

    function populateWorkstreamsDropdown() {
      const select = document.getElementById('workstream-filter');
      const wsNodes = graphData.nodes.filter(n => n.kind === 'workstream');
      const current = select.value;

      select.innerHTML = '<option value="all">🌐 All Workstreams</option>' +
        wsNodes.map(w => '<option value="' + w.id + '">' + w.id + ' (' + (w.title || w.kind) + ')</option>').join('');

      if (current && wsNodes.some(w => w.id === current)) {
        select.value = current;
      }
    }

    function applyFilters() {
      // 1. Causal Subgraph Isolation
      let focusSet = null;
      if (focusedNodeId) {
        focusSet = getCausalSubtreeNodeIds(focusedNodeId);
      }

      filteredNodes = graphData.nodes.filter(n => {
        // Subgraph focus takes priority
        if (focusSet && !focusSet.has(n.id)) return false;

        // Workstream Filter
        if (activeWorkstreamFilter !== 'all') {
          const isWS = n.id === activeWorkstreamFilter;
          const refsWS = n.workstreamRefs && n.workstreamRefs.includes(activeWorkstreamFilter);
          if (!isWS && !refsWS) return false;
        }

        // Density Filter
        if (!focusedNodeId) {
          const tier = getKindTier(n.kind);
          if (activeDensity === 'backbone' && tier > 3) return false;
          if (activeDensity === 'execution' && tier > 4) return false;
        }

        // Kind Filter Pill
        const matchKind = activeKindFilter === 'all' || n.kind === activeKindFilter;
        if (!matchKind) return false;

        // Search Filter
        const matchSearch = !searchQuery ||
          n.id.toLowerCase().includes(searchQuery) ||
          (n.title && n.title.toLowerCase().includes(searchQuery)) ||
          n.kind.toLowerCase().includes(searchQuery);

        return matchSearch;
      });

      document.getElementById('objects-count').textContent = filteredNodes.length;

      calculateLayout();
      renderGraph();
      renderObjectsList();
      if (currentMainView === 'gantt') {
        renderGantt();
      }
    }

    function calculateLayout() {
      nodePositions.clear();
      const tiers = [[], [], [], [], [], [], [], []];
      const NODE_WIDTH = 220;
      const NODE_HEIGHT = 68;
      const COL_GAP = 90;
      const ROW_GAP = 28;

      filteredNodes.forEach(node => {
        const t = Math.min(getKindTier(node.kind), 7);
        tiers[t].push(node);
      });

      let colIndex = 0;
      tiers.forEach((tierNodes) => {
        if (tierNodes.length === 0) return;
        tierNodes.forEach((node, rowIndex) => {
          const x = colIndex * (NODE_WIDTH + COL_GAP);
          const y = rowIndex * (NODE_HEIGHT + ROW_GAP);
          nodePositions.set(node.id, { x, y, width: NODE_WIDTH, height: NODE_HEIGHT });
        });
        colIndex++;
      });
    }

    function renderGraph() {
      nodesLayer.innerHTML = '';
      edgesLayer.innerHTML = '';

      if (filteredNodes.length === 0) {
        nodesLayer.innerHTML = '<text x="60" y="100" fill="var(--text-muted)" font-size="14">No nodes match the active filter or focus criteria.</text>';
        return;
      }

      const activeSet = new Set(filteredNodes.map(n => n.id));

      // Draw Edges
      const renderedEdges = new Set();
      graphData.edges.forEach(edge => {
        if (!activeSet.has(edge.source) || !activeSet.has(edge.target)) return;
        const p1 = nodePositions.get(edge.source);
        const p2 = nodePositions.get(edge.target);
        if (!p1 || !p2) return;

        const edgeKey = edge.source + '->' + edge.target;
        if (renderedEdges.has(edgeKey)) return;
        renderedEdges.add(edgeKey);

        const x1 = p1.x + p1.width;
        const y1 = p1.y + p1.height / 2;
        const x2 = p2.x;
        const y2 = p2.y + p2.height / 2;
        const dx = Math.abs(x2 - x1) / 2;

        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', 'M ' + x1 + ' ' + y1 + ' C ' + (x1 + dx) + ' ' + y1 + ', ' + (x2 - dx) + ' ' + y2 + ', ' + x2 + ' ' + y2);
        path.setAttribute('class', 'edge-line' + ((selectedNodeId && (edge.source === selectedNodeId || edge.target === selectedNodeId)) ? ' highlight' : ''));
        path.setAttribute('marker-end', (selectedNodeId && (edge.source === selectedNodeId || edge.target === selectedNodeId)) ? 'url(#arrow-highlight)' : 'url(#arrow)');

        edgesLayer.appendChild(path);
      });

      // Draw Nodes
      filteredNodes.forEach(node => {
        const pos = nodePositions.get(node.id);
        if (!pos) return;

        const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        g.setAttribute('class', 'node-group' + (selectedNodeId === node.id ? ' selected' : ''));
        g.setAttribute('transform', 'translate(' + pos.x + ',' + pos.y + ')');
        g.setAttribute('data-id', node.id);

        const color = getKindColor(node.kind);

        const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        rect.setAttribute('class', 'node-bg');
        rect.setAttribute('width', pos.width);
        rect.setAttribute('height', pos.height);

        const bar = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        bar.setAttribute('x', 0);
        bar.setAttribute('y', 0);
        bar.setAttribute('width', 4);
        bar.setAttribute('height', pos.height);
        bar.setAttribute('fill', color);
        bar.setAttribute('rx', 2);

        const kindText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        kindText.setAttribute('x', 14);
        kindText.setAttribute('y', 18);
        kindText.setAttribute('class', 'node-badge');
        kindText.setAttribute('fill', color);
        kindText.textContent = node.kind.replace('_', ' ');

        const statusText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        statusText.setAttribute('x', pos.width - 12);
        statusText.setAttribute('y', 18);
        statusText.setAttribute('text-anchor', 'end');
        statusText.setAttribute('class', 'node-badge');
        statusText.setAttribute('fill', 'var(--text-muted)');
        statusText.textContent = node.status || '';

        const idText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        idText.setAttribute('x', 14);
        idText.setAttribute('y', 36);
        idText.setAttribute('class', 'node-text-id');
        idText.textContent = node.id.length > 24 ? node.id.slice(0, 22) + '…' : node.id;

        const titleText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        titleText.setAttribute('x', 14);
        titleText.setAttribute('y', 52);
        titleText.setAttribute('class', 'node-text-title');
        const rawTitle = node.title || '';
        titleText.textContent = rawTitle.length > 28 ? rawTitle.slice(0, 26) + '…' : rawTitle;

        g.appendChild(rect);
        g.appendChild(bar);
        g.appendChild(kindText);
        g.appendChild(statusText);
        g.appendChild(idText);
        g.appendChild(titleText);

        g.addEventListener('click', (e) => {
          e.stopPropagation();
          selectNode(node.id, true);
          // Auto-focus on click: isolate subgraph
          focusNode(node.id);
        });

        nodesLayer.appendChild(g);
      });
    }

    // Gantt / Timeline Renderer
    function renderGantt() {
      const body = document.getElementById('gantt-body');
      const ticks = document.getElementById('gantt-timeline-ticks');
      body.innerHTML = '';
      ticks.innerHTML = '';

      // Collect execution items: workstreams, priority plans, milestones, backlog items
      const validKinds = new Set(['workstream', 'milestone', 'priority_plan', 'backlog_item']);
      const items = filteredNodes.filter(n => validKinds.has(n.kind));
      document.getElementById('gantt-task-count').textContent = items.length + ' items';

      if (items.length === 0) {
        body.innerHTML = '<div style="padding: 40px; color: var(--text-muted); text-align: center;">No execution items to display in timeline. Switch to "Full Mesh" or select "All Workstreams".</div>';
        return;
      }

      // Timeline scale: 6 dynamic checkpoints
      const tickLabels = ['Stage 1: Inception', 'Stage 2: Objectives', 'Stage 3: Planning', 'Stage 4: Execution', 'Stage 5: Verification', 'Stage 6: Done'];
      ticks.innerHTML = tickLabels.map(t => '<div class="gantt-tick">' + t + '</div>').join('');

      // Group items by Workstream or Milestone
      const groups = new Map();
      const unassigned = [];

      items.forEach(item => {
        let ws = (item.workstreamRefs && item.workstreamRefs[0]) || (item.kind === 'workstream' ? item.id : null);
        if (!ws && item.kind === 'milestone') ws = 'Milestones';
        if (ws) {
          if (!groups.has(ws)) groups.set(ws, []);
          groups.get(ws).push(item);
        } else {
          unassigned.push(item);
        }
      });
      if (unassigned.length > 0) groups.set('General Tasks', unassigned);

      // Render rows
      groups.forEach((groupItems, groupName) => {
        const header = document.createElement('div');
        header.className = 'gantt-section-header';
        header.innerHTML = '<span>⚡ ' + groupName + '</span> <span style="font-size: 11px; font-weight: normal; color: var(--text-muted);">(' + groupItems.length + ' tasks)</span>';
        body.appendChild(header);

        groupItems.forEach(item => {
          const row = document.createElement('div');
          row.className = 'gantt-row' + (item.id === selectedNodeId ? ' selected' : '');

          const kindClass = 'kind-' + item.kind;

          // Left info cell
          const info = document.createElement('div');
          info.className = 'gantt-row-info';
          info.innerHTML =
            '<div class="gantt-row-title-line">' +
              '<span class="kind-pill ' + kindClass + '">' + item.kind.replace('_', ' ') + '</span>' +
              '<span class="gantt-row-id">' + item.id + '</span>' +
            '</div>' +
            '<div class="gantt-row-title">' + (item.title || '') + '</div>';

          // Right timeline bar cell
          const cell = document.createElement('div');
          cell.className = 'gantt-bar-cell';

          // Position calculation based on kind tier & status
          let leftPercent = 5;
          let widthPercent = 30;
          const status = (item.status || 'planned').toLowerCase();

          if (item.kind === 'workstream') {
            leftPercent = 2;
            widthPercent = 95;
          } else if (item.kind === 'milestone') {
            leftPercent = 10;
            widthPercent = 75;
          } else if (item.kind === 'priority_plan') {
            leftPercent = 25;
            widthPercent = 60;
          } else {
            // Backlog Item
            if (status === 'complete') {
              leftPercent = 20;
              widthPercent = 75;
            } else if (status === 'in_progress') {
              leftPercent = 45;
              widthPercent = 35;
            } else {
              leftPercent = 60;
              widthPercent = 30;
            }
          }

          let barClass = 'gantt-bar-planned';
          if (status === 'complete' || status === 'verified') barClass = 'gantt-bar-complete';
          else if (status === 'in_progress' || status === 'active') barClass = 'gantt-bar-inprogress';
          else if (status === 'testing' || status === 'metrics_captured') barClass = 'gantt-bar-testing';

          const bar = document.createElement('div');
          bar.className = 'gantt-bar ' + barClass;
          bar.style.left = leftPercent + '%';
          bar.style.width = widthPercent + '%';
          bar.textContent = status.replace('_', ' ');

          cell.appendChild(bar);
          row.appendChild(info);
          row.appendChild(cell);

          row.addEventListener('click', () => {
            selectNode(item.id, true);
          });

          body.appendChild(row);
        });
      });
    }

    function renderObjectsList() {
      const container = document.getElementById('objects-list-container');
      container.innerHTML = '';
      if (filteredNodes.length === 0) {
        container.innerHTML = '<div style="padding: 24px; color: var(--text-muted); text-align: center;">No matching objects.</div>';
        return;
      }

      filteredNodes.forEach(n => {
        const isSel = n.id === selectedNodeId;
        const kindClass = 'kind-' + n.kind;
        const card = document.createElement('div');
        card.className = 'node-card' + (isSel ? ' selected' : '');
        card.innerHTML =
          '<div class="node-card-top">' +
            '<span class="kind-pill ' + kindClass + '">' + n.kind.replace('_', ' ') + '</span>' +
            '<span class="status-pill-mini">' + (n.status || '') + '</span>' +
          '</div>' +
          '<div class="node-card-id">' + n.id + '</div>' +
          '<div class="node-card-title">' + (n.title || '') + '</div>';
        card.addEventListener('click', () => selectNode(n.id, true));
        container.appendChild(card);
      });
    }

    async function selectNode(nodeId, panTo) {
      selectedNodeId = nodeId;
      renderGraph();
      renderObjectsList();
      if (currentMainView === 'gantt') renderGantt();
      switchTab('inspector');

      const inspector = document.getElementById('inspector-content');
      inspector.innerHTML = '<div style="padding: 24px; color: var(--text-muted);">Fetching object details...</div>';

      try {
        const res = await fetch('/api/objects/' + encodeURIComponent(nodeId));
        if (!res.ok) throw new Error('Object not found');
        const node = await res.json();
        renderInspector(node);

        if (panTo && currentMainView === 'dag') {
          const pos = nodePositions.get(nodeId);
          if (pos) {
            const rect = svg.getBoundingClientRect();
            translateX = rect.width / 2 - (pos.x + pos.width / 2) * scale;
            translateY = rect.height / 2 - (pos.y + pos.height / 2) * scale;
            updateTransform();
          }
        }
      } catch (err) {
        const gNode = graphData.nodes.find(n => n.id === nodeId);
        if (gNode) renderInspector(gNode);
      }
    }

    function renderInspector(node) {
      const inspector = document.getElementById('inspector-content');
      inspector.innerHTML = '';
      const kindClass = 'kind-' + (node.kind || 'default');

      const header = document.createElement('div');
      header.className = 'inspector-header';

      const topRow = document.createElement('div');
      topRow.style.cssText = 'display: flex; justify-content: space-between; align-items: center;';
      topRow.innerHTML =
        '<span class="kind-pill ' + kindClass + '">' + (node.kind || 'object').replace('_', ' ') + '</span>' +
        '<span class="status-pill-mini">' + (node.status || '') + '</span>';

      const idRow = document.createElement('div');
      idRow.className = 'inspector-id-row';
      const idSpan = document.createElement('span');
      idSpan.className = 'inspector-id';
      idSpan.textContent = node.id;
      const copyBtn = document.createElement('button');
      copyBtn.className = 'btn btn-sm';
      copyBtn.textContent = 'Copy';
      copyBtn.addEventListener('click', () => navigator.clipboard.writeText(node.id));
      idRow.appendChild(idSpan);
      idRow.appendChild(copyBtn);

      header.appendChild(topRow);
      header.appendChild(idRow);

      if (node.title) {
        const titleDiv = document.createElement('div');
        titleDiv.className = 'inspector-title';
        titleDiv.textContent = node.title;
        header.appendChild(titleDiv);
      }

      const actions = document.createElement('div');
      actions.className = 'inspector-actions';
      const focusBtn = document.createElement('button');
      focusBtn.className = 'btn btn-sm btn-accent';
      focusBtn.textContent = '🎯 Focus Subgraph';
      focusBtn.addEventListener('click', () => {
        focusNode(node.id);
        switchMainView('dag');
      });
      actions.appendChild(focusBtn);

      if (focusedNodeId) {
        const clearBtn = document.createElement('button');
        clearBtn.className = 'btn btn-sm';
        clearBtn.textContent = '✕ Show All';
        clearBtn.addEventListener('click', () => clearFocus());
        actions.appendChild(clearBtn);
      }
      header.appendChild(actions);
      inspector.appendChild(header);

      if (node.references && Object.keys(node.references).length > 0) {
        for (const [rel, targets] of Object.entries(node.references)) {
          if (!Array.isArray(targets) || targets.length === 0) continue;
          const sectionTitle = document.createElement('div');
          sectionTitle.className = 'section-title';
          sectionTitle.textContent = rel.replace('_', ' ');
          inspector.appendChild(sectionTitle);

          const chipsDiv = document.createElement('div');
          targets.forEach(targetId => {
            const chip = document.createElement('span');
            chip.className = 'ref-chip';
            chip.textContent = '↗ ' + targetId;
            chip.addEventListener('click', () => {
              selectNode(targetId, true);
              focusNode(targetId);
            });
            chipsDiv.appendChild(chip);
          });
          inspector.appendChild(chipsDiv);
        }
      }

      const attrTitle = document.createElement('div');
      attrTitle.className = 'section-title';
      attrTitle.textContent = 'Attributes';
      inspector.appendChild(attrTitle);

      const codeBox = document.createElement('div');
      codeBox.className = 'code-box';
      codeBox.textContent = JSON.stringify(node.attributes || node, null, 2);
      inspector.appendChild(codeBox);
    }

    // Initial Load
    loadDAG();
  </script>
</body>
</html>
`
