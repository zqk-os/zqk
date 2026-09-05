"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.CursorTargets = exports.Commands = exports.LEGACY_PATH_MIGRATIONS = exports.DEFAULT_FEED_JSONL = exports.DEFAULT_CONTROL_JSONL = exports.CONTROL_SCHEMA = exports.OUTPUT_CHANNEL = exports.CONFIG_SECTION = void 0;
/** Stable zqk.* command ids and config section. */
exports.CONFIG_SECTION = "zqkIdeBridge";
exports.OUTPUT_CHANNEL = "ZQK IDE Bridge";
exports.CONTROL_SCHEMA = "zqk_ide_bridge_v1";
/** Canonical control bus (matches pkg/idebridge + pkg/paths IDEHooksLogsSubdir). */
exports.DEFAULT_CONTROL_JSONL = ".zqk/logs/ide-hooks/ide_bridge_control.jsonl";
/** Mesh feed transcript (matches pkg/idebridge + pkg/paths IDEHooksLogsSubdir). */
exports.DEFAULT_FEED_JSONL = ".zqk/logs/ide-hooks/agent_chat_channel.jsonl";
/**
 * Pre-Customize defaults that lite configs still carry. The kernel only writes
 * ide-hooks; cursor-hooks now collects unrelated Cursor hook probe telemetry, so
 * a stale setting silently shows a feed without peer seats.
 */
exports.LEGACY_PATH_MIGRATIONS = {
    ".zqk/logs/cursor-hooks/ide_bridge_control.jsonl": exports.DEFAULT_CONTROL_JSONL,
    ".zqk/logs/cursor-hooks/agent_chat_channel.jsonl": exports.DEFAULT_FEED_JSONL,
};
exports.Commands = {
    showStatus: "zqk.ideBridge.showStatus",
    mcpReloadClient: "zqk.mcp.reloadClient",
    mcpProbeAll: "zqk.mcp.probeAllServers",
    mcpRetryExhausted: "zqk.mcp.retryExhaustedServers",
    mcpOpenSettings: "zqk.mcp.openSettings",
    chatNew: "zqk.chat.new",
    chatCancel: "zqk.chat.cancel",
    chatRename: "zqk.chat.rename",
    chatFocus: "zqk.chat.focus",
    modeAgent: "zqk.mode.agent",
    modePlan: "zqk.mode.plan",
    modeMultitask: "zqk.mode.multitask",
    modeDebug: "zqk.mode.debug",
    modeChat: "zqk.mode.chat",
    /** Core (always on): Cursor composer inject for ATTN; toast only for proof-of-life. */
    wakeAttn: "zqk.wake.attn",
    /** Human-readable live transcript of the shared agent feed (3-way visibility). */
    meshShowFeed: "zqk.mesh.showFeed",
};
/**
 * Cursor / VS Code private targets — owned mapping lives only in capability packs.
 *
 * Cursor 2026 moved MCP ops under Customize (`workbench.action.customize.openMCPs`).
 * Old Settings → MCP open id (`aiSettings.action.open.mcp`) and
 * `mcp.probeAllServers` / `mcp.retryExhaustedServers` are gone from the product bundle.
 * TRACK: BLI-MCP-CURSOR-ADAPTER-SYMLINK-001 — keep bridge packs aligned to Customize.
 */
exports.CursorTargets = {
    mcpReloadClient: "mcp.reloadClient",
    /** Replaces removed mcp.probeAllServers — refreshes tool/offering snapshot. */
    mcpRefreshSnapshot: "mcp.refreshSnapshot",
    mcpOpenSettings: "workbench.action.customize.openMCPs",
    /** Fallbacks if Customize command id drifts across Cursor builds. */
    mcpOpenSettingsFallbacks: [
        "workbench.action.openCustomizeEditor",
        "aiSettings.action.open",
    ],
    chatNew: "composer.newAgentChat",
    chatCancel: "composer.cancelChat",
    chatRename: "composer.renameChat",
    chatFocus: "composer.focusComposer",
    modeAgent: "composerMode.agent",
    modePlan: "composerMode.plan",
    modeMultitask: "composerMode.multitask",
    modeDebug: "composerMode.debug",
    modeChat: "composerMode.chat",
};
//# sourceMappingURL=constants.js.map