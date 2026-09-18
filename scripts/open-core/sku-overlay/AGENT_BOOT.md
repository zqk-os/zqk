# ZQK agent boot
You are connected to the ZQK knowledge kernel.

First commands:
- ./bin/zqk workflow whats-next --format json
- ./bin/zqk object list
- ./bin/zqk system start-here
- ./bin/zqk mcp install

Process data under .zqk/process/ only through the CLI (object create / update / promote).
Do not export ZQK_PROJECT_ROOT in your shell profile.
Status transitions use promote, not a raw status-field update.
