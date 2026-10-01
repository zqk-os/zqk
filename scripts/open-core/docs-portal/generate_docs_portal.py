#!/usr/bin/env python3
"""
generate_docs_portal.py — Fast static documentation site generator for ZQK Core.
Generates an offline-browsable, beautiful static documentation portal strictly
from the canonical public open-core documentation base.
"""

import os
import sys
import glob
import hashlib
import html
import json
import re
import shutil
import subprocess

try:
    import markdown
    HAS_MARKDOWN = True
except ImportError:
    HAS_MARKDOWN = False

# Import dynamic package readme index generator
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
try:
    from generate_code_readme_index import generate_readme_index
except ImportError:
    generate_readme_index = None

ASSETS_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "assets")

def _load_asset(filename: str, fallback: str = "") -> str:
    path = os.path.join(ASSETS_DIR, filename)
    if os.path.exists(path):
        with open(path, "r", encoding="utf-8") as f:
            return f.read()
    return fallback

ZQK_LOGO_SVG = _load_asset("logo.svg", fallback='<svg viewBox="0 0 100 100" width="100%" height="100%"><rect width="100" height="100" rx="22" fill="#05070a"/></svg>')
ZQK_HEADER_LOGO_SVG = ZQK_LOGO_SVG.replace('width="100%" height="100%"', 'width="28" height="28" class="logo-mark"')


def get_html_relpath(rel_md_path: str) -> str:
    """Converts a markdown file relative path into an HTML relative path preserving folders."""
    path = rel_md_path.replace("\\", "/")
    if path.startswith(".zqk/skills/"):
        path = path[len(".zqk/"):]
    elif path.startswith(".zqk/specs/"):
        path = "kernel-specs/" + path[len(".zqk/specs/"):]
    elif path.startswith(".zqk/agent_packs/"):
        path = "agent-packs/" + path[len(".zqk/agent_packs/"):]
    elif path.startswith(".agents/"):
        path = "agents/" + path[len(".agents/"):]
    elif path.startswith("./"):
        path = path[2:]

    if path.endswith(".md"):
        return path[:-3] + ".html"
    elif not path.endswith(".html"):
        return path + ".html"
    return path

def get_category_info(rel_path: str) -> tuple[str, str]:
    path = rel_path.replace("\\", "/")
    if path.startswith(".zqk/skills/") or path.startswith("skills/"):
        return "Agent Skills & Protocols", "skills"
    if path.startswith(".zqk/agent_packs/") or path.startswith("agent-packs/"):
        return "Agent Directives & Packs", "agent-directives"
    if path == ".agents/AGENTS.md" or path == "agents/AGENTS.md":
        return "Agent Directives & Packs", "agent-directives"
    if path.startswith("cmd/"):
        return "Kernel Subsystems — CLI Commands & Tooling", "subsystems-cli"
    if path.startswith("internal/"):
        return "Kernel Subsystems — Internal Runtime", "subsystems-internal"
    if path.startswith("pkg/"):
        if path.startswith("pkg/specbuilder/") or path.startswith("pkg/cli/"):
            return "Kernel Subsystems — Spec & Command Builders", "subsystems-builders"
        return "Kernel Subsystems — Core Engines", "subsystems-core"
    if path.startswith(".zqk/specs/") or (path.startswith("specs/") and any(k in path for k in ("objects", "traits", "lifecycles", "configs"))):
        return "Kernel DNA & Object Schemas", "schemas"
    if path == "scripts/onboarding_roadmap/README.md":
        return "Getting Started", "getting-started"
    if path == "scripts/scheduler_jobs/README.md":
        return "How-To & Incident Runbooks", "operations"

    parts = path.split("/")
    if parts[0] != "docs":
        if rel_path == "PACK-COMPOSITION.md":
            return "Architecture & Foundation", "architecture"
        if rel_path == "ZQK_GETTING_STARTED.md":
            return "Getting Started", "getting-started"
        return "Open Core Governance", "governance"

    if len(parts) == 2:
        return "Getting Started", "getting-started"

    sub = parts[1]
    if sub == "onboarding":
        return "Getting Started", "getting-started"
    elif sub == "architecture":
        return "Architecture & Foundation", "architecture"
    elif sub == "specs":
        return "Specifications & Grammars", "specs"
    elif sub == "manual":
        return "Reference Manuals", "manual"
    elif sub in ("howto", "runbooks"):
        return "How-To & Incident Runbooks", "operations"
    elif sub in ("tutorials", "demos", "guides"):
        return "Tutorials, Demos & Guides", "tutorials"
    elif sub in ("development", "explanation"):
        return "Maintenance & Development", "development"
    elif sub == "quality":
        if len(parts) > 2 and parts[2] == "codebase_evaluation":
            if len(parts) > 3:
                if parts[3] == "rubrics":
                    return "Codebase Evaluation — Evaluation Rubrics", "cef-rubrics"
                elif parts[3] == "prompts":
                    if "adversarial.md" in path:
                        return "Codebase Evaluation — Adversarial Prompts", "cef-adversarial"
                    return "Codebase Evaluation — Specialist Prompts", "cef-specialists"
            return "Codebase Evaluation — Framework & Governance", "cef-framework"
        return "Quality & Evaluation", "quality"
    elif sub == "eval":
        return "Codebase Evaluation — Audit Reports", "cef-reports"

    return sub.replace("_", " ").title(), sub

CEF_LENS_NAMES = {
    'L-ARCHITECTURE': 'Architecture & Abstractions',
    'L-CODE-QUALITY': 'Code Quality & Maintainability',
    'L-CONCURRENCY': 'Concurrency & Synchronization',
    'L-DOCS-MODEL': 'Documentation & Domain Model',
    'L-OBSERVABILITY': 'Observability & Diagnostics',
    'L-PERFORMANCE': 'Performance & Complexity',
    'L-PREFLIGHT': 'Preflight & Inventory',
    'L-RELIABILITY': 'Reliability & Error Recovery',
    'L-SECURITY': 'Security & Threat Modeling',
    'L-SUPPLY-RELEASE': 'Supply Chain & Packaging',
    'L-TESTING': 'Test Strategy & Invariant Proofs',
    'L-USABILITY': 'Developer Usability & Ergonomics'
}

def get_cef_doc_title(rel_path: str, fallback_title: str) -> str:
    path = rel_path.replace("\\", "/")
    base = os.path.basename(path)
    if "docs/quality/codebase_evaluation/" in path:
        parts = path.split("/")
        if "/rubrics/" in path:
            lens = os.path.splitext(base)[0]
            name = CEF_LENS_NAMES.get(lens, lens)
            return f"{name} Rubric ({lens})"
        elif "/prompts/" in path:
            if base == "_SHARED_PREAMBLE.md":
                return "CEF Shared Agent Preamble"
            lens = parts[-2]
            if lens == "L-INTEGRATOR":
                return "Lead Integrator & Synthesis Specialist Prompt"
            name = CEF_LENS_NAMES.get(lens, lens)
            if base == "adversarial.md":
                return f"{name} Adversarial Auditor Prompt ({lens})"
            elif base == "specialist.md":
                return f"{name} Specialist Evaluator Prompt ({lens})"
        else:
            titles = {
                "README.md": "Codebase Evaluation Framework (CEF) Overview",
                "CONSTITUTION.md": "CEF Constitution (Binding Rules & Evidence Grades)",
                "DIAMOND_SCALE.md": "Diamond Scale Multi-Axis Quality Grading",
                "WAVE_PLAN.md": "Multi-Wave Analysis Execution Plan",
                "LENSES.md": "Evaluation Lens Catalog & Density Rules",
                "DIAGRAM_CONTRACT.md": "Diagram Contract & Architecture Anchoring",
                "OPERATOR.md": "Operator Quickstart Runbook",
                "KICKOFF_PROMPT.md": "Solo Operator Kickoff Prompt",
                "HANDOFF_SCHEMA.md": "Handoff Schema & Downstream Integration",
                "EXTENSIONS.md": "Framework Extensions & Adapter Architecture",
                "go.md": "Go Language Adapter Specification"
            }
            if base in titles:
                return titles[base]
    return fallback_title

