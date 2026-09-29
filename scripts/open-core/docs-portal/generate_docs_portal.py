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

ZQK_LOGO_SVG = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100%" height="100%">
  <defs>
    <filter id="cursor-glow" x="-30%" y="-30%" width="160%" height="160%">
      <feDropShadow dx="0" dy="0" stdDeviation="1.5" flood-color="#00e676" flood-opacity="0.8"/>
    </filter>
  </defs>
  <!-- Background icon frame -->
  <rect width="100" height="100" rx="22" fill="#05070a" stroke="#1f2937" stroke-width="1.5"/>
  <!-- ZQK glyphs in pure vector geometry fill-rule evenodd for Q hole -->
  <path d="M54.70,35.38 L59.41,35.38 L59.41,44.46 L59.74,44.46 L59.74,44.12 L60.08,44.12 L60.08,43.78 L60.75,43.45 L60.75,42.78 L61.09,42.78 L61.09,42.44 L61.76,42.10 L61.76,41.43 L62.10,41.43 L62.10,41.10 L62.43,41.10 L62.43,40.76 L63.10,40.42 L63.10,39.75 L63.44,39.75 L63.44,39.42 L63.78,39.42 L63.78,39.08 L64.45,38.74 L64.45,38.07 L64.78,38.07 L64.78,37.74 L65.46,37.40 L65.46,36.73 L65.79,36.73 L65.79,36.39 L66.13,36.39 L66.13,36.06 L66.46,36.06 L66.80,35.38 L71.84,35.38 L71.84,35.72 L71.50,35.72 L71.50,36.39 L71.17,36.39 L71.17,36.73 L70.83,36.73 L70.83,37.06 L70.16,37.40 L70.16,38.07 L69.82,38.07 L69.82,38.41 L69.49,38.41 L69.49,38.74 L68.82,39.08 L68.82,39.75 L68.48,39.75 L68.48,40.09 L68.14,40.09 L68.14,40.42 L67.47,40.76 L67.47,41.43 L67.14,41.43 L67.14,41.77 L66.80,41.77 L66.80,42.10 L66.13,42.44 L66.13,43.11 L65.79,43.11 L65.79,43.45 L65.46,43.45 L65.46,43.78 L64.78,44.12 L64.78,45.13 L65.12,45.13 L65.12,45.80 L65.46,45.80 L65.46,46.47 L65.79,46.47 L65.79,47.14 L66.13,47.14 L66.46,48.49 L67.14,48.82 L67.14,49.50 L67.47,49.50 L67.47,50.17 L67.81,50.17 L67.81,50.84 L68.14,50.84 L68.14,51.51 L68.48,51.51 L68.82,52.86 L69.49,53.19 L69.49,53.86 L69.82,53.86 L69.82,54.54 L70.16,54.54 L70.16,55.21 L70.50,55.21 L70.50,55.88 L70.83,55.88 L71.17,57.22 L71.84,57.56 L72.18,58.90 L67.14,58.90 L67.14,58.57 L66.80,58.57 L66.80,57.90 L66.46,57.90 L66.46,57.22 L66.13,57.22 L66.13,56.55 L65.79,56.55 L65.79,55.88 L65.46,55.88 L65.46,55.21 L64.78,54.87 L64.78,54.20 L64.45,54.20 L64.45,53.53 L64.11,53.53 L64.11,52.86 L63.78,52.86 L63.78,52.18 L63.44,52.18 L63.44,51.51 L63.10,51.51 L63.10,50.84 L62.77,50.84 L62.77,50.17 L62.43,50.17 L62.43,49.50 L62.10,49.50 L62.10,48.82 L61.76,48.82 L61.76,48.49 L61.42,48.49 L61.42,48.82 L61.09,48.82 L61.09,49.16 L60.42,49.50 L60.42,50.17 L60.08,50.17 L60.08,50.50 L59.41,50.84 L59.41,58.90 L54.70,58.90Z M8.00,54.87 L8.34,54.87 L8.34,54.20 L9.01,53.86 L9.01,53.19 L9.68,52.86 L9.68,52.18 L10.35,51.85 L10.35,51.18 L11.02,50.84 L11.02,50.17 L11.70,49.83 L11.70,49.16 L12.37,48.82 L12.37,48.15 L13.04,47.82 L13.04,47.14 L13.38,47.14 L13.38,46.81 L13.71,46.81 L13.71,46.14 L14.05,46.14 L14.05,45.80 L14.72,45.46 L14.72,44.79 L15.39,44.46 L15.39,43.78 L16.06,43.45 L16.06,42.78 L16.74,42.44 L16.74,41.77 L17.41,41.43 L17.41,40.76 L17.74,40.76 L17.74,40.42 L18.08,40.42 L18.08,39.75 L18.42,39.75 L18.42,39.42 L8.34,39.42 L8.34,35.38 L24.13,35.38 L24.13,39.42 L23.79,39.42 L23.79,40.09 L23.12,40.42 L23.12,41.10 L22.45,41.43 L22.45,42.10 L22.11,42.10 L22.11,42.44 L21.44,42.78 L21.44,43.45 L20.77,43.78 L20.77,44.46 L20.10,44.79 L20.10,45.46 L19.76,45.46 L19.76,45.80 L19.09,46.14 L19.09,46.81 L18.42,47.14 L18.42,47.82 L17.74,48.15 L17.74,48.82 L17.41,48.82 L17.41,49.16 L16.74,49.50 L16.74,50.17 L16.06,50.50 L16.06,51.18 L15.73,51.18 L15.73,51.51 L15.39,51.51 L15.39,52.18 L15.06,52.18 L15.06,52.52 L14.38,52.86 L14.38,53.53 L13.71,53.86 L13.71,54.54 L13.38,54.54 L13.38,54.87 L24.46,54.87 L24.46,58.90 L8.00,58.90Z M31.18,42.78 L31.52,42.78 L31.86,40.09 L32.19,40.09 L32.19,39.42 L32.53,39.42 L32.86,38.07 L33.20,38.07 L33.20,37.74 L33.54,37.74 L33.54,37.40 L33.87,37.40 L33.87,37.06 L34.21,37.06 L34.21,36.73 L34.54,36.73 L34.88,36.06 L35.55,36.06 L35.55,35.72 L36.22,35.72 L36.22,35.38 L37.23,35.38 L37.23,35.05 L41.26,35.05 L41.26,35.38 L43.28,35.72 L43.28,36.06 L43.62,36.06 L43.95,36.73 L44.62,36.73 L44.62,37.06 L44.96,37.06 L44.96,37.74 L45.63,38.07 L45.63,38.74 L45.97,38.74 L45.97,39.42 L46.30,39.42 L46.30,40.09 L46.64,40.09 L46.64,41.10 L46.98,41.10 L46.98,42.44 L47.31,42.44 L47.31,45.46 L47.65,45.46 L47.65,49.16 L47.31,49.16 L47.31,51.85 L46.98,51.85 L46.98,53.19 L46.64,53.19 L46.30,55.21 L45.97,55.21 L45.97,55.88 L45.30,56.22 L45.30,56.89 L44.96,56.89 L44.96,57.22 L44.62,57.22 L44.29,57.90 L43.62,57.90 L43.62,58.23 L43.95,58.23 L43.95,58.57 L44.29,58.57 L44.29,58.90 L44.62,58.90 L44.62,59.24 L44.96,59.24 L44.96,59.58 L45.30,59.58 L45.30,59.91 L45.63,59.91 L45.63,60.25 L46.30,60.58 L46.30,61.26 L45.97,61.26 L45.63,61.93 L44.96,61.93 L44.96,62.26 L44.62,62.26 L44.29,62.94 L43.62,62.94 L43.62,63.27 L43.28,63.27 L43.28,62.94 L42.94,62.94 L42.94,62.60 L42.61,62.60 L42.61,62.26 L41.94,61.93 L41.94,61.26 L41.60,61.26 L41.60,60.92 L41.26,60.92 L41.26,60.58 L40.93,60.58 L40.93,60.25 L40.59,60.25 L40.59,59.91 L40.26,59.91 L39.92,59.24 L36.90,59.24 L36.90,58.90 L35.89,58.90 L35.55,58.23 L34.88,58.23 L34.88,57.90 L34.54,57.90 L34.54,57.56 L34.21,57.56 L34.21,57.22 L33.87,57.22 L33.87,56.89 L33.54,56.89 L33.54,56.55 L32.86,56.22 L32.86,55.54 L32.53,55.54 L32.53,54.87 L32.19,54.87 L32.19,54.20 L31.86,54.20 L31.52,51.51 L31.18,51.51Z M35.89,43.45 L35.89,50.84 L36.22,50.84 L36.22,52.52 L36.56,52.52 L36.56,53.53 L36.90,53.53 L36.90,53.86 L37.23,53.86 L37.23,54.54 L37.90,54.54 L38.24,55.21 L40.26,55.21 L40.26,54.87 L40.93,54.87 L40.93,54.54 L41.60,54.20 L41.60,53.53 L41.94,53.53 L41.94,52.52 L42.27,52.52 L42.27,51.18 L42.61,51.18 L42.61,43.11 L42.27,43.11 L42.27,41.77 L41.94,41.77 L41.94,40.76 L41.60,40.76 L41.60,40.09 L41.26,40.09 L40.93,39.42 L39.92,39.42 L39.92,39.08 L38.58,39.08 L38.58,39.42 L37.23,39.75 L37.23,40.42 L36.56,40.76 L36.56,41.77 L36.22,41.77 L36.22,43.45Z" fill="#ffffff" fill-rule="evenodd"/>
  <!-- Terminal cursor prompt _ with emerald green glow -->
  <path d="M72.51,61.93 L92.00,61.93 L92.00,64.95 L72.51,64.95Z" fill="#00e676" filter="url(#cursor-glow)"/>
