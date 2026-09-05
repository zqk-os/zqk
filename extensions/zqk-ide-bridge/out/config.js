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
exports.getBridgeConfig = getBridgeConfig;
exports.capabilityEnabled = capabilityEnabled;
const vscode = __importStar(require("vscode"));
const constants_1 = require("./constants");
const ALL = ["mcp", "chat", "modes"];
function jsonlPath(c, key, fallback) {
    const configured = (c.get(key) ?? fallback).trim();
    return constants_1.LEGACY_PATH_MIGRATIONS[configured] ?? configured;
}
function getBridgeConfig() {
    const c = vscode.workspace.getConfiguration(constants_1.CONFIG_SECTION);
    const rawCaps = c.get("capabilities") ?? ["mcp"];
    const capabilities = rawCaps.filter((x) => ALL.includes(x));
    return {
        enabled: c.get("enabled") ?? true,
        capabilities: capabilities.length ? capabilities : ["mcp"],
        controlJsonl: jsonlPath(c, "controlJsonl", constants_1.DEFAULT_CONTROL_JSONL),
        feedJsonl: jsonlPath(c, "feedJsonl", constants_1.DEFAULT_FEED_JSONL),
        // Cursor Customize catalogs project MCP as user-<name> (e.g. user-zqk).
        mcpServerIdentifier: (c.get("mcpServerIdentifier") ?? "user-zqk").trim(),
        debounceMs: c.get("debounceMs") ?? 200,
        wakeInjectComposer: c.get("wakeInjectComposer") ?? true,
        proofOfLifeToast: c.get("proofOfLifeToast") ?? true,
        proofOfLifeStatusBar: c.get("proofOfLifeStatusBar") ?? true,
        proofOfLifeOfferFeed: c.get("proofOfLifeOfferFeed") ?? true,
    };
}
function capabilityEnabled(cfg, id) {
    return cfg.enabled && cfg.capabilities.includes(id);
}
//# sourceMappingURL=config.js.map