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
exports.mcpCapability = void 0;
const vscode = __importStar(require("vscode"));
const constants_1 = require("../constants");
const guard_1 = require("./guard");
async function runFirstAvailable(ctx, candidates, label) {
    for (const c of candidates) {
        const ok = await ctx.runCursor(c.command, { soft: true }, ...(c.args ?? []));
        if (ok) {
            ctx.output.appendLine(`[mcp] ${label} → ${c.command}`);
            return true;
        }
    }
    ctx.output.appendLine(`[mcp] ${label} failed for all candidates`);
    void vscode.window.showErrorMessage(`ZQK IDE Bridge: ${label} failed — Cursor MCP/Customize commands may have changed; check Output → ZQK IDE Bridge`);
    return false;
}
exports.mcpCapability = {
    id: "mcp",
    activate(ctx) {
        return [
            vscode.commands.registerCommand(constants_1.Commands.mcpReloadClient, async () => {
                if (!(0, guard_1.requireCapability)(ctx, "mcp", constants_1.Commands.mcpReloadClient)) {
                    return;
                }
                const identifier = ctx.getConfig().mcpServerIdentifier;
                if (!identifier) {
                    void vscode.window.showErrorMessage("ZQK IDE Bridge: set zqkIdeBridge.mcpServerIdentifier before reload.");
                    return;
                }
                await runFirstAvailable(ctx, [
                    {
                        command: constants_1.CursorTargets.mcpReloadClient,
                        args: [{ identifier }],
                    },
                ], "reloadClient");
            }),
            vscode.commands.registerCommand(constants_1.Commands.mcpProbeAll, async () => {
                if (!(0, guard_1.requireCapability)(ctx, "mcp", constants_1.Commands.mcpProbeAll)) {
                    return;
                }
                const identifier = ctx.getConfig().mcpServerIdentifier;
                // Cursor removed mcp.probeAllServers; refreshSnapshot + reload is the Customize-era path.
                const candidates = [];
                if (identifier) {
                    candidates.push({
                        command: constants_1.CursorTargets.mcpRefreshSnapshot,
                        args: [{ identifier }],
                    });
                    candidates.push({
                        command: constants_1.CursorTargets.mcpReloadClient,
                        args: [{ identifier }],
                    });
                }
                candidates.push({ command: constants_1.CursorTargets.mcpRefreshSnapshot });
                await runFirstAvailable(ctx, candidates, "probe/refresh");
            }),
            vscode.commands.registerCommand(constants_1.Commands.mcpRetryExhausted, async () => {
                if (!(0, guard_1.requireCapability)(ctx, "mcp", constants_1.Commands.mcpRetryExhausted)) {
                    return;
                }
                const identifier = ctx.getConfig().mcpServerIdentifier;
                if (!identifier) {
                    void vscode.window.showErrorMessage("ZQK IDE Bridge: set zqkIdeBridge.mcpServerIdentifier before retry.");
                    return;
                }
                // mcp.retryExhaustedServers removed; reloadClient is the recovery actuate.
                await runFirstAvailable(ctx, [
                    {
                        command: constants_1.CursorTargets.mcpReloadClient,
                        args: [{ identifier }],
                    },
                    {
                        command: constants_1.CursorTargets.mcpRefreshSnapshot,
                        args: [{ identifier }],
                    },
                ], "retryExhausted→reload");
            }),
            vscode.commands.registerCommand(constants_1.Commands.mcpOpenSettings, async () => {
                if (!(0, guard_1.requireCapability)(ctx, "mcp", constants_1.Commands.mcpOpenSettings)) {
                    return;
                }
                const candidates = [
                    { command: constants_1.CursorTargets.mcpOpenSettings },
                    ...constants_1.CursorTargets.mcpOpenSettingsFallbacks.map((command) => ({ command })),
                ];
                await runFirstAvailable(ctx, candidates, "openCustomizeMCPs");
            }),
        ];
    },
};
//# sourceMappingURL=mcp.js.map