</svg>"""

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
    if path.startswith("pkg/") or path.startswith("internal/") or path.startswith("cmd/"):
        return "Kernel Subsystems & Go Packages", "subsystems"
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
        if rel_path == "ANTIGRAVITY.md":
            return "Maintenance & Development", "development"
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
            return "Codebase Evaluation Framework", "codebase-eval"
        return "Quality & Evaluation", "quality"
    elif sub == "eval":
        return "Quality & Evaluation", "quality"

    return sub.replace("_", " ").title(), sub

def get_clean_nav_title(title: str) -> str:
    """Returns a concise, scannable title for sidebar navigation."""
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

def render_markdown_to_html(content: str, current_html_rel: str, link_map: dict, repo_root: str = "", default_hash: str = "") -> str:
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
    
    if HAS_MARKDOWN:
        try:
            rendered = markdown.markdown(
                transformed,
                extensions=['fenced_code', 'tables', 'toc', 'sane_lists']
            )
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
            return rendered
        except Exception:
            pass
            
    preview = f"<pre class=\"markdown-preview\">{html.escape(content)}</pre>"
    return bust_html_img_cache(preview, current_html_rel, repo_root, default_hash)

def get_portal_style_css() -> str:
    return """
:root {
  --bg-primary: #05070c;
  --bg-secondary: #0d1117;
  --bg-tertiary: #161b22;
  --border-color: #30363d;
  --text-main: #c9d1d9;
  --text-muted: #8b949e;
  --accent-cyan: #00e5ff;
  --accent-blue: #58a6ff;
  --accent-green: #00e676;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  margin: 0;
  background: var(--bg-primary);
  color: var(--text-main);
  line-height: 1.6;
}

