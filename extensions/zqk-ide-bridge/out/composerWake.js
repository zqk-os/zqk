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
exports.PROOF_OF_LIFE_PREFIX = void 0;
exports.isProofOfLifePulse = isProofOfLifePulse;
exports.injectComposerTurn = injectComposerTurn;
const child_process_1 = require("child_process");
const util_1 = require("util");
const vscode = __importStar(require("vscode"));
const constants_1 = require("./constants");
const execFileAsync = (0, util_1.promisify)(child_process_1.execFile);
/** Kernel proof-of-life pulses must not start a Composer turn. */
exports.PROOF_OF_LIFE_PREFIX = "PROOF-OF-LIFE";
/**
 * VS Code/Cursor command ids that *might* submit. Composer input is a webview:
 * these often no-op (or succeed without sending). Darwin uses Return after paste.
 */
const SUBMIT_COMMANDS = [
    "composer.startGeneration",
    "workbench.action.chat.submit",
    "composer.submit",
    "composer.submitChat",
];
/** Skip restoring a capture that is almost certainly the wrong pane (full file). */
const MAX_RESTORE_CHARS = 200000;
function delay(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
}
function isProofOfLifePulse(text) {
    return text.trim().toUpperCase().startsWith(exports.PROOF_OF_LIFE_PREFIX);
}
/**
 * Start a turn in the *existing* Composer session (do not open a new chat).
 *
 * Capture any in-progress Composer draft, clear it, paste ATTN, submit, then
 * put the draft back. Fast enough that the user is less likely to lose work.
 * OS clipboard is restored in `finally`.
 *
 * Paste uses the VS Code clipboard action (hits the focused Composer webview).
 * Submit does not: startGeneration/chat.submit are unregistered or no-ops, and
 * `type` newline lands in the text editor. Phase A submits with System Events
 * key code 36 (Return) into the Cursor process — do that from the extension
 * after paste so kernel `--chat` stays opt-in.
 *
 * TRACK: BLI-COMMS-CURSOR-COMPOSER-INJECT-001 — replace clipboard+Return when
 * Cursor ships a supported prompt+submit command for the current agent chat.
 */
