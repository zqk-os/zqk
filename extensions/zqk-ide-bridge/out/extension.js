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
exports.activate = activate;
exports.deactivate = deactivate;
const path = __importStar(require("path"));
const vscode = __importStar(require("vscode"));
const capabilities_1 = require("./capabilities");
const config_1 = require("./config");
const constants_1 = require("./constants");
const controlBus_1 = require("./controlBus");
const composerWake_1 = require("./composerWake");
const meshFeed_1 = require("./meshFeed");
let output;
let controlDisposable;
let proofOfLifeBar;
let lastProofOfLife = "";
let extContext;
function isBestEffortCommand(command) {
    return command === constants_1.Commands.wakeAttn || command.startsWith("zqk.wake.");
}
async function runCursor(command, opts, ...args) {
    const soft = opts.soft || isBestEffortCommand(command);
    try {
        await vscode.commands.executeCommand(command, ...args);
        output?.appendLine(`[ok] ${command}`);
        return true;
    }
    catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        output?.appendLine(`[fail] ${command}: ${msg}`);
        if (!soft) {
            void vscode.window.showErrorMessage(`ZQK IDE Bridge: ${command} failed — ${msg}`);
        }
        return false;
    }
}
function resolveWorkspacePath(rel) {
    if (!rel) {
        return undefined;
    }
    if (path.isAbsolute(rel)) {
        return rel;
    }
    const folder = vscode.workspace.workspaceFolders?.[0];
    if (!folder) {
        return undefined;
    }
    return path.join(folder.uri.fsPath, rel);
}
function openMeshFeed() {
    if (!extContext) {
        return;
    }
    const cfg = (0, config_1.getBridgeConfig)();
    const abs = resolveWorkspacePath(cfg.feedJsonl);
    if (!abs) {
        void vscode.window.showWarningMessage("ZQK Mesh Feed: open a workspace folder first");
        return;
    }
    meshFeed_1.MeshFeedPanel.show(extContext, abs);
}
function ensureProofOfLifeBar(context) {
    if (proofOfLifeBar) {
        return proofOfLifeBar;
    }
    const bar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
    bar.command = constants_1.Commands.meshShowFeed;
    bar.text = "$(comment-discussion) ZQK mesh";
    bar.tooltip = "Open ZQK Mesh Feed. Steer ATTN injects into Composer; proof-of-life stays on this bar.";
    bar.show();
    context.subscriptions.push(bar);
    proofOfLifeBar = bar;
    return bar;
}
function pulseProofOfLifeIndicator(message) {
    const cfg = (0, config_1.getBridgeConfig)();
    if (!cfg.proofOfLifeStatusBar || !proofOfLifeBar) {
        return;
    }
    const when = new Date().toLocaleTimeString();
    lastProofOfLife = message;
    proofOfLifeBar.text = "$(pulse) ZQK alive";
    proofOfLifeBar.tooltip = `${message}\nLast pulse: ${when}\nClick to open Mesh Feed`;
    proofOfLifeBar.backgroundColor = new vscode.ThemeColor("statusBarItem.warningBackground");
    setTimeout(() => {
        if (!proofOfLifeBar) {
            return;
        }
        proofOfLifeBar.text = "$(comment-discussion) ZQK mesh";
        proofOfLifeBar.backgroundColor = undefined;
    }, 2500);
}
async function handleWakeAttn(message) {
    const text = typeof message === "string" && message.trim()
        ? message.trim()
        : "PROOF-OF-LIFE: mesh back-channel active (chat may be quiet — work continues on the feed)";
    output?.appendLine(`[wake] ${text}`);
    pulseProofOfLifeIndicator(text);
    const cfg = (0, config_1.getBridgeConfig)();
    const pulseOnly = (0, composerWake_1.isProofOfLifePulse)(text);
    if (cfg.wakeInjectComposer && !pulseOnly) {
        const injected = await (0, composerWake_1.injectComposerTurn)(text, output);
        if (injected.pasted && injected.submitted) {
            output?.appendLine("[wake] composer inject submitted (existing chat, not a new session)");
            return;
        }
        if (injected.pasted && !injected.submitted) {
            output?.appendLine("[wake] composer pasted but did not submit — falling back to toast");
        }
        else {
            output?.appendLine("[wake] composer inject failed — falling back to toast");
        }
    }
    if (cfg.proofOfLifeToast) {
        try {
            if (cfg.proofOfLifeOfferFeed) {
                const pick = await vscode.window.showInformationMessage(text, "Show Mesh Feed");
                if (pick === "Show Mesh Feed") {
                    openMeshFeed();
                }
            }
            else {
                await vscode.window.showInformationMessage(text);
            }
        }
        catch (e) {
            output?.appendLine(`[wake] toast skipped: ${e instanceof Error ? e.message : String(e)}`);
        }
    }
}
async function dispatchControl(ev) {
    const command = (ev.command || "").trim();
    if (!command) {
        return;
    }
    const args = Array.isArray(ev.args) ? ev.args : [];
    output?.appendLine(`[control] ${ev.request_id ?? "-"} → ${command}`);
    if (command === constants_1.Commands.wakeAttn) {
        await handleWakeAttn(args[0]);
        output?.appendLine(`[ok] ${command} (local)`);
        return;
    }
    if (command === constants_1.Commands.meshShowFeed) {
        openMeshFeed();
        output?.appendLine(`[ok] ${command} (local)`);
        return;
    }
    // Control-bus events invoke stable zqk.* commands (registered locally), not Cursor privates.
    await runCursor(command, { soft: isBestEffortCommand(command) }, ...args);
}
function restartControlBus(context) {
    controlDisposable?.dispose();
    controlDisposable = undefined;
    const cfg = (0, config_1.getBridgeConfig)();
    const abs = resolveWorkspacePath(cfg.controlJsonl);
    if (!cfg.enabled || !abs || !output) {
        output?.appendLine(`[control] watcher off (enabled=${cfg.enabled}, path=${cfg.controlJsonl || "—"})`);
        return;
    }
    const bus = new controlBus_1.ControlBus(abs, cfg.debounceMs, async (events) => {
        for (const ev of events) {
            await dispatchControl(ev);
        }
    }, output);
    controlDisposable = bus.start();
    context.subscriptions.push(controlDisposable);
    output.appendLine(`[control] watching ${abs}`);
    output.appendLine(`[status] capabilities=${cfg.capabilities.join(",")} mcpId=${cfg.mcpServerIdentifier}`);
}
function activate(context) {
    extContext = context;
    output = vscode.window.createOutputChannel(constants_1.OUTPUT_CHANNEL);
    context.subscriptions.push(output);
    ensureProofOfLifeBar(context);
    const capCtx = {
        getConfig: config_1.getBridgeConfig,
        output,
        runCursor: (command, opts, ...args) => runCursor(command, { soft: Boolean(opts?.soft) || isBestEffortCommand(command) }, ...args),
    };
    for (const pack of capabilities_1.ALL_CAPABILITIES) {
        for (const d of pack.activate(capCtx)) {
            context.subscriptions.push(d);
        }
    }
    context.subscriptions.push(vscode.commands.registerCommand(constants_1.Commands.showStatus, () => {
        const cfg = (0, config_1.getBridgeConfig)();
        const abs = resolveWorkspacePath(cfg.controlJsonl);
        const feed = resolveWorkspacePath(cfg.feedJsonl);
        output?.show(true);
        for (const line of [
            `enabled: ${cfg.enabled}`,
            `capabilities: ${cfg.capabilities.join(", ") || "(none)"}`,
            `mcpServerIdentifier: ${cfg.mcpServerIdentifier}`,
            `controlJsonl: ${cfg.controlJsonl || "(disabled)"}`,
            `control resolved: ${abs ?? "(no workspace)"}`,
            `feedJsonl: ${cfg.feedJsonl}`,
            `feed resolved: ${feed ?? "(no workspace)"}`,
            `lastProofOfLife: ${lastProofOfLife || "(none yet)"}`,
        ]) {
            output?.appendLine(line);
        }
        void vscode.window.showInformationMessage(lastProofOfLife
            ? `ZQK mesh: ${lastProofOfLife}`
            : `ZQK IDE Bridge: ${cfg.capabilities.join(", ")}`);
    }), vscode.commands.registerCommand(constants_1.Commands.wakeAttn, async (message) => {
        await handleWakeAttn(message);
    }), vscode.commands.registerCommand(constants_1.Commands.meshShowFeed, () => {
        openMeshFeed();
    }));
    restartControlBus(context);
    context.subscriptions.push(vscode.workspace.onDidChangeConfiguration((e) => {
        if (e.affectsConfiguration(constants_1.CONFIG_SECTION)) {
            restartControlBus(context);
            if (proofOfLifeBar) {
                if ((0, config_1.getBridgeConfig)().proofOfLifeStatusBar) {
                    proofOfLifeBar.show();
                }
                else {
                    proofOfLifeBar.hide();
                }
            }
        }
    }));
}
function deactivate() {
    controlDisposable?.dispose();
    controlDisposable = undefined;
    proofOfLifeBar = undefined;
    extContext = undefined;
}
//# sourceMappingURL=extension.js.map