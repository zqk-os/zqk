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
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Title  string `json:"title"`
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
		"mission", "vision", "goal", "roadmap", "milestone",
		"priority_plan", "backlog_item", "requirement", "criteria", "test_case",
		"decision", "workstream", "agent_task",
	}

	nodes := make([]GraphNode, 0)
	edges := make([]GraphEdge, 0)
	seenEdges := make(map[string]bool)

	for _, kind := range kinds {
		for _, node := range s.indexer.GetNodesByKind(kind) {
			nodes = append(nodes, GraphNode{
				ID:     node.ID,
				Kind:   node.Kind,
				Status: node.Status,
				Title:  node.Title,
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
			"mission", "vision", "goal", "roadmap", "milestone",
			"priority_plan", "backlog_item", "requirement", "criteria", "test_case",
			"decision", "workstream", "agent_task",
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
  <title>ZQK Knowledge Kernel Studio</title>
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
      padding: 12px 20px;
      background: var(--card-bg);
      border-bottom: 1px solid var(--border);
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 16px;
      flex-shrink: 0;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .brand-icon {
      font-size: 18px;
    }
    h1 {
      margin: 0;
      font-size: 16px;
      font-weight: 600;
      color: var(--text-bright);
      letter-spacing: -0.2px;
    }
    .header-controls {
      display: flex;
      align-items: center;
      gap: 12px;
    }
    .search-box {
      position: relative;
    }
    .search-input {
      background: #0d1117;
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--text-bright);
      padding: 6px 12px 6px 30px;
      font-size: 13px;
      width: 240px;
      outline: none;
      transition: border-color 0.15s, width 0.2s;
    }
    .search-input:focus {
      border-color: var(--accent);
      width: 300px;
    }
    .search-icon {
      position: absolute;
      left: 10px;
      top: 50%;
      transform: translateY(-50%);
      color: var(--text-muted);
      font-size: 12px;
      pointer-events: none;
    }
    .stats-badge {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 12px;
      color: var(--text-muted);
      background: #0d1117;
      border: 1px solid var(--border);
      padding: 5px 10px;
      border-radius: 6px;
    }
    .status-pill {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      background: rgba(63, 185, 80, 0.12);
      color: var(--success);
      padding: 4px 10px;
      border-radius: 12px;
      font-size: 12px;
      font-weight: 500;
    }
    .pulse-dot {
      width: 7px;
      height: 7px;
      background: var(--success);
      border-radius: 50%;
      box-shadow: 0 0 8px var(--success);
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
    .btn:hover {
      background: #30363d;
      border-color: var(--border-bright);
    }
    main {
      flex: 1;
      display: grid;
      grid-template-columns: 1fr 380px;
      gap: 0;
      overflow: hidden;
      position: relative;
    }
    .panel {
      background: var(--bg);
      display: flex;
      flex-direction: column;
      overflow: hidden;
      position: relative;
    }
    .panel-graph {
      border-right: 1px solid var(--border);
      background: radial-gradient(circle at 50% 50%, #161b22 0%, #0d1117 100%);
    }
    .graph-toolbar {
      position: absolute;
      top: 14px;
      left: 14px;
      z-index: 10;
      display: flex;
      gap: 6px;
      background: rgba(22, 27, 34, 0.9);
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
      background: rgba(22, 27, 34, 0.9);
      backdrop-filter: blur(8px);
      padding: 4px;
      border-radius: 8px;
      border: 1px solid var(--border);
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
    .graph-canvas {
      flex: 1;
      width: 100%;
      height: 100%;
      cursor: grab;
    }
    .graph-canvas.grabbing {
      cursor: grabbing;
    }
    /* SVG graph styles */
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
    .node-group {
      cursor: pointer;
      transition: transform 0.15s;
    }
    .node-bg {
      fill: #161b22;
      stroke: #30363d;
      stroke-width: 1.5;
      rx: 8;
      ry: 8;
      transition: stroke 0.2s, fill 0.2s, filter 0.2s;
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
    .node-group.dimmed {
      opacity: 0.25;
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
    /* Right Side Panel */
    .sidebar-panel {
      background: var(--card-bg);
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
    .tab-content.active {
      display: block;
    }
    /* Object Card List */
    .node-card {
      padding: 10px 12px;
      margin-bottom: 8px;
      background: #1c2128;
      border: 1px solid var(--border);
      border-radius: 6px;
      cursor: pointer;
      transition: background 0.15s, border-color 0.15s, transform 0.1s;
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
    .kind-goal, .kind-roadmap { background: rgba(63, 185, 80, 0.2); color: var(--success); }
    .kind-priority_plan, .kind-milestone { background: rgba(88, 166, 255, 0.2); color: var(--accent); }
    .kind-backlog_item, .kind-workstream { background: rgba(57, 197, 187, 0.2); color: var(--teal); }
    .kind-requirement { background: rgba(210, 153, 34, 0.2); color: var(--warning); }
    .kind-criteria { background: rgba(63, 185, 80, 0.2); color: var(--success); }
    .kind-test_case { background: rgba(219, 97, 162, 0.2); color: var(--pink); }
    .kind-default { background: #30363d; color: var(--text-muted); }
    .status-pill-mini {
      font-size: 10px;
      color: var(--text-muted);
    }
    .node-card-id {
      font-size: 12px;
      font-weight: 600;
      color: var(--text-bright);
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    .node-card-title {
      font-size: 12px;
      color: var(--text-muted);
      margin-top: 4px;
      line-height: 1.4;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    /* Inspector View */
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
      font-size: 14px;
      font-weight: 700;
      color: var(--text-bright);
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      word-break: break-all;
    }
    .inspector-title {
      font-size: 14px;
      font-weight: 500;
      color: var(--text);
      margin-top: 6px;
      line-height: 1.4;
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
    .empty-state-icon {
      font-size: 32px;
      margin-bottom: 8px;
    }
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
    <div class="header-controls">
      <div class="search-box">
        <span class="search-icon">🔍</span>
        <input type="text" id="global-search" class="search-input" placeholder="Search DAG (press /) ...">
      </div>
      <div class="stats-badge">
        <span id="stat-nodes">0 Nodes</span>
        <span>•</span>
        <span id="stat-edges">0 Edges</span>
      </div>
      <div class="status-pill" id="live-indicator">
        <span class="pulse-dot"></span>
        <span>Connected</span>
      </div>
      <button class="btn" id="btn-refresh" onclick="loadDAG()">⟳ Refresh</button>
    </div>
  </header>

  <main>
    <!-- Left: Interactive Graph Canvas -->
    <div class="panel panel-graph">
      <div class="panel-header" style="display:none;" id="dag-panel-title">Interactive Knowledge Graph DAG</div>
      <div class="graph-toolbar">
        <button class="btn" onclick="zoomIn()" title="Zoom In">+</button>
        <button class="btn" onclick="zoomOut()" title="Zoom Out">-</button>
        <button class="btn" onclick="resetZoom()" title="Reset Zoom">⟲</button>
        <button class="btn" onclick="fitGraph()" title="Fit to Screen">⛶</button>
      </div>
      <div class="filter-bar" id="kind-filters">
        <span class="filter-chip active" data-kind="all" onclick="filterKind('all')">All</span>
        <span class="filter-chip" data-kind="goal" onclick="filterKind('goal')">Goals</span>
        <span class="filter-chip" data-kind="priority_plan" onclick="filterKind('priority_plan')">Plans</span>
        <span class="filter-chip" data-kind="backlog_item" onclick="filterKind('backlog_item')">BLIs</span>
        <span class="filter-chip" data-kind="requirement" onclick="filterKind('requirement')">Reqs</span>
        <span class="filter-chip" data-kind="criteria" onclick="filterKind('criteria')">Crits</span>
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

    <!-- Right: Multi-Tab Sidebar -->
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
            <div style="font-size: 12px; margin-top: 4px;">Click any node in the DAG or select an object from the list to view provenance lineage.</div>
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
    let activeKindFilter = 'all';
    let searchQuery = '';

    // Transform state
    let scale = 1.0;
    let translateX = 40;
    let translateY = 40;
    let isPanning = false;
    let panStartX = 0;
    let panStartY = 0;

    // Node layout positions: nodeId -> { x, y, width, height }
    let nodePositions = new Map();

    const svg = document.getElementById('dag-svg');
    const viewport = document.getElementById('viewport');
    const nodesLayer = document.getElementById('nodes-layer');
    const edgesLayer = document.getElementById('edges-layer');

    function updateTransform() {
      viewport.setAttribute('transform', 'translate(' + translateX + ',' + translateY + ') scale(' + scale + ')');
    }

    function zoomIn() { scale = Math.min(scale * 1.25, 3.0); updateTransform(); }
    function zoomOut() { scale = Math.max(scale / 1.25, 0.2); updateTransform(); }
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
      const contentW = maxX - minX + 80;
      const contentH = maxY - minY + 80;
      scale = Math.min(rect.width / contentW, rect.height / contentH, 1.2);
      translateX = (rect.width - contentW * scale) / 2 - minX * scale + 40 * scale;
      translateY = (rect.height - contentH * scale) / 2 - minY * scale + 40 * scale;
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
      const newScale = Math.min(Math.max(scale * zoomFactor, 0.15), 3.5);
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
        selectedNodeId = null;
        renderGraph();
        renderObjectsList();
      }
    });

    document.getElementById('global-search').addEventListener('input', (e) => {
      searchQuery = e.target.value.toLowerCase().trim();
      applyFilters();
    });

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
        case 'goal': case 'roadmap': return 'var(--success)';
        case 'priority_plan': case 'milestone': return 'var(--accent)';
        case 'backlog_item': case 'workstream': return 'var(--teal)';
        case 'requirement': return 'var(--warning)';
        case 'criteria': return 'var(--success)';
        case 'test_case': return 'var(--pink)';
        default: return 'var(--text-muted)';
      }
    }

    function getKindTier(kind) {
      switch(kind) {
        case 'mission': case 'vision': return 0;
        case 'goal': case 'roadmap': return 1;
        case 'milestone': case 'priority_plan': return 2;
        case 'backlog_item': case 'workstream': return 3;
        case 'requirement': return 4;
        case 'criteria': return 5;
        case 'test_case': return 6;
        default: return 3;
      }
    }

    async function loadDAG() {
      try {
        const res = await fetch('/api/graph');
        const data = await res.json();
        graphData = data || { nodes: [], edges: [] };
        if (!graphData.nodes) graphData.nodes = [];
        if (!graphData.edges) graphData.edges = [];

        document.getElementById('stat-nodes').textContent = graphData.nodes.length + ' Nodes';
        document.getElementById('stat-edges').textContent = graphData.edges.length + ' Edges';
        document.getElementById('objects-count').textContent = graphData.nodes.length;

        applyFilters();
        if (graphData.nodes.length > 0 && !selectedNodeId) {
          selectNode(graphData.nodes[0].id, false);
        }
      } catch (err) {
        console.error('Failed to load DAG', err);
      }
    }

    function applyFilters() {
      filteredNodes = graphData.nodes.filter(n => {
        const matchKind = activeKindFilter === 'all' || n.kind === activeKindFilter;
        const matchSearch = !searchQuery ||
          n.id.toLowerCase().includes(searchQuery) ||
          (n.title && n.title.toLowerCase().includes(searchQuery)) ||
          n.kind.toLowerCase().includes(searchQuery);
        return matchKind && matchSearch;
      });

      calculateLayout();
      renderGraph();
      renderObjectsList();
    }

    function calculateLayout() {
      nodePositions.clear();
      const tiers = [[], [], [], [], [], [], []];
      const NODE_WIDTH = 220;
      const NODE_HEIGHT = 68;
      const COL_GAP = 90;
      const ROW_GAP = 28;

      filteredNodes.forEach(node => {
        const t = Math.min(getKindTier(node.kind), 6);
        tiers[t].push(node);
      });

      // Filter out empty tiers for tighter column alignment
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
        nodesLayer.innerHTML = '<text x="100" y="100" fill="var(--text-muted)" font-size="14">No nodes match the active filter or search query.</text>';
        return;
      }

      // Draw Edges
      const renderedEdges = new Set();
      graphData.edges.forEach(edge => {
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
        path.setAttribute('class', 'edge-line');
        path.setAttribute('marker-end', 'url(#arrow)');
        path.setAttribute('data-source', edge.source);
        path.setAttribute('data-target', edge.target);

        if (selectedNodeId && (edge.source === selectedNodeId || edge.target === selectedNodeId)) {
          path.classList.add('highlight');
          path.setAttribute('marker-end', 'url(#arrow-highlight)');
        }

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
        rect.setAttribute('style', 'border-left: 4px solid ' + color);

        // Accent indicator bar on left
        const bar = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        bar.setAttribute('x', 0);
        bar.setAttribute('y', 0);
        bar.setAttribute('width', 4);
        bar.setAttribute('height', pos.height);
        bar.setAttribute('fill', color);
        bar.setAttribute('rx', 2);

        // Kind badge
        const kindText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        kindText.setAttribute('x', 14);
        kindText.setAttribute('y', 18);
        kindText.setAttribute('class', 'node-badge');
        kindText.setAttribute('fill', color);
        kindText.textContent = node.kind.replace('_', ' ');

        // Status pill text
        const statusText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        statusText.setAttribute('x', pos.width - 12);
        statusText.setAttribute('y', 18);
        statusText.setAttribute('text-anchor', 'end');
        statusText.setAttribute('class', 'node-badge');
        statusText.setAttribute('fill', 'var(--text-muted)');
        statusText.textContent = node.status || '';

        // ID Text
        const idText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        idText.setAttribute('x', 14);
        idText.setAttribute('y', 36);
        idText.setAttribute('class', 'node-text-id');
        idText.textContent = node.id.length > 24 ? node.id.slice(0, 22) + '…' : node.id;

        // Title Text
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
        });

        nodesLayer.appendChild(g);
      });
    }

    function renderObjectsList() {
      const container = document.getElementById('objects-list-container');
      if (filteredNodes.length === 0) {
        container.innerHTML = '<div style="padding: 24px; color: var(--text-muted); text-align: center;">No matching objects.</div>';
        return;
      }

      container.innerHTML = filteredNodes.map(n => {
        const isSel = n.id === selectedNodeId;
        const kindClass = 'kind-' + n.kind;
        return '<div class="node-card' + (isSel ? ' selected' : '') + '" onclick="selectNode(\'' + n.id + '\', true)">' +
          '<div class="node-card-top">' +
            '<span class="kind-pill ' + kindClass + '">' + n.kind.replace('_', ' ') + '</span>' +
            '<span class="status-pill-mini">' + (n.status || '') + '</span>' +
          '</div>' +
          '<div class="node-card-id">' + n.id + '</div>' +
          '<div class="node-card-title">' + (n.title || '') + '</div>' +
        '</div>';
      }).join('');
    }

    async function selectNode(nodeId, panTo) {
      selectedNodeId = nodeId;
      renderGraph();
      renderObjectsList();
      switchTab('inspector');

      const inspector = document.getElementById('inspector-content');
      inspector.innerHTML = '<div style="padding: 24px; color: var(--text-muted);">Fetching object details...</div>';

      try {
        const res = await fetch('/api/objects/' + encodeURIComponent(nodeId));
        if (!res.ok) throw new Error('Object not found');
        const node = await res.json();
        renderInspector(node);

        if (panTo) {
          const pos = nodePositions.get(nodeId);
          if (pos) {
            const rect = svg.getBoundingClientRect();
            translateX = rect.width / 2 - (pos.x + pos.width / 2) * scale;
            translateY = rect.height / 2 - (pos.y + pos.height / 2) * scale;
            updateTransform();
          }
        }
      } catch (err) {
        // Fallback to graph node info
        const gNode = graphData.nodes.find(n => n.id === nodeId);
        if (gNode) renderInspector(gNode);
      }
    }

    function renderInspector(node) {
      const inspector = document.getElementById('inspector-content');
      const kindClass = 'kind-' + (node.kind || 'default');

      let refsHTML = '';
      if (node.references && Object.keys(node.references).length > 0) {
        for (const [rel, targets] of Object.entries(node.references)) {
          refsHTML += '<div class="section-title">' + rel.replace('_', ' ') + '</div><div>';
          targets.forEach(targetId => {
            refsHTML += '<span class="ref-chip" onclick="selectNode(\'' + targetId + '\', true)">↗ ' + targetId + '</span>';
          });
          refsHTML += '</div>';
        }
      }

      inspector.innerHTML =
        '<div class="inspector-header">' +
          '<div style="display: flex; justify-content: space-between; align-items: center;">' +
            '<span class="kind-pill ' + kindClass + '">' + (node.kind || 'object').replace('_', ' ') + '</span>' +
            '<span class="status-pill-mini">' + (node.status || '') + '</span>' +
          '</div>' +
          '<div class="inspector-id-row">' +
            '<span class="inspector-id">' + node.id + '</span>' +
            '<button class="btn" onclick="navigator.clipboard.writeText(\'' + node.id + '\')">Copy</button>' +
          '</div>' +
          (node.title ? '<div class="inspector-title">' + node.title + '</div>' : '') +
        '</div>' +
        refsHTML +
        '<div class="section-title">Attributes</div>' +
        '<div class="code-box">' + JSON.stringify(node.attributes || node, null, 2) + '</div>';
    }

    // Initial Load
    loadDAG();
  </script>
</body>
</html>
`