.header {
  background: var(--bg-secondary);
  padding: 0.85rem 2rem;
  border-bottom: 1px solid var(--border-color);
  position: sticky;
  top: 0;
  z-index: 1000;
}

.nav-container {
  display: flex;
  justify-content: space-between;
  align-items: center;
  max-width: 1400px;
  margin: 0 auto;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 1.2rem;
  font-weight: 700;
  color: #fff;
  text-decoration: none;
}

.logo-mark {
  display: block;
  border-radius: 6px;
}

.logo-accent {
  color: var(--accent-cyan);
}

.badge-tag {
  background: rgba(0, 229, 255, 0.12);
  color: var(--accent-cyan);
  border: 1px solid rgba(0, 229, 255, 0.3);
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 0.75rem;
  font-family: monospace;
}

.content-container {
  display: flex;
  max-width: 1400px;
  margin: 1.5rem auto;
  padding: 0 1.5rem;
  gap: 2rem;
}

.sidebar {
  width: 300px;
  flex-shrink: 0;
  border-right: 1px solid var(--border-color);
  padding-right: 1rem;
  max-height: calc(100vh - 90px);
  position: sticky;
  top: 70px;
  overflow-y: auto;
}

.sidebar-home {
  margin-bottom: 1rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid var(--border-color);
}

