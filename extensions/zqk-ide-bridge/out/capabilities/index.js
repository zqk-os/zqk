"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ALL_CAPABILITIES = void 0;
const mcp_1 = require("./mcp");
const chat_1 = require("./chat");
const modes_1 = require("./modes");
/** All known packs. Order is activation order. */
exports.ALL_CAPABILITIES = [
    mcp_1.mcpCapability,
    chat_1.chatCapability,
    modes_1.modesCapability,
];
//# sourceMappingURL=index.js.map