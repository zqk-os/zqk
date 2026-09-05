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
exports.ControlBus = void 0;
const fs = __importStar(require("fs"));
const path = __importStar(require("path"));
const vscode = __importStar(require("vscode"));
const constants_1 = require("./constants");
/**
 * Append-only control bus. MCP/daemon writes; extension executes last new lines.
 * Protocol stays thin so capability packs can evolve independently.
 */
class ControlBus {
    constructor(absPath, debounceMs, onEvents, output) {
        this.absPath = absPath;
        this.debounceMs = debounceMs;
        this.onEvents = onEvents;
        this.output = output;
        this.offset = 0;
    }
    start() {
        const dir = path.dirname(this.absPath);
        try {
            fs.mkdirSync(dir, { recursive: true });
            if (!fs.existsSync(this.absPath)) {
                fs.writeFileSync(this.absPath, "", { encoding: "utf8", mode: 0o600 });
            }
            this.offset = fs.statSync(this.absPath).size;
        }
        catch (e) {
            this.output.appendLine(`[control] init failed: ${String(e)}`);
        }
        const schedule = () => {
            if (this.timer) {
                clearTimeout(this.timer);
            }
            this.timer = setTimeout(() => {
                void this.drain();
            }, this.debounceMs);
        };
        try {
            this.watcher = fs.watch(this.absPath, () => schedule());
        }
        catch {
            /* file may appear later */
        }
        this.vscodeWatcher = vscode.workspace.createFileSystemWatcher(new vscode.RelativePattern(path.dirname(this.absPath), path.basename(this.absPath)));
        this.vscodeWatcher.onDidChange(schedule);
        this.vscodeWatcher.onDidCreate(schedule);
        return {
            dispose: () => {
                if (this.timer) {
                    clearTimeout(this.timer);
                }
                this.watcher?.close();
                this.vscodeWatcher?.dispose();
            },
        };
    }
    async drain() {
        let buf;
        try {
            const st = fs.statSync(this.absPath);
            if (st.size < this.offset) {
                this.offset = 0; // truncated / rotated
            }
            if (st.size === this.offset) {
                return;
            }
            const fd = fs.openSync(this.absPath, "r");
            try {
                const len = st.size - this.offset;
                const b = Buffer.alloc(len);
                fs.readSync(fd, b, 0, len, this.offset);
                buf = b.toString("utf8");
                this.offset = st.size;
            }
            finally {
                fs.closeSync(fd);
            }
        }
        catch (e) {
            this.output.appendLine(`[control] read failed: ${String(e)}`);
            return;
        }
        const events = [];
        for (const line of buf.split(/\r?\n/)) {
            const t = line.trim();
            if (!t) {
                continue;
            }
            try {
                const o = JSON.parse(t);
                if (o && o.schema === constants_1.CONTROL_SCHEMA && typeof o.command === "string") {
                    events.push(o);
                }
                else {
                    this.output.appendLine(`[control] skip non-v1 line`);
                }
            }
            catch {
                this.output.appendLine(`[control] skip bad JSON`);
            }
        }
        if (events.length) {
            await this.onEvents(events);
        }
    }
}
exports.ControlBus = ControlBus;
//# sourceMappingURL=controlBus.js.map