def get_clean_nav_title(title: str, rel_path: str = "") -> str:
    """Returns a concise, scannable title for sidebar navigation."""
    path = rel_path.replace("\\", "/")
    if "docs/quality/codebase_evaluation/" in path:
        parts = path.split("/")
        base = os.path.basename(path)
        if "/rubrics/" in path:
            lens = os.path.splitext(base)[0]
            name = CEF_LENS_NAMES.get(lens, lens)
            return f"{lens} — {name}"
        elif "/prompts/" in path:
            if base == "_SHARED_PREAMBLE.md":
                return "Shared Agent Preamble"
            lens = parts[-2]
            if lens == "L-INTEGRATOR":
                return "L-INTEGRATOR — Lead Integrator"
            name = CEF_LENS_NAMES.get(lens, lens)
            if base == "adversarial.md":
                return f"{lens} — {name} (Adversarial)"
            elif base == "specialist.md":
                return f"{lens} — {name} (Specialist)"
        else:
            nav_titles = {
                "README.md": "Overview & Quickstart",
                "CONSTITUTION.md": "Constitution (Binding Rules)",
                "DIAMOND_SCALE.md": "Diamond Scale Grading",
                "WAVE_PLAN.md": "Multi-Wave Plan",
                "LENSES.md": "Evaluation Lens Catalog",
                "DIAGRAM_CONTRACT.md": "Diagram Contract",
                "OPERATOR.md": "Operator Runbook",
                "KICKOFF_PROMPT.md": "Solo Kickoff Prompt",
                "HANDOFF_SCHEMA.md": "Handoff Schema",
                "EXTENSIONS.md": "Extensions & Adapters",
                "go.md": "Go Language Adapter"
            }
            if base in nav_titles:
                return nav_titles[base]

    if path.startswith("docs/guides/"):
        guide_nav_titles = {
            "README.md": "Developer & Agent Guides Index",
            "KERNEL_OBJECT_USAGE_GUIDE.md": "Kernel Object Model & Usage Guide",
            "KNOWLEDGE_MANAGEMENT_AND_SEMANTIC_RECALL_GUIDE.md": "Knowledge Management & Semantic Recall",
            "ZQL_ZPARQL_AGENT_GUIDE.md": "ZQL & ZPARQL Graph Operations",
            "OBJECT_LIFECYCLE_AND_CAS_STORAGE_GUIDE.md": "Object Lifecycle, States & CAS",
            "POLICY_CREATION_AND_VALIDATION_DSL_GUIDE.md": "Custom Policy & Rule DSL",
            "SINGLE_COMMAND_EXECUTION_LOOP_GUIDE.md": "Single-Command Loop (zqk do)",
            "AGENT_ORCHESTRATION_AND_SWARM_COLLABORATION_GUIDE.md": "Swarm Orchestration & Mesh",
            "DIAGNOSTICS_SELF_HEALING_AND_REMEDY_GUIDE.md": "Diagnostics & Self-Healing"
        }
        base = os.path.basename(path)
        if base in guide_nav_titles:
            return guide_nav_titles[base]

    subsystem_nav_titles = {
        "pkg/README.md": "Packages Directory (pkg/)",
        "pkg/storage/README.md": "Storage Subsystem & Providers",
        "pkg/storage/DATACELL_MIGRATION.md": "Storage — DataCell Migration Plan",
        "pkg/storage/change_journal_compaction.md": "Storage — Journal Compaction",
        "pkg/graph/README.md": "Graph Backend & In-Memory Graph",
        "pkg/coordination/README.md": "Event Coordination (Spinal Cord)",
        "pkg/concurrency/README.md": "Concurrency & Synchronization",
        "pkg/pipeline/README.md": "Autonomous Pipelines & Steps",
        "pkg/mcp/README.md": "Model Context Protocol (MCP) Server",
        "pkg/mcp/testing/README.md": "MCP Test Verification Harness",
        "pkg/functional/README.md": "Functional Error Monad (Result[T])",
        "pkg/logging/README.md": "Structured Zero-Allocation Logger",
        "pkg/telemetry/README.md": "Kernel Telemetry & Metrics",
        "pkg/validation/README.md": "Schema & Rule Validation Engine",
        "pkg/translation/README.md": "Query & AST Translation Engine",
        "pkg/loader/README.md": "Object & Specification Loader",
        "pkg/migration/README.md": "Database & Schema Migration",
        "pkg/migration/detector/README.md": "Migration Binary Detector",
        "pkg/goroutinelabels/README.md": "Goroutine Thread Tracking",
        "pkg/domain/organizational/README.md": "Organizational Domain Models",
        "pkg/testing/README.md": "Kernel Testing Harness",
        # Spec & Command Builders
        "pkg/specbuilder/README.md": "Spec-Driven Builder Pattern",
        "pkg/cli/README.md": "CLI Architecture & Spec Framework",
        "pkg/cli/bldr_cli_cmd_v1/README.md": "CLI Command Builders (bldr_cli_cmd_v1)",
        "pkg/specbuilder/bldr_v2/README.md": "Object Spec Builders (bldr_v2)",
        "pkg/specbuilder/builders/README.md": "Versioned Spec Builders Engine",
        "pkg/specbuilder/instance_builders/README.md": "Runtime Instance Builders",
        "pkg/specbuilder/bootstrap/README.md": "Specbuilder Bootstrap Package",
        # CLI Commands & Tooling
        "cmd/zqk/README.md": "zqk Primary CLI Commands",
        "cmd/zqk-shim/README.md": "zqk-shim Compatibility Shim",
        "cmd/zqk/system/METRICS_ANALYSIS_README.md": "System Lock Metrics Analysis",
        "cmd/zqk/system/TESTING.md": "System Test Suite & Diagnostics",
        "cmd/zqk/system/INTEGRITY_RESOLUTION_PLAN.md": "System Integrity Resolution",
        # Internal Runtime
        "internal/README.md": "Internal Packages Directory (internal/)",
        "internal/bootstrap/README.md": "Kernel Bootstrap Seeding",
        "internal/cli/README.md": "Internal CLI Framework",
        "internal/cli/context/README.md": "Execution Context & Signals",
    }
    if path in subsystem_nav_titles:
        return subsystem_nav_titles[path]

    prefixes = [
        "Technical Specification: ",
        "Technical Specification — ",
        "Manual: ",
        "Tutorial: ",
        "Guide: ",
        "Agent & Developer Guide: ",
        "Rubric: ",
        "Adversarial prompt — ",
        "Specialist prompt — ",
        "First-run object tutorial (template → create → get → update)",
    ]
    if title == "First-run object tutorial (template → create → get → update)":
        return "First-Run Object Tutorial"
    cleaned = title
    for p in prefixes:
        if cleaned.startswith(p):
            cleaned = cleaned[len(p):].strip()
            break
    return cleaned


_asset_hash_cache = {}

def get_asset_hash(file_path: str) -> str:
    """Return an 8-character sha256 hash of the file contents for cache busting."""
    if not file_path:
        return ""
    if file_path in _asset_hash_cache:
        return _asset_hash_cache[file_path]
    try:
        if os.path.isfile(file_path):
            with open(file_path, "rb") as f:
                h = hashlib.sha256(f.read()).hexdigest()[:8]
                _asset_hash_cache[file_path] = h
                return h
    except Exception:
        pass
    return ""

def resolve_asset_path(target: str, current_dir: str, repo_root: str) -> str:
    """Resolve an asset file path to a physical file on disk."""
    if not repo_root or not target:
        return ""
    clean_target = target.split("?")[0].split("#")[0]
    resolved_repo_rel = os.path.normpath(os.path.join(current_dir, clean_target)) if current_dir else os.path.normpath(clean_target)
    clean_res = resolved_repo_rel.replace("\\", "/")
    
    candidates = [
        os.path.join(repo_root, clean_res),
        os.path.join(repo_root, "docs", clean_res),
        os.path.join(repo_root, clean_target.lstrip("/")),
        os.path.join(repo_root, "docs", clean_target.lstrip("/")),
    ]
    for cand in candidates:
        if os.path.isfile(cand):
            return cand
    return ""

def bust_html_img_cache(html_content: str, current_html_rel: str, repo_root: str, default_hash: str = "") -> str:
    """Ensure any raw <img> tags or un-busted image references have content-hash cache-busting."""
    current_dir = os.path.dirname(current_html_rel)
    img_exts = (".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp", ".avif")

    def _replace_img(match):
        pre = match.group(1)
        src = match.group(2)
        post = match.group(3)
        if src.startswith("http://") or src.startswith("https://") or src.startswith("//") or src.startswith("data:"):
            return match.group(0)
        if "?v=" in src or "&v=" in src:
            return match.group(0)
        src_path_no_query = src.split("?")[0].split("#")[0]
        if not any(src_path_no_query.lower().endswith(ext) for ext in img_exts):
            return match.group(0)
        matched = resolve_asset_path(src_path_no_query, current_dir, repo_root)
        h = get_asset_hash(matched) if matched else default_hash
        if not h:
            return match.group(0)
        sep = "&" if "?" in src else "?v="
        if "#" in src:
            base, anch = src.split("#", 1)
            new_src = f"{base}{sep}{h}#{anch}"
        else:
            new_src = f"{src}{sep}{h}"
        return f"{pre}{new_src}{post}"

    return re.sub(r'(<img\b[^>]*?\bsrc=["\'])([^"\']+?)(["\'])', _replace_img, html_content)

def get_git_commit_sha(repo_root: str) -> str:
    try:
        out = subprocess.check_output(["git", "rev-parse", "--short", "HEAD"], cwd=repo_root, text=True, stderr=subprocess.DEVNULL).strip()
        if out:
            return out
    except Exception:
        pass
    return "zqk"

def is_likely_math(expr: str) -> bool:
    s = expr.strip()
    if not s:
        return False
    # If it contains LaTeX commands
    if re.search(r'\\[a-zA-Z]+', s):
        return True
    # If it contains math relations, superscripts, subscripts, or LaTeX braces
    if re.search(r'[\^_{}\\]', s):
        return True
    if re.search(r'[=<>]\s*[\d\w]', s) or re.search(r'[\d\w]\s*[=<>]', s):
        return True
    if re.search(r'\|[A-Za-z0-9]+\|', s):
        return True
    if re.search(r'O\([^)]+\)', s):
        return True
    # Single mathematical letters or sets (e.g., $Q$, $V$, $S$, $C$)
    if re.match(r'^[A-Z](_[a-z0-9]+)?$', s):
        return True
    return False

