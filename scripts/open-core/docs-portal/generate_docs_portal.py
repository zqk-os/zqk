#!/usr/bin/env python3
"""
generate_docs_portal.py — Fast static documentation site generator for ZQK Core.
Generates an offline-browsable, beautiful static documentation portal strictly
from the canonical public open-core documentation base.
"""

import os
import sys
import glob
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
    if rel_md_path.endswith(".md"):
        return rel_md_path[:-3] + ".html"
    elif not rel_md_path.endswith(".html"):
        return rel_md_path + ".html"
    return rel_md_path

def get_category_info(rel_path: str):
    parts = rel_path.split(os.sep)
    if parts[0] != "docs":
        return "Core & Governance", "core"
    if len(parts) == 2:
        return "Getting Started", "getting-started"
    sub = parts[1]
    category_map = {
        "onboarding": ("Onboarding & First-Run", "onboarding"),
        "architecture": ("Architecture & Foundation", "architecture"),
        "howto": ("How-To Guides", "howto"),
        "tutorials": ("Tutorials", "tutorials"),
        "manual": ("Reference Manual", "manual"),
        "explanation": ("Explanation & Philosophy", "explanation"),
        "enforcement": ("Enforcement & Policies", "enforcement"),
        "planning": ("Product Planning", "planning"),
        "quality": ("Quality & Evaluation", "quality"),
    }
    return category_map.get(sub, (sub.replace("_", " ").title(), sub))

def render_markdown_to_html(content: str, current_html_rel: str, link_map: dict) -> str:
    current_dir = os.path.dirname(current_html_rel)

    # Rewrite markdown links to generated html paths
    def replace_md_link(match):
        prefix = match.group(1)
        target = match.group(2)
        
        # Don't touch external or anchor-only links
        if target.startswith("http://") or target.startswith("https://") or target.startswith("mailto:"):
            return match.group(0)
        if target.startswith("#"):
            return match.group(0)

        anchor = ""
        if "#" in target:
            target, anchor = target.split("#", 1)
            anchor = "#" + anchor

        target_norm = os.path.normpath(target)
        target_base = os.path.basename(target)

        target_html_rel = None
        if target in link_map:
            target_html_rel = link_map[target]
        elif target_norm in link_map:
            target_html_rel = link_map[target_norm]
        elif target_base in link_map:
            target_html_rel = link_map[target_base]
        elif target.endswith(".md"):
            target_html_rel = get_html_relpath(target)

        if target_html_rel:
            if current_dir:
                rel_url = os.path.relpath(target_html_rel, current_dir)
            else:
                rel_url = target_html_rel
            return f"{prefix}({rel_url}{anchor})"

        return match.group(0)

    transformed = re.sub(r'(\[[^\]]+\])\(([^)]+)\)', replace_md_link, content)
    
    if HAS_MARKDOWN:
        try:
            rendered = markdown.markdown(
                transformed,
                extensions=['fenced_code', 'tables', 'toc', 'sane_lists']
            )
            # Transform fenced mermaid code blocks to <pre class="mermaid">
            # markdown fenced_code generates: <pre><code class="language-mermaid">...</code></pre>
            rendered = re.sub(
                r'<pre><code class="(?:language-)?mermaid">([\s\S]*?)</code></pre>',
                r'<pre class="mermaid">\1</pre>',
                rendered
            )
            return rendered
        except Exception:
            pass
            
    return f"<pre class=\"markdown-preview\">{html.escape(content)}</pre>"