.sidebar-home a {
  color: var(--accent-cyan);
  text-decoration: none;
  font-size: 0.95rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  background: rgba(0, 229, 255, 0.06);
  border: 1px solid rgba(0, 229, 255, 0.15);
  transition: all 0.15s ease;
}

.sidebar-home a:hover, .sidebar-home a.active {
  background: rgba(0, 229, 255, 0.15);
  border-color: var(--accent-cyan);
}

.sidebar-group {
  margin-bottom: 0.85rem;
}

.sidebar-group summary {
  cursor: pointer;
  font-size: 0.8rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-muted);
  padding: 4px 6px;
  border-radius: 4px;
  user-select: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  transition: all 0.15s ease;
}

.sidebar-group summary:hover {
  color: #fff;
  background: var(--bg-tertiary);
}

.sidebar-group summary .group-title {
  display: flex;
  align-items: center;
  gap: 6px;
}

.sidebar-group summary .group-count {
  font-size: 0.7rem;
  font-weight: 500;
  background: var(--bg-tertiary);
  color: var(--accent-cyan);
  padding: 1px 6px;
  border-radius: 8px;
}

.sidebar-group ul {
  list-style: none;
  padding: 0 0 0 10px;
  margin: 0.4rem 0 0.6rem 4px;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
}

.sidebar-group li {
  margin-bottom: 0.25rem;
}

.sidebar-group a {
  color: var(--text-main);
  text-decoration: none;
  font-size: 0.86rem;
  display: block;
  padding: 4px 8px;
  border-radius: 4px;
  transition: all 0.15s ease;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-group a:hover {
  background: var(--bg-tertiary);
  color: var(--accent-cyan);
}

.sidebar-group a.active {
  background: rgba(0, 229, 255, 0.12);
  color: var(--accent-cyan);
  font-weight: 600;
  border-left: 2px solid var(--accent-cyan);
}

.doc-body {
  flex-grow: 1;
  min-width: 0;
}

.breadcrumb {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.85rem;
  color: var(--text-muted);
  margin-bottom: 1.5rem;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 0.75rem;
}

.breadcrumb-trail a {
  color: var(--accent-blue);
  text-decoration: none;
}

.breadcrumb .current {
  color: var(--text-main);
}

.raw-md-link {
  font-size: 0.75rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  color: var(--accent-cyan);
  padding: 3px 10px;
  border-radius: 6px;
  text-decoration: none;
  font-family: monospace;
  transition: all 0.15s ease;
}

.raw-md-link:hover {
  background: var(--border-color);
  color: #fff;
}

.hero-box {
  background: linear-gradient(135deg, rgba(0, 229, 255, 0.08), rgba(88, 166, 255, 0.04));
  border: 1px solid rgba(0, 229, 255, 0.2);
  border-radius: 8px;
  padding: 2rem;
  margin-bottom: 2rem;
}

.hero-box h1 {
  margin-top: 0;
  color: #fff;
}

.hero-desc {
  font-size: 1.1rem;
  color: #a5d6ff;
  max-width: 800px;
}

.portal-stat-badge {
  display: inline-block;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  padding: 6px 14px;
  border-radius: 20px;
  font-size: 0.9rem;
  color: var(--accent-green);
  margin-top: 1rem;
}

.quadrant-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 1.5rem;
  margin-bottom: 2.5rem;
}