def render_markdown_to_html(content: str, current_html_rel: str, link_map: dict, repo_root: str = "", default_hash: str = "") -> tuple:
    current_dir = os.path.dirname(current_html_rel)

    # Rewrite markdown links to generated html paths
    def replace_md_link(match):
        prefix = match.group(1)
        target = match.group(2)
        
        # Don't touch external or anchor-only links
        if target.startswith("http://") or target.startswith("https://") or target.startswith("mailto:") or target.startswith("javascript:"):
            return match.group(0)
        if target.startswith("#"):
            return match.group(0)

        anchor = ""
        if "#" in target:
            target, anchor = target.split("#", 1)
            anchor = "#" + anchor

        target_norm = os.path.normpath(target)
        target_base = os.path.basename(target)

        # Resolve target relative to current_dir to get repo-relative path
        resolved_repo_rel = os.path.normpath(os.path.join(current_dir, target))

        target_html_rel = None
        if resolved_repo_rel in link_map:
            target_html_rel = link_map[resolved_repo_rel]
        elif target in link_map:
            target_html_rel = link_map[target]
        elif target_norm in link_map:
            target_html_rel = link_map[target_norm]
        elif target_base in link_map:
            target_html_rel = link_map[target_base]

        if target_html_rel:
            if current_dir:
                rel_url = os.path.relpath(target_html_rel, current_dir)
            else:
                rel_url = target_html_rel
            return f"{prefix}({rel_url}{anchor})"

        # Check if target is an image asset or if this is an image link (![...])
        img_exts = (".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp", ".avif")
        is_img_target = any(target.lower().endswith(ext) or (ext + "#") in target.lower() for ext in img_exts)
        is_img_link = prefix.startswith("!")

        if is_img_target or is_img_link:
            matched_file = resolve_asset_path(target, current_dir, repo_root)
            asset_hash = get_asset_hash(matched_file) if matched_file else default_hash
            cache_bust = f"?v={asset_hash}" if asset_hash else ""

            clean_res = resolved_repo_rel.replace("\\", "/")
            if repo_root and os.path.exists(os.path.join(repo_root, clean_res)):
                if current_dir:
                    rel_url = os.path.relpath(clean_res, current_dir).replace("\\", "/")
                else:
                    rel_url = clean_res
                return f"{prefix}({rel_url}{cache_bust}{anchor})"
            elif matched_file and repo_root:
                rel_from = os.path.join(repo_root, current_dir) if current_dir else repo_root
                rel_url = os.path.relpath(matched_file, rel_from).replace("\\", "/")
                return f"{prefix}({rel_url}{cache_bust}{anchor})"
            return f"{prefix}({target}{cache_bust}{anchor})"

        # Check if target is a source code or config file in the repository (e.g. .go, .yaml, .json, .sh, .md)
        clean_res = resolved_repo_rel.replace("\\", "/")
        if not clean_res.startswith(".."):
            if repo_root and os.path.exists(os.path.join(repo_root, clean_res)):
                gh_url = f"https://github.com/zqk-os/zqk/blob/main/{clean_res}{anchor}"
                return f"{prefix}({gh_url})"
            code_exts = (".go", ".yaml", ".yml", ".json", ".sh", ".proto", ".sql", ".toml", ".mod", ".sum", ".md")
            if target.endswith(code_exts) or any(target.endswith(ext + anchor) for ext in code_exts):
                gh_url = f"https://github.com/zqk-os/zqk/blob/main/{clean_res}{anchor}"
                return f"{prefix}({gh_url})"

        return match.group(0)

    transformed = re.sub(r'(!?\[[^\]]*\])\(([^)]+)\)', replace_md_link, content)
    
    has_math = False
    if HAS_MARKDOWN:
        try:
            # 1. Mask fenced code blocks and inline code to avoid false math matching
            code_tokens = []
            def _save_code(m):
                token = f"ZZZCODETOKEN{len(code_tokens)}ZZZ"
                code_tokens.append(m.group(0))
                return token

            text_masked = re.sub(r'```[\s\S]*?```', _save_code, transformed)
            text_masked = re.sub(r'`[^`\n]+`', _save_code, text_masked)

            # 2. Protect display and inline math from markdown emphasis/italics mangling
            math_tokens = []
            def _save_display_math(m):
                nonlocal has_math
                has_math = True
                token = f"ZZZMATHTOKEN{len(math_tokens)}ZZZ"
                inner = m.group(1).strip()
                math_tokens.append(f'<div class="katex-display">\\[{inner}\\]</div>')
                return token

            def _save_inline_math(m):
                nonlocal has_math
                inner = m.group(1).strip()
                if is_likely_math(inner):
                    has_math = True
                    token = f"ZZZMATHTOKEN{len(math_tokens)}ZZZ"
                    math_tokens.append(f"\\({inner}\\)")
                    return token
                return m.group(0)

            # Display math ($$...$$)
            text_masked = re.sub(r'\$\$([\s\S]*?)\$\$', _save_display_math, text_masked)
            # Inline math ($...$) - ensuring non-empty and non-whitespace bounded
            text_masked = re.sub(r'(?<!\$)\$(?!\$)([^\s$](?:[^\n$]*?[^\s$])?)(?<!\$)\$(?!\$)', _save_inline_math, text_masked)

            # 3. Ensure list items preceded by a paragraph without a blank line are separated
            def separate_unseparated_lists(text: str) -> str:
                lines = text.split('\n')
                new_lines = []
                list_marker_re = re.compile(r'^[ \t]*(?:\d+\.|\*|\-|\+)\s+')
                for i, line in enumerate(lines):
                    if i > 0 and list_marker_re.match(line):
                        prev = lines[i-1]
                        prev_stripped = prev.strip()
                        if (prev_stripped and 
                            not list_marker_re.match(prev) and 
                            not prev_stripped.startswith('#') and 
                            not prev_stripped.startswith('>') and 
                            not prev_stripped.startswith('|') and 
                            not prev_stripped.startswith('---') and
                            not prev_stripped.startswith('```')):
                            new_lines.append('')
                    new_lines.append(line)
                return '\n'.join(new_lines)

            text_masked = separate_unseparated_lists(text_masked)

            # 4. Restore code blocks before markdown processing so fenced blocks are properly converted
            for idx, code_str in enumerate(code_tokens):
                text_masked = text_masked.replace(f"ZZZCODETOKEN{idx}ZZZ", code_str)

            rendered = markdown.markdown(
                text_masked,
                extensions=['fenced_code', 'tables', 'toc', 'sane_lists']
            )

            # 5. Restore math blocks and sanitize any toc heading IDs
            for idx, math_str in enumerate(math_tokens):
                slug = 'math'
                rendered = re.sub(rf'(id="[^"]*?)zzzmathtoken{idx}zzz([^"]*?")', rf'\1{slug}\2', rendered)
                rendered = rendered.replace(f"ZZZMATHTOKEN{idx}ZZZ", math_str)

            # 6. Wrap all markdown tables in responsive containers
            def _wrap_table(m):
                return f'<div class="table-container">{m.group(0)}</div>'
            rendered = re.sub(r'<table>[\s\S]*?</table>', _wrap_table, rendered)

            # Transform fenced mermaid code blocks to <pre class="mermaid">
            # markdown fenced_code generates: <pre><code class="language-mermaid">...</code></pre>
            def _mermaid_replacer(match):
                code_text = match.group(1)
                raw_code = html.unescape(code_text)

                # Auto-quote link labels containing special characters if unquoted:
                # e.g., -->|apply()| => -->|"apply()"|
                def _quote_link_label(m):
                    arrow = m.group(1)
                    label = m.group(2).strip()
                    if (label.startswith('"') and label.endswith('"')) or (label.startswith("'") and label.endswith("'")):
                        return f"{arrow}|{label}|"
                    if any(c in label for c in '()[]{}') or '->' in label:
                        safe = label.replace('"', '\\"')
                        return f'{arrow}|"{safe}"|'
                    return f"{arrow}|{label}|"

                raw_code = re.sub(r'(-->|--|-\.->|==>)\|([^|\n]+)\|', _quote_link_label, raw_code)
                return f'<pre class="mermaid">{html.escape(raw_code, quote=False)}</pre>'

            rendered = re.sub(
                r'<pre><code class="(?:language-)?mermaid">([\s\S]*?)</code></pre>',
                _mermaid_replacer,
                rendered
            )

            # Transform GitHub-style blockquote alerts: > [!IMPORTANT], > [!NOTE], etc.
            def _alert_replacer(match):
                alert_type = match.group(1).lower()
                alert_title = match.group(1).capitalize()
                body = match.group(2).strip()
                # If body ends with </p>, ensure it remains clean
                if body.endswith("</p>"):
                    body = body[:-4].strip()
                if body.startswith("<p>"):
                    body = body[3:].strip()
                icon_svg = {
                    "note": '<svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor"><path d="M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm8-6.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM6.5 7.75A.75.75 0 0 1 7.25 7h1a.75.75 0 0 1 .75.75v2.75h.25a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1 0-1.5h.25v-2h-.25a.75.75 0 0 1-.75-.75ZM8 6a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"></path></svg>',
                    "tip": '<svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor"><path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.21c-.04-.29-.19-.594-.412-.916a4.701 4.701 0 0 0-.58-.729l-.217-.256C3.12 7.64 2.5 6.74 2.5 5.25 2.5 2.31 4.75 0 8 0s5.5 2.31 5.5 5.25c0 1.49-.62 2.39-1.278 3.164l-.217.256c-.18.213-.377.46-.58.729-.221.322-.372.625-.412.916a.75.75 0 0 1-1.484-.21c.084-.594.337-1.079.621-1.49.204-.292.45-.584.673-.848l.214-.253c.56-.679.984-1.32.984-2.304 0-2.06-1.637-3.75-4-3.75ZM6 11.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Zm.75 2.25a.75.75 0 0 0 0 1.5h2.5a.75.75 0 0 0 0-1.5h-2.5Z"></path></svg>',
                    "important": '<svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor"><path d="M0 1.75C0 .784.784 0 1.75 0h12.5C15.216 0 16 .784 16 1.75v9.5A1.75 1.75 0 0 1 14.25 13H9.06l-2.573 2.573A1.458 1.458 0 0 1 4 14.543V13H1.75A1.75 1.75 0 0 1 0 11.25Zm1.75-.25a.25.25 0 0 0-.25.25v9.5c0 .138.112.25.25.25h2.5a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h6a.25.25 0 0 0 .25-.25v-9.5a.25.25 0 0 0-.25-.25Zm6.25 2.75a.75.75 0 0 1 .75.75v3.5a.75.75 0 0 1-1.5 0v-3.5a.75.75 0 0 1 .75-.75Zm0 7a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"></path></svg>',
                    "warning": '<svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor"><path d="M6.457 1.047c.659-1.234 2.427-1.234 3.086 0l6.082 11.378A1.75 1.75 0 0 1 14.082 15H1.918a1.75 1.75 0 0 1-1.543-2.575Zm1.763.707a.25.25 0 0 0-.44 0L1.698 13.132a.25.25 0 0 0 .22.368h12.164a.25.25 0 0 0 .22-.368Zm.53 3.996v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 11a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"></path></svg>',
                    "caution": '<svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor"><path d="M4.47.047A.75.75 0 0 1 5 0h6a.75.75 0 0 1 .53.22l4.25 4.25c.141.14.22.331.22.53v6a.75.75 0 0 1-.22.53l-4.25 4.25A.75.75 0 0 1 11 16H5a.75.75 0 0 1-.53-.22L.22 11.53A.75.75 0 0 1 0 11V5a.75.75 0 0 1 .22-.53L4.47.047Zm.53 1.28L1.5 4.82v6.36l3.5 3.5h6l3.5-3.5V4.82L11.5 1.33H5ZM8 4a.75.75 0 0 1 .75.75v3.5a.75.75 0 0 1-1.5 0v-3.5A.75.75 0 0 1 8 4Zm0 7.5a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"></path></svg>'
                }.get(alert_type, '')
                return f'<div class="alert alert-{alert_type}"><div class="alert-title">{icon_svg}<span>{alert_title}</span></div><div class="alert-body"><p>{body}</p></div></div>'

            rendered = re.sub(
                r'<blockquote>\s*<p>\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\](?:\s*<br\s*/?>|\s*</p>\s*<p>|\s*\n)?([\s\S]*?)</blockquote>',
                _alert_replacer,
                rendered
            )
            rendered = bust_html_img_cache(rendered, current_html_rel, repo_root, default_hash)
            return rendered, has_math
        except Exception:
            pass
            
    preview = f"<pre class=\"markdown-preview\">{html.escape(content)}</pre>"
    return bust_html_img_cache(preview, current_html_rel, repo_root, default_hash), False

