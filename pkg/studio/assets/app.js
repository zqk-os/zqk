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
    let ganttZoom = '1m'; // '2w', '1m', '3m', 'all'

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
      document.getElementById('tab-btn-inbox').classList.toggle('active', tab === 'inbox');
      document.getElementById('tab-inspector').classList.toggle('active', tab === 'inspector');
      document.getElementById('tab-objects').classList.toggle('active', tab === 'objects');
      document.getElementById('tab-inbox').classList.toggle('active', tab === 'inbox');
      if (tab === 'inbox') {
        fetchInbox();
      }
    }

    function escapeHtml(s) {
      if (!s) return '';
      return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    function getKindColor(kind) {
      switch(kind) {
        case 'mission': case 'vision': return 'var(--purple)';
        case 'workstream': return 'var(--orange)';
        case 'goal': case 'roadmap': return 'var(--success)';
        case 'priority_plan': return 'var(--accent)';
        case 'milestone': return 'var(--warning)';
        case 'backlog_item': return 'var(--teal)';
        case 'agent_task': return '#58a6ff';
        case 'requirement': return '#d2a8ff';
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
        case 'priority_plan': return 3;
        case 'milestone': return 4;
        case 'backlog_item': return 5;
        case 'agent_task': return 6;
        case 'requirement': return 7;
        case 'criteria': case 'test_case': return 8;
        default: return 5;
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

      // Build adjacency maps strictly for structural lineage
      const outgoing = new Map(); // child -> parent
      const incoming = new Map(); // parent -> child

      graphData.edges.forEach(e => {
        // Exclude loose metadata references (policies, personas, skills) from DAG causal focus
        if (e.structural === false) return;

        if (!outgoing.has(e.source)) outgoing.set(e.source, new Set());
        outgoing.get(e.source).add(e.target);

        if (!incoming.has(e.target)) incoming.set(e.target, new Set());
        incoming.get(e.target).add(e.source);
      });

      // BFS upstream (ancestors / parents)
      let queue = [centerId];
      while (queue.length > 0) {
        const cur = queue.shift();
        const parents = outgoing.get(cur);
        if (parents) {
          parents.forEach(p => {
            if (!activeIds.has(p)) {
              activeIds.add(p);
              queue.push(p);
            }
          });
        }
      }

      // BFS downstream (descendants / children)
      queue = [centerId];
      while (queue.length > 0) {
        const cur = queue.shift();
        const children = incoming.get(cur);
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
        const res = await fetch('/api/graph?_t=' + Date.now(), { cache: 'no-store' });
        const data = await res.json();
        graphData = data || { nodes: [], edges: [] };
        if (!graphData.nodes) graphData.nodes = [];
        if (!graphData.edges) graphData.edges = [];

        // Bidirectional workstream inheritance
        const wsMap = new Map();
        const nodeById = new Map();
        graphData.nodes.forEach(n => {
          nodeById.set(n.id, n);
          if (!wsMap.has(n.id)) wsMap.set(n.id, new Set());
          if (n.kind === 'workstream') {
            wsMap.get(n.id).add(n.id);
          }
          if (n.workstreamRefs) {
            n.workstreamRefs.forEach(w => { if (w) wsMap.get(n.id).add(w); });
          }
          if (n.references) {
            ['workstream_ref', 'workstream_refs', 'from_workstream_ref', 'to_workstream_ref'].forEach(k => {
              if (n.references[k]) {
                n.references[k].forEach(w => { if (w) wsMap.get(n.id).add(w); });
              }
            });
          }
        });

        // Propagate across structural edges:
        // In edge (source -> target), source is child and target is parent.
        let changed = true;
        let iters = 0;
        while (changed && iters < 15) {
          changed = false;
          iters++;
          graphData.edges.forEach(e => {
            if (e.structural === false) return;
            const childSet = wsMap.get(e.source);
            const parentSet = wsMap.get(e.target);
            if (!childSet || !parentSet) return;

            // Downstream: Parent passes workstreams to Child
            parentSet.forEach(wsId => {
              if (!childSet.has(wsId)) {
                childSet.add(wsId);
                changed = true;
              }
            });

            // Upstream: Child passes workstreams to Parent (unless parent is another workstream)
            const targetNode = nodeById.get(e.target);
            if (targetNode && targetNode.kind !== 'workstream') {
              childSet.forEach(wsId => {
                if (!parentSet.has(wsId)) {
                  parentSet.add(wsId);
                  changed = true;
                }
              });
            }
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
          const targetWs = activeWorkstreamFilter.toLowerCase();
          const isWS = n.id.toLowerCase() === targetWs;
          const refsWS = n.workstreamRefs && n.workstreamRefs.some(w => {
            const wl = (w || '').toLowerCase();
            return wl === targetWs || targetWs.includes(wl) || wl.includes(targetWs);
          });
          if (!isWS && !refsWS) return false;
        }

        // Density Filter
        if (!focusedNodeId) {
          const tier = getKindTier(n.kind);
          if (activeDensity === 'backbone' && tier > 4) return false;
          if (activeDensity === 'execution' && tier > 6) return false;
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
      const tiers = [[], [], [], [], [], [], [], [], []];
      const NODE_WIDTH = 220;
      const NODE_HEIGHT = 68;
      const COL_GAP = 90;
      const ROW_GAP = 28;

      filteredNodes.forEach(node => {
        const t = Math.min(getKindTier(node.kind), 8);
        tiers[t].push(node);
      });

      // Map child -> primary parent for vertical grouping alignment
      const parentMap = new Map();
      graphData.edges.forEach(e => {
        if (e.structural === false) return;
        if (!parentMap.has(e.source)) parentMap.set(e.source, e.target);
      });

      let colIndex = 0;
      const nodeOrderInPrevTier = new Map();

      tiers.forEach((tierNodes) => {
        if (tierNodes.length === 0) return;

        // Sort nodes in this tier by their parent's order in previous tier to minimize edge crossings
        tierNodes.sort((a, b) => {
          const parentA = parentMap.get(a.id);
          const parentB = parentMap.get(b.id);
          const orderA = parentA && nodeOrderInPrevTier.has(parentA) ? nodeOrderInPrevTier.get(parentA) : 9999;
          const orderB = parentB && nodeOrderInPrevTier.has(parentB) ? nodeOrderInPrevTier.get(parentB) : 9999;
          if (orderA !== orderB) return orderA - orderB;
          return a.id.localeCompare(b.id);
        });

        // Record positions for this tier
        tierNodes.forEach((node, rowIndex) => {
          nodeOrderInPrevTier.set(node.id, rowIndex);
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
        // Exclude loose metadata (policies, personas, skills) from DAG unless full mesh is chosen
        if (activeDensity !== 'all' && edge.structural === false) return;
        if (!activeSet.has(edge.source) || !activeSet.has(edge.target)) return;
        const p1 = nodePositions.get(edge.source);
        const p2 = nodePositions.get(edge.target);
        if (!p1 || !p2) return;

        const edgeKey = edge.source + '->' + edge.target;
        if (renderedEdges.has(edgeKey)) return;
        renderedEdges.add(edgeKey);

        // Always connect from earlier column (left) to later column (right)
        let leftPos = p1;
        let rightPos = p2;
        if (p1.x < p2.x) {
          leftPos = p1;
          rightPos = p2;
        } else if (p2.x < p1.x) {
          leftPos = p2;
          rightPos = p1;
        }

        const x1 = leftPos.x + leftPos.width;
        const y1 = leftPos.y + leftPos.height / 2;
        const x2 = rightPos.x;
        const y2 = rightPos.y + rightPos.height / 2;
        const dx = Math.max(30, Math.abs(x2 - x1) / 2);

        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', 'M ' + x1 + ' ' + y1 + ' C ' + (x1 + dx) + ' ' + y1 + ', ' + (x2 - dx) + ' ' + y2 + ', ' + x2 + ' ' + y2);
        const isHighlight = selectedNodeId && (edge.source === selectedNodeId || edge.target === selectedNodeId);
        path.setAttribute('class', 'edge-line' + (isHighlight ? ' highlight' : ''));
        path.setAttribute('marker-end', isHighlight ? 'url(#arrow-highlight)' : 'url(#arrow)');

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
          selectNode(node.id, false);
        });

        g.addEventListener('dblclick', (e) => {
          e.stopPropagation();
          focusNode(node.id);
        });

        nodesLayer.appendChild(g);
      });
    }

    let ganttStatusFilter = 'all';

    function setGanttStatusFilter(filter) {
      ganttStatusFilter = filter;
      document.querySelectorAll('.gantt-filter-pill').forEach(btn => {
        if (btn.id.startsWith('gantt-pill-')) {
          btn.classList.toggle('active', btn.id === 'gantt-pill-' + filter);
        }
      });
      renderGantt();
    }

    function setGanttZoom(zoom) {
      ganttZoom = zoom;
      document.querySelectorAll('.gantt-filter-pill').forEach(btn => {
        if (btn.id.startsWith('gantt-zoom-')) {
          btn.classList.toggle('active', btn.id === 'gantt-zoom-' + zoom);
        }
      });
      renderGantt();
    }

    // Chronological Timeline & Gantt Renderer
    function renderGantt() {
      const body = document.getElementById('gantt-body');
      const ticksContainer = document.getElementById('gantt-timeline-ticks');
      if (!body || !ticksContainer) return;
      body.innerHTML = '';
      ticksContainer.innerHTML = '';

      // Collect execution items: workstreams, priority plans, milestones, backlog items, tasks
      const validKinds = new Set(['workstream', 'milestone', 'priority_plan', 'backlog_item', 'agent_task']);
      let items = filteredNodes.filter(n => validKinds.has(n.kind));

      // Apply Gantt status filter
      if (ganttStatusFilter === 'active') {
        items = items.filter(n => ['in_progress', 'active', 'testing', 'metrics_captured', 'executing', 'started'].includes((n.status || '').toLowerCase()));
      } else if (ganttStatusFilter === 'planned') {
        items = items.filter(n => ['planned', 'originated', 'draft', 'pending', 'queued', 'shovel_ready', 'ready'].includes((n.status || '').toLowerCase()));
      } else if (ganttStatusFilter === 'done') {
        items = items.filter(n => ['complete', 'completed', 'implemented', 'verified', 'approved', 'done', 'sealed', 'archived', 'resolved', 'satisfied', 'passed', 'closed'].includes((n.status || '').toLowerCase()));
      }

      document.getElementById('gantt-task-count').textContent = items.length + ' items';

      if (items.length === 0) {
        body.innerHTML = '<div style="padding: 40px; color: var(--text-muted); text-align: center;">No execution items match current filters. Select "All" or adjust density.</div>';
        return;
      }

      // 1. Establish chronological schedule for every item
      const now = Date.now();
      const MS_PER_DAY = 86400000;

      const parsedItems = items.map((item, idx) => {
        let start = null;
        let end = null;

        // 1. Check explicit date fields from object attributes
        if (item.startDate) {
          const d = Date.parse(item.startDate);
          if (!isNaN(d)) start = d;
        }
        if (item.targetDate) {
          const d = Date.parse(item.targetDate);
          if (!isNaN(d)) end = d;
        } else if (item.dueDate) {
          const d = Date.parse(item.dueDate);
          if (!isNaN(d)) end = d;
        }

        const status = (item.status || 'planned').toLowerCase();
        const isDone = ['complete', 'completed', 'implemented', 'verified', 'approved', 'done', 'sealed', 'archived', 'resolved', 'satisfied', 'passed', 'closed'].includes(status);
        const isActive = ['in_progress', 'active', 'testing', 'metrics_captured', 'executing', 'started'].includes(status);

        // Duration heuristics
        const defaultDurationDays = item.kind === 'workstream' ? 45 :
                                    item.kind === 'milestone' ? 5 :
                                    item.kind === 'priority_plan' ? 21 :
                                    item.kind === 'agent_task' ? 2 : 5;
        const durationMs = defaultDurationDays * MS_PER_DAY;

        if (isDone) {
          // Completed / Implemented items belong strictly in the past leading up to completion date
          if (!end && item.updatedAt) {
            const d = Date.parse(item.updatedAt);
            if (!isNaN(d)) end = Math.min(now, d);
          }
          if (!end && item.createdAt) {
            const d = Date.parse(item.createdAt);
            if (!isNaN(d)) end = Math.min(now, d);
          }
          if (!end) end = now - (0.5 * MS_PER_DAY);

          if (!start && item.createdAt) {
            const d = Date.parse(item.createdAt);
            if (!isNaN(d) && d < end) start = d;
          }
          if (!start) start = end - durationMs;
        } else if (isActive) {
          // In-Progress / Active items: active NOW, spanning across TODAY
          if (item.kind === 'milestone') {
            if (!end) end = now + (5 * MS_PER_DAY);
            if (!start) start = now - (2 * MS_PER_DAY);
          } else if (item.kind === 'workstream') {
            if (!start) start = now - (14 * MS_PER_DAY);
            if (!end) end = start + durationMs;
            if (end <= now) end = now + (15 * MS_PER_DAY);
          } else {
            // Active task / backlog item / plan
            if (!start) {
              if (item.createdAt) {
                const d = Date.parse(item.createdAt);
                if (!isNaN(d) && d <= now) start = Math.max(now - (3 * MS_PER_DAY), d);
              }
              if (!start) start = now - (2 * MS_PER_DAY);
            }
            if (!end) end = now + Math.max(2 * MS_PER_DAY, durationMs - (now - start));
          }
        } else {
          // Planned / upcoming items: scheduled from TODAY onwards into the future
          const planOffsetDays = (idx % 6) * 2 + 1;
          if (!start || start < now) {
            start = now + (planOffsetDays * MS_PER_DAY);
          }
          if (!end) end = start + durationMs;
        }

        // Safety guarantee: end must be after start
        if (end <= start) {
          end = start + Math.max(MS_PER_DAY, durationMs);
        }

        return { ...item, startTime: start, endTime: end };
      });

      // 2. Compute timeline window based on selected zoom
      let minTime, maxTime;
      if (ganttZoom === '2w') {
        minTime = now - (5 * MS_PER_DAY);
        maxTime = now + (9 * MS_PER_DAY);
      } else if (ganttZoom === '1m') {
        minTime = now - (10 * MS_PER_DAY);
        maxTime = now + (20 * MS_PER_DAY);
      } else if (ganttZoom === '3m') {
        minTime = now - (20 * MS_PER_DAY);
        maxTime = now + (70 * MS_PER_DAY);
      } else {
        // 'all': fit full range
        minTime = Infinity;
        maxTime = -Infinity;
        parsedItems.forEach(i => {
          if (i.startTime < minTime) minTime = i.startTime;
          if (i.endTime > maxTime) maxTime = i.endTime;
        });
        if (now < minTime) minTime = now - (3 * MS_PER_DAY);
        if (now > maxTime) maxTime = now + (7 * MS_PER_DAY);
        minTime -= 2 * MS_PER_DAY;
        maxTime += 6 * MS_PER_DAY;
      }
      const totalDuration = Math.max(MS_PER_DAY * 7, maxTime - minTime);

      // 3. Render Calendar Date Ticks across header
      let tickStepDays = 1;
      if (ganttZoom === '2w') {
        tickStepDays = 1;
      } else if (ganttZoom === '1m') {
        tickStepDays = 2;
      } else if (ganttZoom === '3m') {
        tickStepDays = 7;
      } else {
        tickStepDays = totalDuration > (90 * MS_PER_DAY) ? 14 : (totalDuration > (30 * MS_PER_DAY) ? 7 : 3);
      }
      const tickStepMs = tickStepDays * MS_PER_DAY;

      const ticks = [];
      const firstTickTime = Math.ceil(minTime / tickStepMs) * tickStepMs;
      for (let t = firstTickTime; t <= maxTime; t += tickStepMs) {
        const leftPercent = ((t - minTime) / totalDuration) * 100;
        if (leftPercent >= 0 && leftPercent <= 98) {
          ticks.push({ time: t, left: leftPercent });
        }
      }

      ticksContainer.innerHTML = ticks.map(tick => {
        const d = new Date(tick.time);
        const month = d.toLocaleString('en-US', { month: 'short' });
        const day = d.getDate();
        const weekday = d.toLocaleString('en-US', { weekday: 'short' });
        return '<div class="gantt-tick" style="left: ' + tick.left + '%;">' +
          '<span>' + month + ' ' + day + '</span>' +
          '<span class="gantt-tick-sub">' + weekday + '</span>' +
        '</div>';
      }).join('');

      // Add Today vertical marker badge in header
      const todayPercent = ((now - minTime) / totalDuration) * 100;
      if (todayPercent >= 0 && todayPercent <= 100) {
        const todayHeaderBadge = document.createElement('div');
        todayHeaderBadge.className = 'gantt-today-badge';
        todayHeaderBadge.style.left = todayPercent + '%';
        todayHeaderBadge.textContent = 'TODAY';
        ticksContainer.appendChild(todayHeaderBadge);
      }

      // 4. Grouping
      const groupingSelect = document.getElementById('gantt-grouping');
      const groupingMode = groupingSelect ? groupingSelect.value : 'workstream';

      const groups = new Map();
      if (groupingMode === 'flat') {
        groups.set('All Tasks (Chronological)', parsedItems.sort((a, b) => a.startTime - b.startTime));
      } else if (groupingMode === 'plan') {
        parsedItems.forEach(item => {
          let p = (item.references && item.references.priority_plan_ref && item.references.priority_plan_ref[0]) ||
                  (item.kind === 'priority_plan' ? item.id : 'Other Work Items');
          if (!groups.has(p)) groups.set(p, []);
          groups.get(p).push(item);
        });
      } else {
        // By Workstream
        const unassigned = [];
        parsedItems.forEach(item => {
          let ws = (item.workstreamRefs && item.workstreamRefs[0]) || (item.kind === 'workstream' ? item.id : null);
          if (ws) {
            if (!groups.has(ws)) groups.set(ws, []);
            groups.get(ws).push(item);
          } else {
            unassigned.push(item);
          }
        });
        if (unassigned.length > 0) groups.set('General Tasks & Milestones', unassigned);
      }

      // 5. Render Rows with Today Line and Grid
      if (todayPercent >= 0 && todayPercent <= 100) {
        const todayLine = document.createElement('div');
        todayLine.className = 'gantt-today-line';
        todayLine.style.left = 'calc(380px + (100% - 380px) * ' + (todayPercent / 100) + ')';
        body.appendChild(todayLine);
      }

      groups.forEach((groupItems, groupName) => {
        const header = document.createElement('div');
        header.className = 'gantt-section-header';
        header.innerHTML = '<span>⚡ ' + escapeHtml(groupName) + '</span> <span style="font-size: 11px; font-weight: normal; color: var(--text-muted);">' + groupItems.length + ' items</span>';
        body.appendChild(header);

        // Sort items hierarchically: Tier first (Plans -> Milestones -> BLIs -> Tasks), then startTime
        groupItems.sort((a, b) => {
          const tierA = getKindTier(a.kind);
          const tierB = getKindTier(b.kind);
          if (tierA !== tierB) return tierA - tierB;
          return a.startTime - b.startTime;
        });

        groupItems.forEach(item => {
          const row = document.createElement('div');
          row.className = 'gantt-row' + (item.id === selectedNodeId ? ' selected' : '');

          const kindClass = 'kind-' + item.kind;
          const status = (item.status || 'planned').toLowerCase();
          const tier = getKindTier(item.kind);

          let indentPx = 0;
          if (tier === 3) indentPx = 0; // Priority plan
          else if (tier === 4) indentPx = 12; // Milestone
          else if (tier === 5) indentPx = 24; // BLI
          else if (tier === 6) indentPx = 36; // Agent Task
          else if (tier > 6) indentPx = 48;

          // Left Info Cell with hierarchy indentation
          const info = document.createElement('div');
          info.className = 'gantt-row-info';
          info.style.paddingLeft = (16 + indentPx) + 'px';
          info.innerHTML =
            '<div class="gantt-row-title-line">' +
              (indentPx > 0 ? '<span class="gantt-tree-elbow">↳</span>' : '') +
              '<span class="kind-pill ' + kindClass + '">' + item.kind.replace('_', ' ') + '</span>' +
              '<span class="gantt-row-id">' + item.id + '</span>' +
              '<span class="status-pill-mini status-' + status + '">' + (item.status || '') + '</span>' +
            '</div>' +
            '<div class="gantt-row-title" title="' + escapeHtml(item.title || '') + '">' + escapeHtml(item.title || '') + '</div>';

          // Right Bar Cell
          const cell = document.createElement('div');
          cell.className = 'gantt-bar-cell';

          // Grid lines inside row
          ticks.forEach(t => {
            const gridCol = document.createElement('div');
            gridCol.className = 'gantt-grid-col';
            gridCol.style.left = t.left + '%';
            cell.appendChild(gridCol);
          });

          // Horizontal placement relative to timeline window
          const leftPercent = Math.max(0, Math.min(98, ((item.startTime - minTime) / totalDuration) * 100));
          const rawRight = ((item.endTime - minTime) / totalDuration) * 100;
          const rightPercent = Math.max(leftPercent + 1.8, Math.min(100, rawRight));
          const widthPercent = Math.max(1.8, rightPercent - leftPercent);

          const startDateStr = new Date(item.startTime).toISOString().slice(0, 10);
          const endDateStr = new Date(item.endTime).toISOString().slice(0, 10);
          const durationDays = Math.max(1, Math.round((item.endTime - item.startTime) / MS_PER_DAY));

          const isDone = ['complete', 'completed', 'implemented', 'verified', 'approved', 'done', 'sealed', 'archived', 'resolved', 'satisfied', 'passed', 'closed'].includes(status);
          const isActive = ['in_progress', 'active', 'testing', 'metrics_captured', 'executing', 'started'].includes(status);

          let barClass = 'gantt-bar-planned';
          let statusIcon = '⏳';
          if (item.kind === 'milestone') {
            barClass = 'gantt-bar-milestone';
            statusIcon = '◆';
          } else if (item.kind === 'workstream') {
            barClass = 'gantt-bar-workstream';
            statusIcon = '🌐';
          } else if (isDone) {
            barClass = 'gantt-bar-complete';
            statusIcon = '✓';
          } else if (isActive) {
            barClass = 'gantt-bar-inprogress';
            statusIcon = '▶';
          } else if (status === 'testing' || status === 'metrics_captured') {
            barClass = 'gantt-bar-testing';
            statusIcon = '⚙';
          }

          const bar = document.createElement('div');
          bar.className = 'gantt-bar ' + barClass;
          bar.style.left = leftPercent + '%';
          bar.style.width = widthPercent + '%';
          const effortStr = item.estimatedEffort ? '\nEffort: ' + item.estimatedEffort : '';
          bar.setAttribute('title', item.id + ': ' + (item.title || '') + '\nKind: ' + item.kind.replace('_', ' ') + ' | Status: ' + status + '\nSchedule: ' + startDateStr + ' → ' + endDateStr + ' (' + durationDays + ' days)' + effortStr);

          bar.innerHTML = '<span style="margin-right: 4px;">' + statusIcon + '</span><span style="font-weight: 700;">' + item.id + '</span>' +
            (widthPercent > 12 && item.title ? ': <span style="opacity: 0.9; font-weight: normal; margin-left: 3px;">' + escapeHtml(item.title) + '</span>' : '');

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
        const res = await fetch('/api/objects/' + encodeURIComponent(nodeId) + '?_t=' + Date.now(), { cache: 'no-store' });
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

    async function fetchInbox() {
      const container = document.getElementById('inbox-list-container');
      try {
        const res = await fetch('/api/inbox?_t=' + Date.now(), { cache: 'no-store' });
        if (!res.ok) throw new Error('Failed to load inbox');
        const data = await res.json();
        const unacked = data.inbox_unacked || [];
        const envelopes = data.staged_envelopes || [];
        const count = unacked.length + envelopes.length;
        document.getElementById('inbox-count').textContent = count;

        if (count === 0) {
          container.innerHTML = '<div class="empty-state"><div class="empty-state-icon">📭</div><div class="empty-state-title">Inbox Empty</div><div style="font-size: 12px; margin-top: 4px;">No unacknowledged correspondence or staged envelopes.</div></div>';
          return;
        }

        let html = '';
        if (envelopes.length > 0) {
          html += '<div style="font-size: 11px; font-weight: 700; text-transform: uppercase; color: var(--warning); margin: 8px 4px;">⚡ Staged Envelopes (' + envelopes.length + ')</div>';
          envelopes.forEach(env => {
            html += '<div style="background: var(--card-bg); border: 1px solid var(--border); border-radius: 6px; padding: 10px; margin-bottom: 8px;">' +
              '<div style="display: flex; justify-content: space-between; align-items: center;">' +
                '<span style="font-weight: 600; color: var(--accent); font-size: 12px;">' + escapeHtml(env.id) + '</span>' +
                '<span class="status-pill-mini">' + escapeHtml(env.status) + '</span>' +
              '</div>' +
              '<div style="font-size: 12px; margin-top: 4px; color: var(--text);">' + escapeHtml(env.operation) + ' ' + escapeHtml(env.kind) + ' (' + escapeHtml(env.target_id) + ')</div>' +
              '<div style="margin-top: 8px; display: flex; gap: 6px;">' +
                '<button class="btn btn-sm" onclick="ackEnvelope(\'' + escapeHtml(env.id) + '\')">Commit / Ack</button>' +
              '</div>' +
            '</div>';
          });
        }

        if (unacked.length > 0) {
          html += '<div style="font-size: 11px; font-weight: 700; text-transform: uppercase; color: var(--text-muted); margin: 8px 4px;">📬 Unacknowledged Messages (' + unacked.length + ')</div>';
          unacked.forEach(item => {
            const sender = item.from_agent_id || item.sender || 'agent';
            html += '<div style="background: var(--card-bg); border: 1px solid var(--border); border-radius: 6px; padding: 10px; margin-bottom: 8px;">' +
              '<div style="display: flex; justify-content: space-between; align-items: center;">' +
                '<span style="font-weight: 600; color: var(--teal); font-size: 12px;">From: ' + escapeHtml(sender) + '</span>' +
                '<span style="font-size: 10px; color: var(--text-muted);">' + escapeHtml(item.timestamp || '') + '</span>' +
              '</div>' +
              '<div style="font-size: 12px; margin-top: 6px; color: var(--text-bright); line-height: 1.4;">' + escapeHtml(item.message || item.summary || '') + '</div>' +
              '<div style="margin-top: 8px; display: flex; gap: 6px;">' +
                '<button class="btn btn-sm" onclick="ackInboxItem(\'' + escapeHtml(item.event_id) + '\')">✓ Ack</button>' +
                '<button class="btn btn-sm" onclick="promptRespond(\'' + escapeHtml(sender) + '\', \'' + escapeHtml(item.event_id) + '\')">💬 Reply</button>' +
              '</div>' +
            '</div>';
          });
        }
        container.innerHTML = html;
      } catch (err) {
        container.innerHTML = '<div style="padding: 16px; color: var(--text-muted);">Failed to load inbox: ' + escapeHtml(err.message) + '</div>';
      }
    }

    async function ackEnvelope(envId) {
      try {
        const res = await fetch('/api/inbox/ack', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ envelope_id: envId })
        });
        if (!res.ok) throw new Error('Ack failed');
        await fetchInbox();
      } catch (err) {
        alert('Failed to ack envelope: ' + err.message);
      }
    }

    async function ackInboxItem(eventId) {
      try {
        const res = await fetch('/api/inbox/ack', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ in_reply_to: eventId })
        });
        if (!res.ok) throw new Error('Ack failed');
        await fetchInbox();
      } catch (err) {
        alert('Failed to ack inbox item: ' + err.message);
      }
    }

    async function promptRespond(toAgent, replyTo) {
      const msg = prompt('Enter response to agent ' + toAgent + ':');
      if (!msg || !msg.trim()) return;
      try {
        const res = await fetch('/api/inbox/respond', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ to_agent_id: toAgent, message: msg.trim(), in_reply_to: replyTo })
        });
        if (!res.ok) throw new Error('Respond failed');
        await fetchInbox();
      } catch (err) {
        alert('Failed to send response: ' + err.message);
      }
    }

    // Initial Load
    loadDAG();