.quad-box {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 1.5rem;
}

.quad-box h3 {
  margin-top: 0;
  color: #fff;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 0.5rem;
}

.quad-box ul {
  padding-left: 1.2rem;
  margin-bottom: 0;
}

.quad-box li {
  margin-bottom: 0.4rem;
}

.quad-box a {
  color: var(--accent-blue);
  text-decoration: none;
}

.quad-box a:hover {
  text-decoration: underline;
}

.category-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 1rem;
  margin-bottom: 2.5rem;
}

.cat-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  padding: 1rem;
}

.cat-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.cat-card h4 {
  margin: 0;
  font-size: 0.95rem;
  color: #fff;
}

.cat-card h4 a {
  color: #fff;
  text-decoration: none;
}

.cat-card h4 a:hover {
  color: var(--accent-cyan);
}

.cat-card-count {
  font-size: 0.75rem;
  background: var(--bg-tertiary);
  color: var(--accent-cyan);
  padding: 2px 6px;
  border-radius: 10px;
}

.cat-card p {
  margin: 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.cat-card-links {
  list-style: none;
  padding: 0;
  margin: 0.75rem 0 0;
}

.cat-card-links li {
  margin-bottom: 0.35rem;
}

.cat-card-links a {
  color: var(--accent-blue);
  text-decoration: none;
  font-size: 0.82rem;
  display: block;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cat-card-links a:hover {
  text-decoration: underline;
}

.cat-card-links .more-link a {
  color: var(--accent-cyan);
  font-weight: 500;
  margin-top: 4px;
}

.search-box {
  position: relative;
}

.search-box input {
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 0.5rem 1rem;
  border-radius: 6px;
  width: 280px;
  font-size: 0.9rem;
}

.search-box input:focus {
  outline: none;
  border-color: var(--accent-cyan);
}

#search-results {
  position: absolute;
  right: 0;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  width: 380px;
  max-height: 400px;
  overflow-y: auto;
  display: none;
  margin-top: 8px;
  box-shadow: 0 10px 30px rgba(0,0,0,0.8);
  z-index: 1000;
}

.search-result-item {
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-color);
}

.search-result-item a {
  color: var(--accent-blue);
  font-weight: 600;
  text-decoration: none;
  font-size: 0.95rem;
}

.search-result-category {
  font-size: 0.75rem;
  color: var(--accent-cyan);
  margin-left: 6px;
}

.search-result-item p {
  margin: 4px 0 0;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.markdown-body {
  color: var(--text-main);
}

.markdown-body h1, .markdown-body h2, .markdown-body h3 {
  color: #fff;
  border-bottom: 1px solid rgba(255,255,255,0.08);
  padding-bottom: 0.3em;
  margin-top: 1.5em;
}

.markdown-body a {
  color: var(--accent-blue);
  text-decoration: none;
}

.markdown-body a:hover {
  text-decoration: underline;
}

.markdown-body pre {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  padding: 1rem;
  border-radius: 6px;
  overflow-x: auto;
}

.markdown-body code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  background: rgba(255, 255, 255, 0.08);
  padding: 0.2em 0.4em;
  border-radius: 4px;
  font-size: 85%;
}

.markdown-body pre code {
  background: transparent;
  padding: 0;
}

.markdown-body table {
  width: 100%;
  border-collapse: collapse;
  margin: 1.5rem 0;
}

.markdown-body th, .markdown-body td {
  border: 1px solid var(--border-color);
  padding: 8px 12px;
  text-align: left;
}

.markdown-body th {
  background: var(--bg-tertiary);
  color: #fff;
}

.markdown-body img {
  max-width: 100%;
  height: auto;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  margin: 1.5rem 0;
  display: block;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
}