def get_portal_style_css() -> str:
    return _load_asset("portal.css", fallback=":root { --bg-primary: #05070c; }")

def build_portal(repo_root: str, target_dir: str):
    print(f"📚 Generating ZQK Core Documentation Portal into {target_dir}...")
    
    if os.path.exists(target_dir):
        shutil.rmtree(target_dir)

    # Dynamically regenerate package indexes to ensure package catalog never goes stale
    if generate_readme_index:
        try:
            print("📦 Dynamically generating comprehensive package indexes (pkg/ and internal/)...")
            generate_readme_index(repo_root, "pkg")
            generate_readme_index(repo_root, "internal")
        except Exception as e:
            print(f"Warning: could not dynamically update package catalogs: {e}", file=sys.stderr)
    
    assets_dir = os.path.join(target_dir, "assets")
    search_dir = os.path.join(target_dir, "search")
    os.makedirs(assets_dir, exist_ok=True)
    os.makedirs(search_dir, exist_ok=True)

    portal_build_id = get_git_commit_sha(repo_root)
    logo_hash = hashlib.sha256(ZQK_LOGO_SVG.encode("utf-8")).hexdigest()[:8]
    style_css = get_portal_style_css()
    style_hash = hashlib.sha256(style_css.encode("utf-8")).hexdigest()[:8]

    # Write authentic ZQK logo asset
    with open(os.path.join(assets_dir, "zqk-logo.svg"), "w", encoding="utf-8") as f:
        f.write(ZQK_LOGO_SVG.strip() + "\n")

    # Collect documentation files strictly from Core, excluding archive/internal dirs
    raw_doc_files = sorted(glob.glob(os.path.join(repo_root, "docs", "**", "*.md"), recursive=True))
    doc_files = []
    exclude_parts = {"archive", "_archive", "_archive-cef-runs", "cef-runs", ".zqk", "audit", "templates", "package_skeleton", "testdata"}
    for df in raw_doc_files:
        rel = os.path.relpath(df, repo_root)
        parts = set(rel.split(os.sep))
        if parts.intersection(exclude_parts):
            continue
        doc_files.append(df)

    for root_doc in ["README.md", "CONTRIBUTING.md", "SECURITY.md", "GOVERNANCE.md", "CODE_OF_CONDUCT.md", "PACK-COMPOSITION.md", "ZQK_GETTING_STARTED.md"]:
        p = os.path.join(repo_root, root_doc)
        if os.path.isfile(p):
            doc_files.append(p)

    agents_file = os.path.join(repo_root, ".agents", "AGENTS.md")
    if os.path.isfile(agents_file):
        doc_files.append(agents_file)

    raw_skill_files = sorted(glob.glob(os.path.join(repo_root, ".zqk", "skills", "**", "*.md"), recursive=True))
    for sf in raw_skill_files:
        doc_files.append(sf)

    # Subsystem and Go package documentation (pkg/, internal/, cmd/)
    for sub_dir in ["pkg", "internal", "cmd"]:
        sub_path = os.path.join(repo_root, sub_dir)
        if not os.path.isdir(sub_path):
            continue
        for root, dirs, files in os.walk(sub_path):
            dirs[:] = [d for d in dirs if d not in exclude_parts and not d.startswith(".")]
            for f in files:
                if f.endswith(".md") and not f.startswith("REFACTORING_PLAN_"):
                    doc_files.append(os.path.join(root, f))

    # Kernel DNA specs (.zqk/specs/)
    spec_mds = sorted(glob.glob(os.path.join(repo_root, ".zqk", "specs", "**", "*.md"), recursive=True))
    for sm in spec_mds:
        doc_files.append(sm)

    # Agent Packs (.zqk/agent_packs/)
    agent_pack_mds = sorted(glob.glob(os.path.join(repo_root, ".zqk", "agent_packs", "**", "*.md"), recursive=True))
    for ap in agent_pack_mds:
        doc_files.append(ap)

    # Operational Guides in scripts/
    for script_doc in ["scripts/onboarding_roadmap/README.md", "scripts/scheduler_jobs/README.md"]:
        sp = os.path.join(repo_root, script_doc)
        if os.path.isfile(sp):
            doc_files.append(sp)

    # Scrutinizer filter: reject empty stubs, placeholder text, or template scaffolds
    FORBIDDEN_DOC_PATTERNS = [
        re.compile(r'\btest content\b', re.IGNORECASE),
        re.compile(r'\bTODO_OVERWRITE\b'),
        re.compile(r'\bREPLACE_ME\b'),
        re.compile(r'\blorem ipsum\b', re.IGNORECASE),
    ]

    def is_substantive_doc(file_path: str) -> bool:
        try:
            sz = os.path.getsize(file_path)
            if sz < 60:
                print(f"⚠️ [DOC-SCRUTINIZER] Dropping stub file (< 60b): {file_path}")
                return False
            # Only apply placeholder token rejection to template scaffolds
            if "templates" in file_path or "skeleton" in file_path:
                with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
                    content = f.read()
                for pat in FORBIDDEN_DOC_PATTERNS:
                    if pat.search(content):
                        print(f"⚠️ [DOC-SCRUTINIZER] Dropping template scaffold with placeholder '{pat.pattern}': {file_path}")
                        return False
            return True
        except Exception:
            return False

    # Deduplicate while preserving order and applying quality scrutiny
    doc_files = [df for df in sorted(list(dict.fromkeys(doc_files))) if is_substantive_doc(df)]

    # Collect and mirror all static/non-markdown files in docs/ (YAML, JSON, images, etc.)
    for root, dirs, files in os.walk(os.path.join(repo_root, "docs")):
        dirs[:] = [d for d in dirs if d not in exclude_parts and not d.startswith(".")]
        for f in files:
            if f.endswith(".md") or f == "CNAME":
                continue
            src_path = os.path.join(root, f)
            rel_path = os.path.relpath(src_path, repo_root)
            dest_path = os.path.join(target_dir, rel_path)
            os.makedirs(os.path.dirname(dest_path), exist_ok=True)
            shutil.copy2(src_path, dest_path)

    # Also copy root project files like LICENSE and NOTICE
    for rf in ["LICENSE", "NOTICE"]:
        rp = os.path.join(repo_root, rf)
        if os.path.isfile(rp):
            shutil.copy2(rp, os.path.join(target_dir, rf))

    link_map = {}
    doc_entries = []

    # First pass: Build link map, extract titles, and copy raw markdown files
    for doc in doc_files:
        rel_path = os.path.relpath(doc, repo_root)
        html_rel = get_html_relpath(rel_path)
        base_name = os.path.basename(rel_path)
        clean_md_rel = html_rel[:-5] + ".md" if html_rel.endswith(".html") else html_rel + ".md"

        link_map[rel_path] = html_rel
        link_map["./" + rel_path] = html_rel
        link_map[clean_md_rel] = html_rel
        link_map["./" + clean_md_rel] = html_rel
        if base_name not in ("README.md", "INDEX.md", "SKILL.md") and base_name not in link_map:
            link_map[base_name] = html_rel
            link_map["./" + base_name] = html_rel

        try:
            with open(doc, "r", encoding="utf-8", errors="replace") as f:
                content = f.read()
        except Exception as e:
            print(f"Warning: could not read {doc}: {e}", file=sys.stderr)
            continue

        # Mirror the raw .md file into target_dir so raw markdown is accessible
        target_md_path = os.path.join(target_dir, rel_path)
        os.makedirs(os.path.dirname(target_md_path), exist_ok=True)
        with open(target_md_path, "w", encoding="utf-8") as mf:
            mf.write(content)

        # Also write clean non-dot path for web accessibility
        clean_md_path = os.path.join(target_dir, clean_md_rel)
        if clean_md_path != target_md_path:
            os.makedirs(os.path.dirname(clean_md_path), exist_ok=True)
            with open(clean_md_path, "w", encoding="utf-8") as mf:
                mf.write(content)

        clean_content = content
        frontmatter = {}
        if clean_content.startswith("---"):
            fm_end = clean_content.find("\n---", 3)
            if fm_end != -1:
                fm_raw = clean_content[3:fm_end].strip()
                clean_content = clean_content[fm_end + 4:].strip()
                try:
                    import yaml
                    frontmatter = yaml.safe_load(fm_raw) or {}
                except Exception:
                    pass

        lines = [line.strip() for line in clean_content.splitlines() if line.strip()]
        title = ""
        for line in lines[:25]:
            if line.startswith("# "):
                title = line[2:].strip()
                break
        if rel_path.startswith(".zqk/agent_packs/"):
            pack_name = rel_path.split("/")[2]
            p_display = pack_name.upper() if pack_name in ("ide", "mcp") else pack_name.replace("_", " ").title()
            title = f"{p_display} Agent Boot Protocol"
        elif rel_path == "scripts/onboarding_roadmap/README.md":
            title = "Curriculum as Data: Onboarding Roadmap"
        elif rel_path == "scripts/scheduler_jobs/README.md":
            title = "Scheduler Job Templates"
        elif rel_path.startswith("docs/quality/codebase_evaluation/"):
            title = get_cef_doc_title(rel_path, title)
        elif not title:
            fm_name_match = re.search(r'^name:\s*(.+)$', content, re.MULTILINE)
            if fm_name_match:
                title = fm_name_match.group(1).strip().replace("-", " ").title()
            else:
                title = os.path.splitext(base_name)[0].replace("_", " ").replace("-", " ").title()

        if frontmatter and isinstance(frontmatter, dict) and "name" in frontmatter:
            skill_name = html.escape(str(frontmatter.get("name", "")))
            skill_desc = html.escape(str(frontmatter.get("description", "")))
            seal_pill = ""
            if "seal_hash" in frontmatter:
                seal_ver = html.escape(str(frontmatter.get("seal_version", "1.0.0")))
                seal_hash_short = html.escape(str(frontmatter.get("seal_hash", ""))[:12])
                seal_pill = f'<span class="skill-seal-badge" title="Cryptographically sealed skill protocol">🔒 Sealed v{seal_ver} ({seal_hash_short}…)</span>'
            
            banner_md = f"""
<div class="skill-spec-card">
  <div class="skill-spec-header">
    <span class="skill-spec-tag">Agent Skill Protocol</span>
    <code class="skill-spec-name">{skill_name}</code>
    {seal_pill}
  </div>
  <p class="skill-spec-desc">{skill_desc}</p>
</div>
"""
            h1_match = re.search(r'^(#\s+[^\n]+)', clean_content, re.MULTILINE)
            if h1_match:
                end_pos = h1_match.end()
                clean_content = clean_content[:end_pos] + "\n\n" + banner_md + "\n" + clean_content[end_pos:]
            else:
                clean_content = banner_md + "\n\n" + clean_content

        snippet = " ".join(" ".join(lines[:15]).split())[:200]
        cat_name, cat_key = get_category_info(rel_path)

        doc_entries.append({
            "doc": doc,
            "rel_path": rel_path,
            "clean_md_rel": clean_md_rel,
            "html_rel": html_rel,
            "title": title,
            "content": clean_content,
            "snippet": snippet,
            "category": cat_name,
            "category_key": cat_key
        })

    # Canonical README.md for the docs repository root
    portal_readme_content = """# ZQK Core Documentation Portal

Official documentation portal for [ZQK Core](https://github.com/zqk-os/zqk), deployed at **[docs.zqk.dev](https://docs.zqk.dev/)**.

## Structure
- Canonical web documentation: `*.html`
- Source Markdown mirrors: `docs/**/*.md`
- Deployed via GitHub Pages.
"""
    with open(os.path.join(target_dir, "README.md"), "w", encoding="utf-8") as f:
        f.write(portal_readme_content)

    # CNAME and .nojekyll for GitHub Pages deployment
    with open(os.path.join(target_dir, "CNAME"), "w", encoding="utf-8") as f:
        f.write("docs.zqk.dev\n")
    with open(os.path.join(target_dir, ".nojekyll"), "w", encoding="utf-8") as f:
        f.write("")

    pages = []
    category_counts = {}

    CATEGORY_ORDER = [
        ("Getting Started", "getting-started"),
        ("Architecture & Foundation", "architecture"),
        ("Specifications & Grammars", "specs"),
        ("Kernel Subsystems — Core Engines", "subsystems-core"),
        ("Kernel Subsystems — Spec & Command Builders", "subsystems-builders"),
        ("Kernel Subsystems — CLI Commands & Tooling", "subsystems-cli"),
        ("Kernel Subsystems — Internal Runtime", "subsystems-internal"),
        ("Kernel DNA & Object Schemas", "schemas"),
        ("Reference Manuals", "manual"),
        ("How-To & Incident Runbooks", "operations"),
        ("Tutorials, Demos & Guides", "tutorials"),
        ("Agent Skills & Protocols", "skills"),
        ("Agent Directives & Packs", "agent-directives"),
        ("Maintenance & Development", "development"),
        ("Quality & Evaluation", "quality"),
        ("Codebase Evaluation — Framework & Governance", "cef-framework"),
        ("Codebase Evaluation — Evaluation Rubrics", "cef-rubrics"),
        ("Codebase Evaluation — Specialist Prompts", "cef-specialists"),
        ("Codebase Evaluation — Adversarial Prompts", "cef-adversarial"),
        ("Codebase Evaluation — Audit Reports", "cef-reports"),
        ("Open Core Governance", "governance"),
    ]

    grouped_docs = {cat_name: [] for cat_name, _ in CATEGORY_ORDER}
    for item in doc_entries:
        cat_name = item["category"]
        if cat_name not in grouped_docs:
            grouped_docs[cat_name] = []
        grouped_docs[cat_name].append(item)

    # Dynamic category safeguard: ensure all populated categories appear in navigation
    ordered_cat_names = set(c[0] for c in CATEGORY_ORDER)
    for cat_name in grouped_docs:
        if cat_name not in ordered_cat_names and grouped_docs[cat_name]:
            cat_slug = cat_name.lower().replace(" ", "-").replace("&", "").replace("--", "-")
            CATEGORY_ORDER.append((cat_name, cat_slug))

    # Sort items within each category
    def sort_key(entry):
        rel = entry["rel_path"]
        if entry["category"] == "Kernel Subsystems — Core Engines":
            pkg_flow = [
                "pkg/README.md", "pkg/storage/README.md", "pkg/graph/README.md",
                "pkg/coordination/README.md", "pkg/concurrency/README.md",
                "pkg/pipeline/README.md", "pkg/mcp/README.md", "pkg/functional/README.md",
                "pkg/logging/README.md", "pkg/telemetry/README.md", "pkg/validation/README.md",
                "pkg/translation/README.md", "pkg/loader/README.md", "pkg/migration/README.md",
                "pkg/migration/detector/README.md", "pkg/goroutinelabels/README.md",
                "pkg/domain/organizational/README.md", "pkg/testing/README.md",
                "pkg/mcp/testing/README.md", "pkg/storage/change_journal_compaction.md",
                "pkg/storage/DATACELL_MIGRATION.md"
            ]
            if rel in pkg_flow:
                return (0, pkg_flow.index(rel))
        if entry["category"] == "Kernel Subsystems — Spec & Command Builders":
            bldr_flow = [
                "pkg/specbuilder/README.md", "pkg/cli/README.md",
                "pkg/cli/bldr_cli_cmd_v1/README.md", "pkg/specbuilder/bldr_v2/README.md",
                "pkg/specbuilder/builders/README.md", "pkg/specbuilder/instance_builders/README.md",
                "pkg/specbuilder/bootstrap/README.md"
            ]
            if rel in bldr_flow:
                return (0, bldr_flow.index(rel))
        if entry["category"] == "Kernel Subsystems — CLI Commands & Tooling":
            cmd_flow = [
                "cmd/zqk/README.md", "cmd/zqk-shim/README.md",
                "cmd/zqk/system/METRICS_ANALYSIS_README.md", "cmd/zqk/system/TESTING.md",
                "cmd/zqk/system/INTEGRITY_RESOLUTION_PLAN.md"
            ]
            if rel in cmd_flow:
                return (0, cmd_flow.index(rel))
        if entry["category"] == "Kernel Subsystems — Internal Runtime":
            internal_flow = [
                "internal/README.md", "internal/bootstrap/README.md",
                "internal/cli/README.md", "internal/cli/context/README.md"
            ]
            if rel in internal_flow:
                return (0, internal_flow.index(rel))
        if entry["category"] == "Codebase Evaluation — Framework & Governance":
            cef_flow = [
                "README.md", "CONSTITUTION.md", "DIAMOND_SCALE.md", "WAVE_PLAN.md",
                "LENSES.md", "DIAGRAM_CONTRACT.md", "OPERATOR.md", "KICKOFF_PROMPT.md",
                "HANDOFF_SCHEMA.md", "EXTENSIONS.md", "go.md"
            ]
            base = os.path.basename(rel)
            if base in cef_flow:
                return (0, cef_flow.index(base))
        if entry["category"] == "Codebase Evaluation — Audit Reports":
            eval_flow = [
                "README.md", "MNT-code-quality-maintainability.md",
                "OBS-observability-diagnostics.md", "RDB-architecture-package-boundaries.md",
                "REL-reliability-error-recovery.md", "SEC-security-threat-vectors.md",
                "TST-test-strategy-invariant-proofs.md"
            ]
            base = os.path.basename(rel)
            if base in eval_flow:
                return (0, eval_flow.index(base))
        # Pin index or overview docs to top of their category
        if "README.md" in rel or "INDEX.md" in rel or rel == "index.html":
            return (0, entry["title"])
        if "COMMUNITY_FIRST_RUN" in rel or "QUICKSTART" in rel or "ZQK_GETTING_STARTED" in rel or "zqk-expert/SKILL" in rel or "CONTRIBUTING" in rel:
            return (1, entry["title"])
        return (2, entry["title"])

    for cat_name in grouped_docs:
        grouped_docs[cat_name].sort(key=sort_key)

    def get_sidebar_nav_html(root_rel: str, current_html_rel: str) -> str:
        nav_html = ['<nav class="sidebar-nav">']
        # Top-level Overview
        active_home = ' class="active"' if current_html_rel == "index.html" else ''
        nav_html.append(f'<div class="sidebar-home"><a href="{root_rel}index.html"{active_home}>🏠 Portal Overview</a></div>')

        for cat_name, cat_key in CATEGORY_ORDER:
            items = grouped_docs.get(cat_name, [])
            if not items:
                continue

            contains_active = any(it["html_rel"] == current_html_rel for it in items)
            # Default open for core categories, or if this category contains the active page
            # Keep CEF closed by default unless active page is inside it
            is_open = contains_active or (cat_key in ("getting-started", "architecture", "specs", "manual", "operations", "skills", "agent-directives", "tutorials", "subsystems-core", "subsystems-builders", "subsystems-cli", "subsystems-internal", "schemas") and current_html_rel == "index.html")
            open_attr = ' open' if is_open else ''

            nav_html.append(f'<details class="sidebar-group"{open_attr}>')
            nav_html.append(f'<summary><span class="group-title">{html.escape(cat_name)}</span><span class="group-count">{len(items)}</span></summary>')
            nav_html.append('<ul>')
            for it in items:
                is_active = (it["html_rel"] == current_html_rel)
                active_cls = ' class="active"' if is_active else ''
                nav_title = html.escape(get_clean_nav_title(it["title"], it["rel_path"]))
                full_title = html.escape(it["title"])
                href = f"{root_rel}{it['html_rel']}"
                nav_html.append(f'<li><a href="{href}"{active_cls} title="{full_title}">{nav_title}</a></li>')
            nav_html.append('</ul>')
            nav_html.append('</details>')

        nav_html.append('</nav>')
        return "\n".join(nav_html)

    for item in doc_entries:
        cat_name = item["category"]
        category_counts[cat_name] = category_counts.get(cat_name, 0) + 1
        
        rendered_body, has_math = render_markdown_to_html(item["content"], item["html_rel"], link_map, repo_root, portal_build_id)
        escaped_title = html.escape(item["title"])
        
        depth = item["html_rel"].count("/")
        root_rel = "../" * depth if depth > 0 else ""
        sidebar_nav_html = get_sidebar_nav_html(root_rel, item["html_rel"])
        
        katex_tags = ""
        if has_math:
            katex_tags = """  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.css">
  <script defer src="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.js"></script>
  <script defer src="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/contrib/auto-render.min.js" onload="initKaTeX()"></script>
  <script>
    let katexRendered = false;
    function initKaTeX() {
      if (katexRendered) return;
      const target = document.querySelector('.markdown-body');
      if (target && typeof renderMathInElement === 'function') {
        renderMathInElement(target, {
          delimiters: [
            {left: '$$', right: '$$', display: true},
            {left: '\\\\(', right: '\\\\)', display: false},
            {left: '\\\\[', right: '\\\\]', display: true}
          ],
          ignoredTags: ['script', 'noscript', 'style', 'textarea', 'pre', 'code'],
          throwOnError: false
        });
        katexRendered = true;
      }
    }
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', initKaTeX);
    } else {
      initKaTeX();
    }
  </script>"""

        page_html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate">
  <meta http-equiv="Pragma" content="no-cache">
  <meta http-equiv="Expires" content="0">
  <title>{escaped_title} - ZQK Core Documentation</title>
  <link rel="stylesheet" href="{root_rel}assets/style.css?v={style_hash}">
  <link rel="icon" type="image/svg+xml" href="{root_rel}assets/zqk-logo.svg?v={logo_hash}">
  <script type="module">
    import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
    mermaid.initialize({{
      startOnLoad: false,
      securityLevel: 'loose',
      theme: 'dark',
      themeVariables: {{
        darkMode: true,
        background: '#0d1117',
        primaryColor: '#1f6feb',
        primaryTextColor: '#c9d1d9',
        primaryBorderColor: '#30363d',
        lineColor: '#58a6ff',
        secondaryColor: '#161b22',
        tertiaryColor: '#0d1117'
      }}
    }});

    async function initMermaid() {{
      const els = document.querySelectorAll('.mermaid, pre code.language-mermaid');
      if (!els.length) return;
      document.querySelectorAll('pre code.language-mermaid').forEach(el => {{
        const pre = el.parentElement;
        pre.className = 'mermaid';
        pre.textContent = el.textContent;
      }});
      try {{
        await mermaid.run({{ querySelector: '.mermaid' }});
      }} catch (err) {{
        console.warn('Mermaid rendering:', err);
      }}
    }}

    if (document.readyState === 'loading') {{
      document.addEventListener('DOMContentLoaded', initMermaid);
    }} else {{
      initMermaid();
    }}
  </script>
{katex_tags}
</head>
<body data-root-rel="{root_rel}">
  <header class="header">
    <div class="nav-container">
      <div class="brand">
        <a href="{root_rel}index.html" class="logo">
          {ZQK_HEADER_LOGO_SVG}
          <span class="logo-text">ZQK <span class="logo-accent">Core</span></span>
        </a>
        <span class="badge-tag">{html.escape(item["category"])}</span>
      </div>
      <div class="search-box">
        <input type="text" id="search-input" placeholder="Search core documentation..." onkeyup="runSearch()">
        <div id="search-results"></div>
      </div>
    </div>
  </header>
  <main class="content-container">
    <aside class="sidebar">
      {sidebar_nav_html}
    </aside>
    <article class="doc-body">
      <div class="breadcrumb">
        <div class="breadcrumb-trail">
          <a href="{root_rel}index.html">Docs</a> &raquo; <span>{html.escape(item["category"])}</span> &raquo; <span class="current">{escaped_title}</span>
        </div>
        <a href="{root_rel}{item['clean_md_rel']}" class="raw-md-link" title="View canonical Markdown source">Raw .md</a>
      </div>
      <div class="markdown-body">
        {rendered_body}
      </div>
    </article>
  </main>
  <script src="{root_rel}search/search-index.js?v={portal_build_id}"></script>
