#!/usr/bin/env python3
"""
generate_manual_screenshots.py — Wrapper that invokes generate_authentic_svgs.go
to generate authentic, vector-sharp SVG screenshots directly from ZQK's core UI
rendering logic (`cmd/zqk/ui`, `cmd/zqk/test`, `pkg/studio`).
"""

import os
import subprocess
import sys

def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(script_dir, "..", ".."))
    go_script = os.path.join(script_dir, "generate_authentic_svgs.go")
    out_dir = os.path.join(repo_root, "docs", "manual", "screenshots")

    cmd = ["go", "run", go_script, out_dir]
    print(f"🚀 Invoking ZQK core UI rendering engine: {' '.join(cmd)}...")
    res = subprocess.run(cmd, cwd=repo_root)
    sys.exit(res.returncode)

if __name__ == "__main__":
    main()