/* Mermaid Architectural Diagram Styling */
.mermaid {
  background: var(--bg-secondary) !important;
  border: 1px solid var(--border-color) !important;
  border-radius: 8px !important;
  padding: 1.75rem 1rem !important;
  margin: 1.75rem 0 !important;
  overflow-x: auto !important;
  white-space: pre !important;
}

.mermaid[data-processed="true"] {
  display: flex !important;
  justify-content: center !important;
  align-items: center !important;
  white-space: normal !important;
}

.mermaid svg {
  max-width: 100% !important;
  height: auto !important;
}

/* GitHub Alert Callouts */
.alert {
  border-left: 4px solid var(--border-color);
  background: var(--bg-secondary);
  border-radius: 6px;
  padding: 1rem 1.25rem;
  margin: 1.5rem 0;
}

.alert-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  font-size: 0.95rem;
  margin-bottom: 0.5rem;
}

.alert-title svg {
  flex-shrink: 0;
}

.alert-body {
  color: var(--text-main);
  font-size: 0.95rem;
  line-height: 1.6;
}

.alert-body p:last-child {
  margin-bottom: 0;
}

.alert-note {
  border-left-color: #2f81f7;
  background: rgba(56, 139, 253, 0.08);
}
.alert-note .alert-title {
  color: #58a6ff;
}

.alert-tip {
  border-left-color: #3fb950;
  background: rgba(63, 185, 80, 0.08);
}
.alert-tip .alert-title {
  color: #3fb950;
}

.alert-important {
  border-left-color: #a371f7;
  background: rgba(163, 113, 247, 0.08);
}
.alert-important .alert-title {
  color: #bc8cff;
}

.alert-warning {
  border-left-color: #d29922;
  background: rgba(210, 153, 34, 0.08);
}
.alert-warning .alert-title {
  color: #d29922;
}