</body>
</html>
"""
        target_file_path = os.path.join(target_dir, item["html_rel"])
        os.makedirs(os.path.dirname(target_file_path), exist_ok=True)
        with open(target_file_path, "w", encoding="utf-8") as f:
            f.write(page_html)

        pages.append({
            "title": item["title"],
            "path": item["html_rel"],
            "category": item["category"],
            "snippet": item["snippet"]
        })

    # Build categories table for landing page
    cat_cards_html = ""
    for cat_name, cat_key in CATEGORY_ORDER:
        items = grouped_docs.get(cat_name, [])
        if not items:
            continue
        first_doc = items[0]
        preview_links = "".join([f'<li><a href="{it["html_rel"]}">{html.escape(get_clean_nav_title(it["title"], it["rel_path"]))}</a></li>' for it in items[:4]])
        if len(items) > 4:
            preview_links += f'<li class="more-link"><a href="{first_doc["html_rel"]}">+ {len(items)-4} more guides &rarr;</a></li>'
        cat_desc_map = {
            "Kernel Subsystems — Core Engines": "Architectural design, interface contracts, and core engine implementations across the Go microkernel.",
            "Kernel Subsystems — Spec & Command Builders": "Spec-driven builder patterns, versioned code generation engines, and generated Cobra command builders.",
            "Kernel Subsystems — CLI Commands & Tooling": "Command-line interfaces, daemon shims, operational diagnostics, and system test suites under cmd/.",
            "Kernel Subsystems — Internal Runtime": "Internal Go runtime utilities, embedded seed archives, and CLI context managers under internal/.",
            "Codebase Evaluation — Framework & Governance": "Constitutional invariants, multi-axis Diamond Scale grading, wave orchestration, and handoff contracts.",
            "Codebase Evaluation — Evaluation Rubrics": "Standardized, cited quality rubrics defining observable failure modes and criteria across 12 engineering dimensions.",
            "Codebase Evaluation — Specialist Prompts": "Lens-specialized investigation prompts for deep, evidence-backed codebase analysis.",
            "Codebase Evaluation — Adversarial Prompts": "Falsifiable adversarial auditor prompts designed to challenge, stress-test, and verify specialist findings.",
        }
        card_desc = cat_desc_map.get(cat_name, f"Authoritative open-core specifications and guides for {html.escape(cat_name.lower())}.")
        cat_cards_html += f"""
        <div class="cat-card">
          <div class="cat-card-header">
            <h4><a href="{first_doc['html_rel']}">{html.escape(cat_name)}</a></h4>
            <span class="cat-card-count">{len(items)} articles</span>
          </div>
          <p>{card_desc}</p>
          <ul class="cat-card-links">
            {preview_links}
          </ul>
        </div>
        """

    index_sidebar_html = get_sidebar_nav_html("", "index.html")
    index_html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate">
  <meta http-equiv="Pragma" content="no-cache">
  <meta http-equiv="Expires" content="0">
  <title>ZQK Core Documentation Portal</title>
  <link rel="stylesheet" href="assets/style.css?v={style_hash}">
  <link rel="icon" type="image/svg+xml" href="assets/zqk-logo.svg?v={logo_hash}">
  <script type="module">
    import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
    mermaid.initialize({{
      startOnLoad: false,
      securityLevel: 'loose',
      theme: 'dark',
      themeVariables: {{
        darkMode: true,
        background: '#0d1117',
        primaryColor: '#1f6feb',
        primaryTextColor: '#c9d1d9',
        primaryBorderColor: '#30363d',
        lineColor: '#58a6ff',
        secondaryColor: '#161b22',
        tertiaryColor: '#0d1117'
      }}
    }});

    async function initMermaid() {{
      const els = document.querySelectorAll('.mermaid, pre code.language-mermaid');
      if (!els.length) return;
      document.querySelectorAll('pre code.language-mermaid').forEach(el => {{
        const pre = el.parentElement;
        pre.className = 'mermaid';
        pre.textContent = el.textContent;
      }});
      try {{
        await mermaid.run({{ querySelector: '.mermaid' }});
      }} catch (err) {{
        console.warn('Mermaid rendering:', err);
      }}
    }}

    if (document.readyState === 'loading') {{
      document.addEventListener('DOMContentLoaded', initMermaid);
    }} else {{
      initMermaid();
    }}
  </script>
</head>
<body data-root-rel="">
  <header class="header">
    <div class="nav-container">
      <div class="brand">
        <a href="index.html" class="logo">
          {ZQK_HEADER_LOGO_SVG}
          <span class="logo-text">ZQK <span class="logo-accent">Core</span> <span style="font-size: 0.82rem; color: #8b949e; font-weight: normal; margin-left: 6px;">Community Docs</span></span>
        </a>
      </div>
      <div class="search-box">
        <input type="text" id="search-input" placeholder="Search core documentation..." onkeyup="runSearch()">
        <div id="search-results"></div>
      </div>
    </div>
  </header>
  <main class="content-container">
    <aside class="sidebar">
      {index_sidebar_html}
    </aside>
    <article class="doc-body">
      <div class="hero-box">
        <h1>ZQK Community Docs</h1>
        <p class="hero-desc">
          The Cellular Knowledge Operating System for autonomous AI agent swarms and human engineering teams.
        </p>
        <div class="portal-stat-badge">
          <strong>{len(pages)}</strong> official open-core documentation guides and specifications
        </div>
      </div>

      <h2>Core Documentation Quadrants</h2>
      <div class="quadrant-grid">
        <div class="quad-box">
          <h3>🚀 Onboarding & First-Run</h3>
          <p>Get up and running with the ZQK Core CLI, daemons, and autonomous agent seating.</p>
          <ul>
            <li><a href="docs/onboarding/COMMUNITY_FIRST_RUN.html">Community First-Run Guide</a></li>
            <li><a href="docs/onboarding/QUICKSTART.html">Quickstart & MCP Configuration</a></li>
            <li><a href="docs/onboarding/AI_AGENT_ONBOARDING.html">AI Agent Directives & Seating</a></li>
            <li><a href="docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.html">First-Run Object Tutorial</a></li>
            <li><a href="docs/onboarding/EDGE_HEADLESS_FIRST_RUN.html">Edge / Headless Mode</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>🏛️ Core Architecture</h3>
          <p>Deep foundational specifications governing the Knowledge Kernel.</p>
          <ul>
            <li><a href="docs/architecture/README.html">Core Architecture Overview</a></li>
            <li><a href="docs/architecture/AMBIENT_SIGNAL_ACTION_RUBRIC.html">Ambient Signal Action Rubric</a></li>
            <li><a href="docs/architecture/LIFECYCLE_STATE_MACHINE.html">Visual Lifecycle State Machines</a></li>
            <li><a href="docs/architecture/PACK_COMPOSITION_AND_EXTENSIBILITY.html">Modular Pack Composition</a></li>
            <li><a href="docs/architecture/CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.html">Cellular Membrane Mode B Runbook</a></li>
            <li><a href="docs/architecture/CLI_COMMAND_TAXONOMY_STANDARDS.html">CLI Command Taxonomy & Standards</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>📜 Specifications & Grammars</h3>
          <p>Formal AST grammars, execution engines, and query planning algorithms.</p>
          <ul>
            <li><a href="docs/specs/SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.html">ZPARQL Graph Traversal Grammar</a></li>
            <li><a href="docs/specs/SPEC-ZPARQL-QUERY-PLANNER.html">ZPARQL Indexed Query Planner</a></li>
            <li><a href="docs/specs/SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR.html">ZQL Mutation Grammar & AST</a></li>
            <li><a href="docs/specs/SPEC-VALIDATION-RULE-DSL-GRAMMAR.html">Validation Rule DSL Grammar</a></li>
            <li><a href="docs/specs/SPEC-ZQL-TRANSACTION-EXECUTION.html">ZQL ACID Transaction Execution</a></li>
            <li><a href="docs/specs/SPEC-OBJECT-INSPECTOR-CONSOLE-001.html">Interactive Object Inspector Spec</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>📖 Reference Manuals & Guides</h3>
          <p>Complete syntax reference, CLI options, and developer field guides.</p>
          <ul>
            <li><a href="docs/manual/README.html">Manual & CLI Reference</a></li>
            <li><a href="docs/guides/README.html">Developer & Agent Guides Index</a></li>
            <li><a href="docs/guides/KERNEL_OBJECT_USAGE_GUIDE.html">Kernel Object Model & Usage Guide</a></li>
            <li><a href="docs/guides/KNOWLEDGE_MANAGEMENT_AND_SEMANTIC_RECALL_GUIDE.html">Knowledge Management & Semantic Recall</a></li>
            <li><a href="docs/guides/ZQL_ZPARQL_AGENT_GUIDE.html">ZQL & ZPARQL Graph Operations</a></li>
            <li><a href="docs/guides/OBJECT_LIFECYCLE_AND_CAS_STORAGE_GUIDE.html">Object Lifecycle & CAS Storage</a></li>
            <li><a href="docs/guides/POLICY_CREATION_AND_VALIDATION_DSL_GUIDE.html">Custom Policy & Rule DSL</a></li>
            <li><a href="docs/guides/SINGLE_COMMAND_EXECUTION_LOOP_GUIDE.html">Single-Command Loop (zqk do)</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>🚨 Incident Runbooks & Operations</h3>
          <p>Operational triage recipes, crash recovery, and daemon health management.</p>
          <ul>
            <li><a href="docs/runbooks/README.html">Operational Incident Runbooks</a></li>
            <li><a href="docs/runbooks/RB-CAS-001-CAS-CORRUPTION-RECOVERY.html">RB-CAS-001: CAS Hash Recovery</a></li>
            <li><a href="docs/runbooks/RB-LCK-001-LOCK-CONTENTION-DEADLOCKS.html">RB-LCK-001: Lock Contention</a></li>
            <li><a href="docs/runbooks/RB-SCH-001-SCHEDULER-DAEMON-TRIAGE.html">RB-SCH-001: Scheduler Triage</a></li>
            <li><a href="docs/runbooks/RB-WAL-001-WAL-COMPACTION-FAILURES.html">RB-WAL-001: WAL Failures</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>🔬 Quality & Codebase Evaluation</h3>
          <p>Multi-agent evaluation framework (CEF), Diamond Scale, and 52 lens rubrics.</p>
          <ul>
            <li><a href="docs/quality/README.html">Quality & Verification Gates (DoD/VDS)</a></li>
            <li><a href="docs/quality/codebase_evaluation/README.html">CEF Multi-Agent Evaluation Framework</a></li>
            <li><a href="docs/quality/codebase_evaluation/CONSTITUTION.html">CEF Evaluation Constitution</a></li>
            <li><a href="docs/quality/codebase_evaluation/DIAMOND_SCALE.html">Diamond Scale Multi-Axis Quality</a></li>
            <li><a href="docs/quality/codebase_evaluation/LENSES.html">52 Evaluation Dimensions & Lenses</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>🔧 Maintenance & Development</h3>
          <p>Foundational engineering conventions, policy durability models, and AST gates.</p>
          <ul>
            <li><a href="docs/development/README.html">Maintenance & Development Overview</a></li>
            <li><a href="docs/development/POLICY_GOVERNANCE_AND_DURABILITY.html">Policy Governance & Durability</a></li>
            <li><a href="docs/howto/SCHEDULER_AND_MAINTENANCE.html">Scheduler & Maintenance Jobs</a></li>
            <li><a href="docs/explanation/README.html">System Architecture Philosophy</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>📦 Kernel Subsystems & Core Engines</h3>
          <p>Architectural design, interface contracts, and storage implementations across the Go microkernel.</p>
          <ul>
            <li><a href="pkg/README.html">Go Packages Master Index (pkg/)</a></li>
            <li><a href="pkg/storage/README.html">Storage Subsystem & Providers</a></li>
            <li><a href="pkg/graph/README.html">Graph Backend & MemGraph Provider</a></li>
            <li><a href="pkg/coordination/README.html">Event Coordination System (Spinal Cord)</a></li>
            <li><a href="pkg/mcp/README.html">Model Context Protocol (MCP) Server</a></li>
            <li><a href="pkg/cli/bldr_cli_cmd_v1/README.html">CLI Command Builders (bldr_cli_cmd_v1)</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>🤖 Agent Operating Protocols</h3>
          <p>Directives, MCP integration, persona seating, and continuous autonomous loop discipline.</p>
          <ul>
            <li><a href="docs/onboarding/AI_AGENT_ONBOARDING.html">AI Agent Directives & Seating</a></li>
            <li><a href="docs/architecture/AMBIENT_SIGNAL_ACTION_RUBRIC.html">Ambient Signal Action Rubric</a></li>
            <li><a href="docs/guides/AGENT_ORCHESTRATION_AND_SWARM_COLLABORATION_GUIDE.html">Swarm Orchestration & Mesh</a></li>
            <li><a href="docs/guides/SINGLE_COMMAND_EXECUTION_LOOP_GUIDE.html">Single-Command Loop (zqk do)</a></li>
            <li><a href="docs/guides/DIAGNOSTICS_SELF_HEALING_AND_REMEDY_GUIDE.html">Diagnostics & Self-Healing</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>⚖️ Open Core Governance</h3>
          <p>Open-source policies, contributing guidelines, and security disclosures.</p>
          <ul>
            <li><a href="CONTRIBUTING.html">Contributing & DCO Compliance</a></li>
            <li><a href="SECURITY.html">Security Vulnerability Disclosures</a></li>
            <li><a href="GOVERNANCE.html">Open-Core Decision Making</a></li>
            <li><a href="CODE_OF_CONDUCT.html">Contributor Code of Conduct</a></li>
          </ul>
        </div>
      </div>

      <h2>Core Documentation Directory</h2>
      <p>Explore all {len(pages)} canonical guides published in ZQK Core:</p>
      <div class="category-grid">
        {cat_cards_html}
      </div>
    </article>
  </main>
  <script src="search/search-index.js?v={portal_build_id}"></script>
</body>
</html>
"""
    with open(os.path.join(target_dir, "index.html"), "w", encoding="utf-8") as f:
        f.write(index_html)

    # Write CSS
    with open(os.path.join(assets_dir, "style.css"), "w", encoding="utf-8") as f:
        f.write(style_css)


    # Write search index
    pages_json_str = json.dumps(pages, ensure_ascii=False)
    search_js = f"""const docsIndex = {pages_json_str};

function runSearch() {{
  const query = document.getElementById('search-input').value.toLowerCase();
  const resultsDiv = document.getElementById('search-results');
  if (!query || query.length < 2) {{
    resultsDiv.style.display = 'none';
    resultsDiv.innerHTML = '';
    return;
  }}
  const rootRel = document.body.getAttribute('data-root-rel') || '';
  const matched = docsIndex.filter(d => 
    d.title.toLowerCase().includes(query) || 
    d.category.toLowerCase().includes(query) || 
    d.snippet.toLowerCase().includes(query)
  );
  if (matched.length === 0) {{
    resultsDiv.innerHTML = '<div style="padding: 12px; color: #8b949e;">No documentation matches found</div>';
  }} else {{
    resultsDiv.innerHTML = matched.slice(0, 20).map(m => 
      '<div class="search-result-item">' +
      '  <a href="' + rootRel + m.path + '">' + m.title + '</a>' +
      '  <span class="search-result-category">[' + m.category + ']</span>' +
      '  <p>' + m.snippet + '...</p>' +
      '</div>'
    ).join('');
  }}
  resultsDiv.style.display = 'block';
}}

document.addEventListener('click', function(e) {{
  const searchBox = document.querySelector('.search-box');
  const resultsDiv = document.getElementById('search-results');
  if (searchBox && !searchBox.contains(e.target)) {{
    resultsDiv.style.display = 'none';
  }}
}});
"""
    with open(os.path.join(search_dir, "search-index.js"), "w", encoding="utf-8") as f:
        f.write(search_js)

    # Generate clean root-level aliases and redirect stubs for all docs/* pages
    # So that URLs like https://docs.zqk.dev/onboarding/COMMUNITY_FIRST_RUN or /specs/... work seamlessly
    for item in doc_entries:
        html_rel = item["html_rel"]
        if html_rel.startswith("docs/"):
            clean_alias_rel = html_rel[len("docs/"):]
            alias_path = os.path.join(target_dir, clean_alias_rel)
            if not os.path.exists(alias_path):
                os.makedirs(os.path.dirname(alias_path), exist_ok=True)
                alias_dir = os.path.dirname(clean_alias_rel)
                rel_to_canonical = os.path.relpath(html_rel, alias_dir) if alias_dir else html_rel
                redirect_html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta http-equiv="refresh" content="0; url={rel_to_canonical}">
  <link rel="canonical" href="https://docs.zqk.dev/{html_rel[:-5] if html_rel.endswith('.html') else html_rel}">
  <script>window.location.replace('{rel_to_canonical}' + window.location.hash);</script>
  <title>Redirecting to {html.escape(item['title'])} — ZQK Docs</title>
