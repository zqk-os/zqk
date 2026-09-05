"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.MeshFeedPanel = void 0;
const fs = __importStar(require("fs"));
const path = __importStar(require("path"));
const vscode = __importStar(require("vscode"));
const SKIP_TYPES = new Set(["kernel_ack"]);
function seatColor(seat) {
    const s = seat.toLowerCase();
    if (s.includes("cursor") || s.includes("tpm")) {
        return "#6cb6ff";
    }
    if (s.includes("antigravity-1") || s.endsWith("-1")) {
        return "#7ee787";
    }
    if (s.includes("antigravity-2") || s.endsWith("-2")) {
        return "#d2a8ff";
    }
    if (s === "kernel" || s === "wake") {
        return "#8b949e";
    }
    return "#ffa657";
}
function formatWhen(ts) {
    if (!ts) {
        return "";
    }
    try {
        return new Date(ts).toLocaleTimeString();
    }
    catch {
        return ts;
    }
}
function isMeshCorrespondence(ev) {
    // Cursor postToolUse probe lines share the same JSONL — exclude them.
    if (ev.tool_name != null || ev.stdin_sha256 != null || ev.probe_mode != null) {
        return false;
    }
    const typ = typeof ev.event_type === "string" ? ev.event_type.trim() : "";
    const id = typeof ev.event_id === "string" ? ev.event_id.trim() : "";
    if (typ && id) {
        return true;
    }
    if (typeof ev.message === "string" && ev.message && typeof ev.agent_id === "string") {
        return true;
    }
    return false;
}
function normalizeFeedLine(raw) {
    return {
        event_id: typeof raw.event_id === "string" ? raw.event_id : undefined,
        event_type: typeof raw.event_type === "string" ? raw.event_type : undefined,
        agent_id: typeof raw.agent_id === "string" ? raw.agent_id : undefined,
        from_agent_id: typeof raw.from_agent_id === "string" ? raw.from_agent_id : undefined,
        to_agent_id: typeof raw.to_agent_id === "string" ? raw.to_agent_id : undefined,
        message: typeof raw.message === "string" ? raw.message : undefined,
        summary: typeof raw.summary === "string" ? raw.summary : undefined,
        timestamp: (typeof raw.timestamp === "string" && raw.timestamp) ||
            (typeof raw.ts === "string" && raw.ts) ||
            (typeof raw.created_at === "string" && raw.created_at) ||
            undefined,
        in_reply_to: typeof raw.in_reply_to === "string" ? raw.in_reply_to : undefined,
    };
}
function loadTail(absPath, maxMeshLines) {
    if (!fs.existsSync(absPath)) {
        return [];
    }
    const raw = fs.readFileSync(absPath, "utf8");
    const lines = raw.split("\n").filter((l) => l.trim());
    // Scan a deep window — file mixes probe noise with correspondence.
    const slice = lines.slice(Math.max(0, lines.length - Math.max(maxMeshLines * 20, 2000)));
    const out = [];
    for (const line of slice) {
        try {
            const parsed = JSON.parse(line);
            if (!isMeshCorrespondence(parsed)) {
                continue;
            }
            out.push(normalizeFeedLine(parsed));
        }
        catch {
            /* skip bad */
        }
    }
    return out.slice(Math.max(0, out.length - maxMeshLines));
}
function isWakeStubBody(body) {
    const b = body.trim();
    if (!b) {
        return false;
    }
    // Short doorbells from wake_paste.go — not full feed substance.
    if (b.startsWith("ATTN PEER") || b.startsWith("ATTN TPM")) {
        return true;
    }
    if (b.includes("(substance on feed)")) {
        return true;
    }
    return false;
}
function renderHtml(events, absPath, showNoise, seatFilter) {
    const rows = [];
    for (const ev of events) {
        const typ = (ev.event_type || "").trim();
        if (!showNoise && SKIP_TYPES.has(typ)) {
            continue;
        }
        if (!showNoise && typ === "delivery_receipt") {
            continue;
        }
        const from = (ev.from_agent_id || ev.agent_id || "?").trim();
        const to = (ev.to_agent_id || "").trim();
        if (seatFilter) {
            const fromBase = from.toLowerCase();
            const toBase = to.toLowerCase();
            if (!fromBase.includes(seatFilter) && !toBase.includes(seatFilter)) {
                continue;
            }
        }
        // Prefer full message; summary alone is never enough for COMMS substance.
        const body = (ev.message || ev.summary || "").trim();
        if (!body && !typ) {
            continue;
        }
        const wakeStub = isWakeStubBody(body) || typ === "wake";
        const typeLabel = wakeStub ? `${typ || "event"} · wake/notify stub` : typ || "event";
        const arrow = to ? ` → <span style="color:${seatColor(to)}">${escapeHtml(to)}</span>` : "";
        const when = formatWhen(ev.timestamp);
        rows.push(`
      <div class="row ${escapeHtml(typ)} ${wakeStub ? "wake-stub" : "substance"}">
        <div class="meta">
          <span class="time">${escapeHtml(when)}</span>
          <span class="type">${escapeHtml(typeLabel)}</span>
          <span class="from" style="color:${seatColor(from)}">${escapeHtml(from)}</span>${arrow}
          ${ev.event_id ? `<span class="id">${escapeHtml(ev.event_id)}</span>` : ""}
          ${wakeStub ? `<span class="badge">notify-only</span>` : `<span class="badge substance-badge">feed substance</span>`}
        </div>
        <div class="body">${escapeHtml(body)}</div>
      </div>`);
    }
    return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8" />
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline';" />
<style>
  body {
    font-family: var(--vscode-font-family);
    font-size: var(--vscode-font-size);
    color: var(--vscode-foreground);
    background: var(--vscode-editor-background);
    margin: 0;
    padding: 12px 14px 48px;
  }
  h1 { font-size: 14px; font-weight: 600; margin: 0 0 4px; }
  .sub { opacity: 0.7; font-size: 12px; margin-bottom: 14px; word-break: break-all; }
  .toolbar { margin-bottom: 12px; display: flex; gap: 8px; flex-wrap: wrap; }
  button {
    background: var(--vscode-button-background);
    color: var(--vscode-button-foreground);
    border: none;
    padding: 4px 10px;
    cursor: pointer;
  }
  .row {
    border-left: 3px solid var(--vscode-panel-border);
    padding: 8px 10px;
    margin: 0 0 8px;
    background: color-mix(in srgb, var(--vscode-editor-background) 88%, var(--vscode-foreground) 12%);
  }
  .row.steering { border-left-color: #6cb6ff; }
  .row.mesh_status { border-left-color: #7ee787; }
  .row.peer_ack { border-left-color: #d2a8ff; }
  .row.delivery_receipt { border-left-color: #8b949e; opacity: 0.85; }
  .row.wake-stub { border-left-color: #ffa657; opacity: 0.9; }
  .row.substance { border-left-width: 4px; }
  .badge {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    opacity: 0.75;
    border: 1px solid color-mix(in srgb, var(--vscode-foreground) 35%, transparent);
    padding: 0 6px;
    border-radius: 3px;
  }
  .substance-badge { opacity: 0.9; }
  .chip { background: transparent; border: 1px solid transparent; opacity: 0.6; }
  .chip:hover { opacity: 0.8; }
  .chip.active { opacity: 1; border-bottom: 2px solid currentColor; background: var(--vscode-editor-selectionBackground); }
  .meta { font-size: 11px; opacity: 0.85; margin-bottom: 4px; display: flex; flex-wrap: wrap; gap: 8px; }
  .from { font-weight: 600; }
  .id { opacity: 0.55; font-family: var(--vscode-editor-font-family); }
  .body { white-space: pre-wrap; word-break: break-word; line-height: 1.35; }
  .empty { opacity: 0.7; padding: 24px 0; }
</style>
</head>
<body>
  <h1>ZQK Mesh Feed</h1>
  <div class="sub">${escapeHtml(absPath)} · full correspondence (wake/notify stubs labeled; COMMS substance is message body)</div>
  <div class="toolbar">
    <button id="refresh">Refresh</button>
    <button id="toggleNoise">${showNoise ? "Hide kernel/receipts" : "Show kernel/receipts"}</button>
  </div>
  <div class="toolbar" id="seat-chips">
    <button class="chip ${!seatFilter ? "active" : ""}" onclick="vscode.postMessage({type: 'filterSeat', seat: null})">All Seats</button>
    <button class="chip ${seatFilter === 'cursor' ? "active" : ""}" style="color:#6cb6ff" onclick="vscode.postMessage({type: 'filterSeat', seat: 'cursor'})">cursor</button>
    <button class="chip ${seatFilter === 'antigravity-1' ? "active" : ""}" style="color:#7ee787" onclick="vscode.postMessage({type: 'filterSeat', seat: 'antigravity-1'})">antigravity-1</button>
    <button class="chip ${seatFilter === 'antigravity-2' ? "active" : ""}" style="color:#d2a8ff" onclick="vscode.postMessage({type: 'filterSeat', seat: 'antigravity-2'})">antigravity-2</button>
  </div>
  ${rows.length
        ? rows.join("\n")
        : `<div class="empty">No mesh events yet. Run <code>zqk feed steer</code> / <code>proof-of-life</code> or wait for seats to reply.</div>`}
  <script>
    const vscode = acquireVsCodeApi();
    document.getElementById('refresh')?.addEventListener('click', () => vscode.postMessage({ type: 'refresh' }));
    document.getElementById('toggleNoise')?.addEventListener('click', () => vscode.postMessage({ type: 'toggleNoise' }));
  </script>
</body>
</html>`;
}
function escapeHtml(s) {
    return s
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;");
}
/**
 * Human-facing live transcript of the shared agent feed (3-way mesh visibility).
 * Not a toast — this is where the conversation is readable.
 */
class MeshFeedPanel {
    static show(context, feedAbsPath) {
        const col = vscode.window.activeTextEditor?.viewColumn ?? vscode.ViewColumn.Beside;
        if (MeshFeedPanel.current) {
            MeshFeedPanel.current.panel.reveal(col);
            MeshFeedPanel.current.refresh();
            return;
        }
        const panel = vscode.window.createWebviewPanel(MeshFeedPanel.viewType, "ZQK Mesh Feed", col, { enableScripts: true, retainContextWhenHidden: true });
        MeshFeedPanel.current = new MeshFeedPanel(panel, feedAbsPath, context);
    }
    constructor(panel, feedAbsPath, context) {
        this.feedAbsPath = feedAbsPath;
        this.disposables = [];
        this.showNoise = false;
        this.seatFilter = null;
        this.panel = panel;
        this.panel.onDidDispose(() => this.dispose(), null, this.disposables);
        this.panel.webview.onDidReceiveMessage((msg) => {
            if (msg?.type === "refresh") {
                this.refresh();
            }
            if (msg?.type === "toggleNoise") {
                this.showNoise = !this.showNoise;
                this.refresh();
            }
            if (msg?.type === "filterSeat") {
                this.seatFilter = msg.seat;
                this.refresh();
            }
        }, null, this.disposables);
        this.watch();
        this.refresh();
        context.subscriptions.push({ dispose: () => this.dispose() });
    }
    setFeedPath(abs) {
        this.feedAbsPath = abs;
        this.watch();
        this.refresh();
    }
    watch() {
        this.watcher?.close();
        this.vscodeWatcher?.dispose();
        const schedule = () => {
            if (this.timer) {
                clearTimeout(this.timer);
            }
            this.timer = setTimeout(() => this.refresh(), 200);
        };
        try {
            const dir = path.dirname(this.feedAbsPath);
            fs.mkdirSync(dir, { recursive: true });
            if (!fs.existsSync(this.feedAbsPath)) {
                fs.writeFileSync(this.feedAbsPath, "", { encoding: "utf8" });
            }
            this.watcher = fs.watch(this.feedAbsPath, () => schedule());
        }
        catch {
            /* appear later */
        }
        this.vscodeWatcher = vscode.workspace.createFileSystemWatcher(new vscode.RelativePattern(path.dirname(this.feedAbsPath), path.basename(this.feedAbsPath)));
        this.vscodeWatcher.onDidChange(schedule);
        this.vscodeWatcher.onDidCreate(schedule);
    }
    refresh() {
        const events = loadTail(this.feedAbsPath, 200);
        this.panel.webview.html = renderHtml(events, this.feedAbsPath, this.showNoise, this.seatFilter);
    }
    dispose() {
        MeshFeedPanel.current = undefined;
        if (this.timer) {
            clearTimeout(this.timer);
        }
        this.watcher?.close();
        this.vscodeWatcher?.dispose();
        while (this.disposables.length) {
            this.disposables.pop()?.dispose();
        }
    }
}
exports.MeshFeedPanel = MeshFeedPanel;
MeshFeedPanel.viewType = "zqk.meshFeed";
//# sourceMappingURL=meshFeed.js.map