.alert-caution {
  border-left-color: #f85149;
  background: rgba(248, 81, 73, 0.08);
}
.alert-caution .alert-title {
  color: #f85149;
}
"""

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

    for root_doc in ["README.md", "CONTRIBUTING.md", "SECURITY.md", "GOVERNANCE.md", "CODE_OF_CONDUCT.md", "PACK-COMPOSITION.md", "ZQK_GETTING_STARTED.md", "ANTIGRAVITY.md"]:
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
        if clean_content.startswith("---"):
            fm_end = clean_content.find("\n---", 3)
            if fm_end != -1:
                clean_content = clean_content[fm_end + 4:].strip()

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
        elif not title:
            fm_name_match = re.search(r'^name:\s*(.+)$', content, re.MULTILINE)
            if fm_name_match:
                title = fm_name_match.group(1).strip().replace("-", " ").title()
            else:
                title = os.path.splitext(base_name)[0].replace("_", " ").replace("-", " ").title()

        snippet = " ".join(" ".join(lines[:15]).split())[:200]
        cat_name, cat_key = get_category_info(rel_path)

        doc_entries.append({
            "doc": doc,
            "rel_path": rel_path,
            "clean_md_rel": clean_md_rel,
            "html_rel": html_rel,
            "title": title,
            "content": content,
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
        ("Kernel Subsystems & Go Packages", "subsystems"),
        ("Kernel DNA & Object Schemas", "schemas"),
        ("Reference Manuals", "manual"),
        ("How-To & Incident Runbooks", "operations"),
        ("Tutorials, Demos & Guides", "tutorials"),
        ("Agent Skills & Protocols", "skills"),
        ("Agent Directives & Packs", "agent-directives"),
        ("Maintenance & Development", "development"),
        ("Quality & Evaluation", "quality"),
        ("Codebase Evaluation Framework", "codebase-eval"),
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
            is_open = contains_active or (cat_key in ("getting-started", "architecture", "specs", "manual", "operations", "skills", "agent-directives", "tutorials", "subsystems", "schemas") and current_html_rel == "index.html")
            open_attr = ' open' if is_open else ''

            nav_html.append(f'<details class="sidebar-group"{open_attr}>')
            nav_html.append(f'<summary><span class="group-title">{html.escape(cat_name)}</span><span class="group-count">{len(items)}</span></summary>')
            nav_html.append('<ul>')
            for it in items:
                is_active = (it["html_rel"] == current_html_rel)
                active_cls = ' class="active"' if is_active else ''
                nav_title = html.escape(get_clean_nav_title(it["title"]))
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
        
        rendered_body = render_markdown_to_html(item["content"], item["html_rel"], link_map, repo_root, portal_build_id)
        escaped_title = html.escape(item["title"])
        
        depth = item["html_rel"].count("/")
        root_rel = "../" * depth if depth > 0 else ""
        sidebar_nav_html = get_sidebar_nav_html(root_rel, item["html_rel"])
        
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
        preview_links = "".join([f'<li><a href="{it["html_rel"]}">{html.escape(get_clean_nav_title(it["title"]))}</a></li>' for it in items[:4]])
        if len(items) > 4:
            preview_links += f'<li class="more-link"><a href="{first_doc["html_rel"]}">+ {len(items)-4} more guides &rarr;</a></li>'
        cat_cards_html += f"""
        <div class="cat-card">
          <div class="cat-card-header">
            <h4><a href="{first_doc['html_rel']}">{html.escape(cat_name)}</a></h4>
            <span class="cat-card-count">{len(items)} articles</span>
          </div>
          <p>Authoritative open-core specifications and guides for {html.escape(cat_name.lower())}.</p>
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
            <li><a href="docs/manual/ZPARQL_QUERY_LANGUAGE.html">ZPARQL Query Language Manual</a></li>
            <li><a href="docs/manual/ZQL_MUTATIONS.html">ZQL Declarative Mutations Manual</a></li>
            <li><a href="docs/manual/OBJECT_INSPECTOR_AND_POLICY_STUDIO.html">Object Inspector & Policy Studio</a></li>
            <li><a href="docs/guides/ZQL_ZPARQL_AGENT_GUIDE.html">ZQL & ZPARQL Agent Guide</a></li>
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
            <li><a href="docs/eval/README.html">Multi-Axis Benchmark Synthesis</a></li>
            <li><a href="docs/quality/codebase_evaluation/README.html">CEF Multi-Agent Evaluation Framework</a></li>
            <li><a href="docs/quality/codebase_evaluation/CONSTITUTION.html">CEF Evaluation Constitution</a></li>
            <li><a href="docs/quality/codebase_evaluation/DIAMOND_SCALE.html">Diamond Scale Multi-Axis Quality</a></li>
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
          <h3>📦 Kernel Subsystems & Packages</h3>
          <p>Architectural design, interface contracts, and storage implementations across the Go microkernel.</p>
          <ul>
            <li><a href="pkg/storage/README.html">Storage Subsystem & Providers</a></li>
            <li><a href="pkg/graph/README.html">Graph Backend & MemGraph Provider</a></li>
            <li><a href="pkg/mcp/README.html">Model Context Protocol (MCP) Server</a></li>
            <li><a href="pkg/concurrency/README.html">Concurrency & Synchronization</a></li>
            <li><a href="pkg/pipeline/README.html">Pipeline & Step Execution</a></li>
            <li><a href="internal/bootstrap/README.html">Bootstrap Archive & Seeding</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>🤖 Agent Operating Protocols</h3>
          <p>Directives, MCP integration, persona seating, and continuous autonomous loop discipline.</p>
          <ul>
            <li><a href="docs/onboarding/AI_AGENT_ONBOARDING.html">AI Agent Directives & Seating</a></li>
            <li><a href="docs/architecture/AMBIENT_SIGNAL_ACTION_RUBRIC.html">Ambient Signal Action Rubric</a></li>
            <li><a href="docs/quality/README.html">Verification Done-Gates (VDS)</a></li>
            <li><a href="docs/guides/ZQL_ZPARQL_AGENT_GUIDE.html">ZQL & ZPARQL Agent Guide</a></li>
            <li><a href="docs/onboarding/EDGE_HEADLESS_FIRST_RUN.html">Edge / Headless Mode</a></li>
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