</head>
<body style="background:#05070a;color:#c9d1d9;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;">
  <p>Redirecting to <a href="{rel_to_canonical}" style="color:#00e5ff;">canonical documentation</a>...</p>
</body>
</html>
"""
                with open(alias_path, "w", encoding="utf-8") as f:
                    f.write(redirect_html)

    # Write custom 404.html for GitHub Pages with smart path resolution and branded fallback
    custom_404_html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate">
  <meta http-equiv="Pragma" content="no-cache">
  <meta http-equiv="Expires" content="0">
  <title>Page Not Found — ZQK Documentation</title>
  <link rel="stylesheet" href="/assets/style.css?v={style_hash}">
  <link rel="icon" type="image/svg+xml" href="/assets/zqk-logo.svg?v={logo_hash}">
  <script>
    // Automatic intelligent client-side redirect for links missing /docs/ or missing .html
    (function() {{
      var path = window.location.pathname;
      if (!path.startsWith('/docs/') && path !== '/' && !path.startsWith('/assets/') && !path.startsWith('/search/')) {{
        var cleanPath = path;
        if (!cleanPath.endsWith('.html') && !cleanPath.endsWith('/')) {{
          cleanPath += '.html';
        }}
        var candidate = '/docs' + (cleanPath.startsWith('/') ? cleanPath : '/' + cleanPath);
        window.location.replace(candidate + window.location.search + window.location.hash);
      }}
    }})();
  </script>
</head>
<body class="docs-page">
  <header class="header">
    <div class="nav-container">
      <div class="brand">
        <a href="/" class="logo">
          {ZQK_HEADER_LOGO_SVG}
          <span class="logo-text">ZQK <span class="logo-accent">Core</span> <span style="font-size: 0.82rem; color: #8b949e; font-weight: normal; margin-left: 6px;">Community Docs</span></span>
        </a>
      </div>
      <div class="search-box">
        <input type="text" id="search-input" placeholder="Search documentation... (Press '/' to focus)" oninput="runSearch()">
        <div id="search-results"></div>
      </div>
    </div>
  </header>
  <main class="content-container" style="justify-content:center; text-align:center; padding: 4rem 1rem;">
    <div style="max-width: 600px; margin: 0 auto;">
      <h1 style="font-size: 3.5rem; color: #00e5ff; margin-bottom: 1rem;">404</h1>
      <h2 style="color: #fff; margin-bottom: 1rem;">Documentation Page Not Found</h2>
      <p style="color: #8b949e; line-height: 1.6; margin-bottom: 2rem;">The requested page could not be located. You can search our documentation above or return to the portal homepage.</p>
      <a href="/" style="display:inline-block; padding: 10px 20px; background: #00e5ff; color: #05070a; font-weight: 600; border-radius: 6px; text-decoration: none;">&larr; Return to Documentation Portal</a>
    </div>
  </main>
  <script src="/search/search-index.js?v={portal_build_id}"></script>
</body>
</html>
"""
    with open(os.path.join(target_dir, "404.html"), "w", encoding="utf-8") as f:
        f.write(custom_404_html)

    print(f"✅ ZQK Core docs portal successfully generated ({len(pages)} articles).")

