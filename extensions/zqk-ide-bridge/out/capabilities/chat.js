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
exports.chatCapability = void 0;
const vscode = __importStar(require("vscode"));
const constants_1 = require("../constants");
const guard_1 = require("./guard");
exports.chatCapability = {
    id: "chat",
    activate(ctx) {
        const wrap = (zqkCmd, cursorCmd) => vscode.commands.registerCommand(zqkCmd, async () => {
            if (!(0, guard_1.requireCapability)(ctx, "chat", zqkCmd)) {
                return;
            }
            await ctx.runCursor(cursorCmd);
        });
        return [
            wrap(constants_1.Commands.chatNew, constants_1.CursorTargets.chatNew),
            wrap(constants_1.Commands.chatCancel, constants_1.CursorTargets.chatCancel),
            wrap(constants_1.Commands.chatRename, constants_1.CursorTargets.chatRename),
            wrap(constants_1.Commands.chatFocus, constants_1.CursorTargets.chatFocus),
        ];
    },
};
//# sourceMappingURL=chat.js.map