async function injectComposerTurn(text, output) {
    const log = (line) => {
        output?.appendLine(`[wake-inject] ${line}`);
    };
    const prompt = text.trim();
    if (!prompt) {
        log("empty prompt");
        return { pasted: false, submitted: false };
    }
    try {
        await vscode.commands.executeCommand(constants_1.CursorTargets.modeAgent);
        log("composerMode.agent");
    }
    catch (e) {
        log(`composerMode.agent skipped: ${errText(e)}`);
    }
    try {
        await vscode.commands.executeCommand(constants_1.CursorTargets.chatFocus);
        log("composer.focusComposer");
    }
    catch (e) {
        log(`focusComposer failed: ${errText(e)}`);
        return { pasted: false, submitted: false };
    }
    await delay(200);
    let previousClipboard = "";
    try {
        previousClipboard = await vscode.env.clipboard.readText();
    }
    catch {
        previousClipboard = "";
    }
    try {
        const draft = await captureComposerDraft(log);
        await writeClipboardAndPaste(prompt, log, { selectAllFirst: true });
        log("clipboard paste ATTN over cleared composer");
        await delay(300);
        try {
            await vscode.commands.executeCommand(constants_1.CursorTargets.chatFocus);
        }
        catch (e) {
            log(`refocus before submit skipped: ${errText(e)}`);
        }
        await delay(80);
        const submitted = await submitComposer(log);
        if (draft) {
            await delay(250);
            try {
                await vscode.commands.executeCommand(constants_1.CursorTargets.chatFocus);
            }
            catch (e) {
                log(`refocus before restore skipped: ${errText(e)}`);
            }
            await delay(80);
            await writeClipboardAndPaste(draft, log, { selectAllFirst: false });
            log(`restored composer draft (${draft.length} chars)`);
        }
        return { pasted: true, submitted };
    }
    catch (e) {
        log(`paste failed: ${errText(e)}`);
        return { pasted: false, submitted: false };
    }
    finally {
        try {
            await vscode.env.clipboard.writeText(previousClipboard);
        }
        catch {
            /* ignore */
        }
    }
}
async function captureComposerDraft(log) {
    const sentinel = `__zqk_wake_draft_sentinel_${Date.now()}__`;
    try {
        await vscode.env.clipboard.writeText(sentinel);
    }
    catch (e) {
        log(`sentinel clipboard write skipped: ${errText(e)}`);
        return "";
    }
    await selectAllFocused(log);
    await delay(50);
    await copyFocused(log);
    await delay(80);
    let captured = "";
    try {
        captured = await vscode.env.clipboard.readText();
    }
    catch {
        captured = "";
    }
    if (!captured || captured === sentinel) {
        log("composer draft empty or copy missed");
        return "";
    }
    if (captured.length > MAX_RESTORE_CHARS) {
        log(`composer draft capture ${captured.length} chars — skip restore (likely wrong pane)`);
        return "";
    }
    log(`captured composer draft (${captured.length} chars)`);
    return captured;
}
async function writeClipboardAndPaste(text, log, opts) {
    if (opts.selectAllFirst) {
        await selectAllFocused(log);
        await delay(40);
    }
    await vscode.env.clipboard.writeText(text);
    await vscode.commands.executeCommand("editor.action.clipboardPasteAction");
}
async function selectAllFocused(log) {
    if (process.platform === "darwin") {
        if (await darwinCommandKeystroke("a", log, "select-all")) {
            return;
        }
    }
    try {
        await vscode.commands.executeCommand("editor.action.selectAll");
    }
    catch (e) {
        log(`selectAll skipped: ${errText(e)}`);
    }
}
async function copyFocused(log) {
    if (process.platform === "darwin") {
        if (await darwinCommandKeystroke("c", log, "copy")) {
            return;
        }
    }
    try {
        await vscode.commands.executeCommand("editor.action.clipboardCopyAction");
    }
    catch (e) {
        log(`copy skipped: ${errText(e)}`);
    }
}
async function submitComposer(log) {
    if (process.platform === "darwin") {
        const viaReturn = await submitViaReturnKey(log);
        if (viaReturn) {
            return true;
        }
        log("Return keystroke failed — trying registered submit commands");
    }
    return trySubmitCommands(log);
}
async function trySubmitCommands(log) {
    let registered = [];
    try {
        registered = await vscode.commands.getCommands(true);
    }
    catch (e) {
        log(`getCommands failed: ${errText(e)}`);
    }
    const set = new Set(registered);
    for (const cmd of SUBMIT_COMMANDS) {
        if (!set.has(cmd)) {
            log(`submit ${cmd} not registered`);
            continue;
        }
        try {
            await vscode.commands.executeCommand(cmd);
            log(`submit ${cmd} executed (may still be a no-op)`);
            return true;
        }
        catch (e) {
            log(`submit ${cmd} failed: ${errText(e)}`);
        }
    }
    try {
        await vscode.commands.executeCommand("type", { text: "\n" });
        log("submit via type newline (editor, not composer webview — last resort)");
    }
    catch (e) {
        log(`type newline skipped: ${errText(e)}`);
    }
    return false;
}
/**
 * Same submit as cmd/zqk/scheduler/ide-agent-paste.applescript (key code 36).
 * Requires Accessibility for Cursor (already granted for Phase A --chat).
 */
async function submitViaReturnKey(log) {
    return darwinSystemEvents(["-e", "key code 36"], log, "submit via System Events key code 36 (Return)");
}
async function darwinCommandKeystroke(key, log, label) {
    return darwinSystemEvents(["-e", `keystroke "${key}" using command down`], log, label);
}
async function darwinSystemEvents(extra, log, label) {
    try {
        await execFileAsync("osascript", [
            "-e",
            'tell application "System Events"',
            "-e",
            'tell (first application process whose name is "Cursor")',
            "-e",
            "set frontmost to true",
            ...extra,
            "-e",
            "end tell",
            "-e",
            "end tell",
        ], { timeout: 5000 });
        log(label);
        return true;
    }
    catch (e) {
        log(`${label} failed: ${errText(e)}`);
        return false;
    }
}
function errText(e) {
    return e instanceof Error ? e.message : String(e);
}
//# sourceMappingURL=composerWake.js.map