def verify_tarball(tarball_path: str) -> bool:
    """Verifies that the generated documentation tarball contains valid structure."""
    if not os.path.exists(tarball_path):
        print(f"Error: tarball does not exist: {tarball_path}", file=sys.stderr)
        return False
    try:
        import tarfile
        with tarfile.open(tarball_path, "r:gz") as tar:
            names = tar.getnames()
            has_index = any(n.endswith("index.html") for n in names)
            has_style = any("assets/style.css" in n for n in names)
            if has_index and has_style:
                print(f"Tarball {tarball_path} verified: valid structure (contains index.html and assets/style.css)")
                return True
            else:
                print(f"Error: tarball missing required index.html or assets/style.css", file=sys.stderr)
                return False
    except Exception as e:
        print(f"Error reading tarball {tarball_path}: {e}", file=sys.stderr)
        return False

def main():
    if len(sys.argv) > 1 and sys.argv[1] in ("--help", "-h"):
        print("Usage: generate_docs_portal.py [output_dir] | --verify <tarball>")
        sys.exit(0)

    if len(sys.argv) > 1 and sys.argv[1] == "--verify":
        if len(sys.argv) < 3:
            print("Error: --verify requires tarball path", file=sys.stderr)
            sys.exit(1)
        ok = verify_tarball(sys.argv[2])
        sys.exit(0 if ok else 1)

    script_dir = os.path.dirname(os.path.abspath(__file__))
    # Try git rev-parse first, fallback to ../../.. relative to this script
    try:
        repo_root = subprocess.check_output(
            ["git", "-C", script_dir, "rev-parse", "--show-toplevel"],
            text=True
        ).strip()
    except Exception:
        repo_root = os.path.abspath(os.path.join(script_dir, "../../.."))

    output_dir = sys.argv[1] if len(sys.argv) > 1 else "dist-docs"
    
    if not os.path.isabs(output_dir):
        target_dir = os.path.join(repo_root, output_dir)
    else:
        target_dir = output_dir

    try:
        build_portal(repo_root, target_dir)
    except Exception as e:
        print(f"Error generating documentation portal: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
