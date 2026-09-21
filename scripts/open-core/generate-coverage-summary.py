#!/usr/bin/env python3
"""
generate-coverage-summary.py
Parses a Go coverage profile (coverage.out) and generates a formatted Markdown
summary table suitable for GitHub Actions Job Summaries ($GITHUB_STEP_SUMMARY).
"""

import os
import sys
from collections import defaultdict


def parse_coverage(coverprofile_path):
    packages = defaultdict(lambda: [0, 0])  # pkg -> [total_stmts, covered_stmts]
    files = defaultdict(lambda: [0, 0])     # file -> [total_stmts, covered_stmts]
    total_stmts = 0
    covered_stmts = 0

    if not os.path.exists(coverprofile_path) or os.path.getsize(coverprofile_path) == 0:
        return None, None, 0, 0

    with open(coverprofile_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("mode:"):
                continue
            # format: path/file.go:start.col,end.col num_stmts count
            parts = line.rsplit(" ", 2)
            if len(parts) != 3:
                continue
            loc, stmts_str, count_str = parts
            try:
                stmts = int(stmts_str)
                count = int(count_str)
            except ValueError:
                continue

            file_path = loc.split(":")[0]
            # Strip the public module prefix from go cover lines.
            clean_path = file_path
            prefix = "github.com/zqk-os/zqk/"
            if clean_path.startswith(prefix):
                clean_path = clean_path[len(prefix):]

            pkg_name = os.path.dirname(clean_path) or clean_path

            packages[pkg_name][0] += stmts
            files[clean_path][0] += stmts
            total_stmts += stmts

            if count > 0:
                packages[pkg_name][1] += stmts
                files[clean_path][1] += stmts
                covered_stmts += stmts

    return packages, files, total_stmts, covered_stmts


def status_indicator(pct):
    if pct >= 80.0:
        return "🟢", "High"
    elif pct >= 50.0:
        return "🟡", "Medium"
    else:
        return "🔴", "Low"


def generate_markdown(title, packages, files, total_stmts, covered_stmts):
    if not packages or total_stmts == 0:
        return f"### {title}\n\n*No test coverage data recorded.*"

    total_pct = (covered_stmts / total_stmts * 100.0) if total_stmts else 0.0
    circle, label = status_indicator(total_pct)

    lines = []
    lines.append(f"### {title}")
    lines.append(f"**Total Coverage:** `{total_pct:.1f}%` ({covered_stmts}/{total_stmts} statements) &nbsp; {circle} {label}\n")
    lines.append("| Package | Statements | Covered | Coverage | Status | Level |")
    lines.append("| :--- | :---: | :---: | :---: | :---: | :---: |")

    for pkg, (tot, cov) in sorted(packages.items()):
        pct = (cov / tot * 100.0) if tot else 0.0
        c, l = status_indicator(pct)
        lines.append(f"| `{pkg}` | {tot} | {cov} | **{pct:.1f}%** | {c} | {l} |")

    # If there are multiple files, provide a collapsible file breakdown
    if len(files) > 1:
        lines.append("\n<details><summary><b>View detailed file breakdown</b></summary>\n")
        lines.append("| File | Statements | Covered | Coverage | Status | Level |")
        lines.append("| :--- | :---: | :---: | :---: | :---: | :---: |")
        for fpath, (tot, cov) in sorted(files.items()):
            pct = (cov / tot * 100.0) if tot else 0.0
            c, l = status_indicator(pct)
            lines.append(f"| `{fpath}` | {tot} | {cov} | {pct:.1f}% | {c} | {l} |")
        lines.append("\n</details>\n")

    return "\n".join(lines) + "\n"


def main():
    if len(sys.argv) < 2:
        print("Usage: generate-coverage-summary.py <coverage.out> [title]", file=sys.stderr)
        sys.exit(1)

    coverprofile = sys.argv[1]
    title = sys.argv[2] if len(sys.argv) > 2 else "Test Coverage Summary"

    packages, files, total_stmts, covered_stmts = parse_coverage(coverprofile)
    md = generate_markdown(title, packages, files, total_stmts, covered_stmts)

    # Print to stdout for log visibility
    print(md)

    # Write to GitHub Actions Step Summary if in runner environment
    summary_path = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary_path:
        with open(summary_path, "a", encoding="utf-8") as f:
            f.write(md + "\n")


if __name__ == "__main__":
    main()