def build_portal(repo_root: str, target_dir: str):
    print(f"📚 Generating ZQK Core Documentation Portal into {target_dir}...")
    
    if os.path.exists(target_dir):
        shutil.rmtree(target_dir)
    
    assets_dir = os.path.join(target_dir, "assets")
    search_dir = os.path.join(target_dir, "search")
    os.makedirs(assets_dir, exist_ok=True)
    os.makedirs(search_dir, exist_ok=True)

    # Write authentic ZQK logo asset
    with open(os.path.join(assets_dir, "zqk-logo.svg"), "w", encoding="utf-8") as f:
        f.write(ZQK_LOGO_SVG.strip() + "\n")

    # Collect documentation files strictly from Core, excluding archive/internal dirs
    raw_doc_files = sorted(glob.glob(os.path.join(repo_root, "docs", "**", "*.md"), recursive=True))
    doc_files = []
    exclude_parts = {"archive", "_archive", "_archive-cef-runs", "cef-runs", ".zqk", "audit"}
    for df in raw_doc_files:
        rel = os.path.relpath(df, repo_root)
        parts = set(rel.split(os.sep))
        if parts.intersection(exclude_parts):
            continue
        doc_files.append(df)

    for root_doc in ["README.md", "CONTRIBUTING.md", "SECURITY.md", "GOVERNANCE.md"]:
        p = os.path.join(repo_root, root_doc)
        if os.path.isfile(p):
            doc_files.append(p)

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
        
        link_map[rel_path] = html_rel
        link_map["./" + rel_path] = html_rel
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

        lines = [line.strip() for line in content.splitlines() if line.strip()]
        title = ""
        for line in lines[:15]:
            if line.startswith("# "):
                title = line[2:].strip()
                break
        if not title:
            title = os.path.splitext(base_name)[0].replace("_", " ").replace("-", " ").title()

        snippet = " ".join(" ".join(lines[:15]).split())[:200]
        cat_name, cat_key = get_category_info(rel_path)
        
        doc_entries.append({
            "doc": doc,
            "rel_path": rel_path,
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

    pages = []
    category_counts = {}

    def get_sidebar_nav_html(root_rel: str) -> str:
        return f"""
      <nav>
        <div class="sidebar-section">
          <h3>Getting Started</h3>
          <ul>
            <li><a href="{root_rel}index.html">Overview</a></li>
            <li><a href="{root_rel}docs/INDEX.html">Core Docs Index</a></li>
            <li><a href="{root_rel}docs/onboarding/COMMUNITY_FIRST_RUN.html">Community First-Run</a></li>
            <li><a href="{root_rel}docs/onboarding/QUICKSTART.html">Quickstart & MCP</a></li>
            <li><a href="{root_rel}docs/onboarding/AI_AGENT_ONBOARDING.html">AI Agent Directives</a></li>
            <li><a href="{root_rel}docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.html">First-Run Object Tutorial</a></li>
          </ul>
        </div>
        <div class="sidebar-section">
          <h3>Core Architecture</h3>
          <ul>
            <li><a href="{root_rel}docs/architecture/README.html">Architecture Overview</a></li>
            <li><a href="{root_rel}docs/architecture/CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.html">Cellular Membrane Mode B</a></li>
            <li><a href="{root_rel}docs/architecture/CLI_COMMAND_TAXONOMY_STANDARDS.html">CLI Command Taxonomy</a></li>
            <li><a href="{root_rel}docs/architecture/TIERED_STORAGE_AND_ARCHIVAL_LIFECYCLE.html">Tiered Storage Lifecycle</a></li>
            <li><a href="{root_rel}docs/architecture/TRAY_COMMAND_CRYPTOGRAPHIC_SECURITY.html">Tray Cryptographic Security</a></li>
          </ul>
        </div>
        <div class="sidebar-section">
          <h3>Operations & Guides</h3>
          <ul>
            <li><a href="{root_rel}docs/howto/README.html">How-To Overview</a></li>
            <li><a href="{root_rel}docs/howto/SCHEDULER_AND_MAINTENANCE.html">Scheduler & Maintenance</a></li>
            <li><a href="{root_rel}docs/onboarding/EDGE_HEADLESS_FIRST_RUN.html">Edge / Headless Mode</a></li>
            <li><a href="{root_rel}docs/quality/README.html">Quality & Verification Gates</a></li>
          </ul>
        </div>
        <div class="sidebar-section">
          <h3>Open Core Governance</h3>
          <ul>
            <li><a href="{root_rel}CONTRIBUTING.html">Contributing & DCO</a></li>
            <li><a href="{root_rel}SECURITY.html">Security Policy</a></li>
            <li><a href="{root_rel}GOVERNANCE.html">Open-Core Governance</a></li>
          </ul>
        </div>
      </nav>
        """

    for item in doc_entries:
        cat_name = item["category"]
        category_counts[cat_name] = category_counts.get(cat_name, 0) + 1
        
        rendered_body = render_markdown_to_html(item["content"], item["html_rel"], link_map)
        escaped_title = html.escape(item["title"])
        
        depth = item["html_rel"].count("/")
        root_rel = "../" * depth if depth > 0 else ""
        sidebar_nav_html = get_sidebar_nav_html(root_rel)
        
        page_html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{escaped_title} - ZQK Core Documentation</title>
  <link rel="stylesheet" href="{root_rel}assets/style.css">
  <link rel="icon" type="image/svg+xml" href="{root_rel}assets/zqk-logo.svg">
  <script type="module">
    import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
    mermaid.initialize({{
      startOnLoad: true,
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
        <a href="{root_rel}{item['rel_path']}" class="raw-md-link" title="View canonical Markdown source">Raw .md</a>
      </div>
      <div class="markdown-body">
        {rendered_body}
      </div>
    </article>
  </main>
  <script src="{root_rel}search/search-index.js"></script>
  <script>
    document.addEventListener('DOMContentLoaded', () => {{
      document.querySelectorAll('pre code.language-mermaid').forEach(el => {{
        const pre = el.parentElement;
        pre.className = 'mermaid';
        pre.textContent = el.textContent;
      }});
    }});
  </script>
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
    for cname, count in sorted(category_counts.items(), key=lambda x: -x[1]):
        cat_cards_html += f"""
        <div class="cat-card">
          <div class="cat-card-header">
            <h4>{html.escape(cname)}</h4>
            <span class="cat-card-count">{count} articles</span>
          </div>
          <p>Authoritative open-core specifications and guides for {html.escape(cname.lower())}.</p>
        </div>
        """

    index_sidebar_html = get_sidebar_nav_html("")
    index_html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>ZQK Core Documentation Portal</title>
  <link rel="stylesheet" href="assets/style.css">
  <link rel="icon" type="image/svg+xml" href="assets/zqk-logo.svg">
  <script type="module">
    import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
    mermaid.initialize({{
      startOnLoad: true,
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
            <li><a href="docs/architecture/CELLULAR_MEMBRANE_MODE_B_CONFIGURATION.html">Cellular Membrane Mode B Runbook</a></li>
            <li><a href="docs/architecture/CLI_COMMAND_TAXONOMY_STANDARDS.html">CLI Command Taxonomy & Standards</a></li>
            <li><a href="docs/architecture/TIERED_STORAGE_AND_ARCHIVAL_LIFECYCLE.html">Tiered Storage & Capsule Archival</a></li>
            <li><a href="docs/architecture/TRAY_COMMAND_CRYPTOGRAPHIC_SECURITY.html">Tray Cryptographic Security</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>🛠️ How-To & Operations</h3>
          <p>Operational task recipes for running daemons and background organisms.</p>
          <ul>
            <li><a href="docs/howto/README.html">How-To Guides Overview</a></li>
            <li><a href="docs/howto/SCHEDULER_AND_MAINTENANCE.html">Scheduler & Maintenance Jobs Guide</a></li>
            <li><a href="docs/quality/README.html">First-Run Quality & Verification Gates</a></li>
          </ul>
        </div>
        <div class="quad-box">
          <h3>⚖️ Governance & Repository</h3>
          <p>Open-source policies, contributing guidelines, and security disclosures.</p>
          <ul>
            <li><a href="CONTRIBUTING.html">Contributing & DCO Compliance</a></li>
            <li><a href="SECURITY.html">Security Vulnerability Disclosures</a></li>
            <li><a href="GOVERNANCE.html">Open-Core Decision Making & Boundaries</a></li>
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
  <script src="search/search-index.js"></script>
</body>
</html>
"""
    with open(os.path.join(target_dir, "index.html"), "w", encoding="utf-8") as f:
        f.write(index_html)

    # Write CSS
    style_css = """
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
  width: 280px;
  flex-shrink: 0;
  border-right: 1px solid var(--border-color);
  padding-right: 1.5rem;
  max-height: calc(100vh - 100px);
  position: sticky;
  top: 70px;
  overflow-y: auto;
}

.sidebar-section {
  margin-bottom: 1.75rem;
}

.sidebar-section h3 {
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-muted);
  margin-bottom: 0.5rem;
}

.sidebar ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.sidebar li {
  margin-bottom: 0.4rem;
}

.sidebar a {
  color: var(--text-main);
  text-decoration: none;
  font-size: 0.9rem;
  display: block;
  padding: 4px 8px;
  border-radius: 4px;
  transition: all 0.15s ease;
}

.sidebar a:hover {
  background: var(--bg-tertiary);
  color: var(--accent-cyan);
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

/* Mermaid Architectural Diagram Styling */
.mermaid {
  display: flex !important;
  justify-content: center !important;
  align-items: center !important;
  background: var(--bg-secondary) !important;
  border: 1px solid var(--border-color) !important;
  border-radius: 8px !important;
  padding: 1.75rem 1rem !important;
  margin: 1.75rem 0 !important;
  overflow-x: auto !important;
}

.mermaid svg {
  max-width: 100% !important;
  height: auto !important;
}
"""
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
