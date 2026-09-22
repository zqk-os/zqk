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
import tarfile
import gzip
import shutil

try:
    import markdown
    HAS_MARKDOWN = True
except ImportError:
    HAS_MARKDOWN = False

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
            return markdown.markdown(
                transformed,
                extensions=['fenced_code', 'tables', 'toc', 'sane_lists']
            )
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

    link_map = {}
    doc_entries = []

    # First pass: Build link map and extract titles
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
            <li><a href="{root_rel}README.html">Kernel README</a></li>
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
  <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='6' fill='%2305070c'/%3E%3Cpath d='M7 10h18l-14 12h14' stroke='%2300e5ff' stroke-width='2.8' stroke-linecap='round' stroke-linejoin='round' fill='none'/%3E%3C/svg%3E">
</head>
<body data-root-rel="{root_rel}">
  <header class="header">
    <div class="nav-container">
      <div class="brand">
        <a href="{root_rel}index.html" class="logo">
          <svg class="logo-mark" viewBox="0 0 32 32" width="24" height="24">
            <rect width="32" height="32" rx="6" fill="#05070c"/>
            <path d="M7 10h18l-14 12h14" stroke="#00e5ff" stroke-width="2.8" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
          </svg>
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
        <a href="{root_rel}index.html">Docs</a> &raquo; <span>{html.escape(item["category"])}</span> &raquo; <span class="current">{escaped_title}</span>
      </div>
      <div class="markdown-body">
        {rendered_body}
      </div>
    </article>
  </main>
  <script src="{root_rel}search/search-index.js"></script>
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
  <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='6' fill='%2305070c'/%3E%3Cpath d='M7 10h18l-14 12h14' stroke='%2300e5ff' stroke-width='2.8' stroke-linecap='round' stroke-linejoin='round' fill='none'/%3E%3C/svg%3E">
</head>
<body data-root-rel="">
  <header class="header">
    <div class="nav-container">
      <div class="brand">
        <a href="index.html" class="logo">
          <svg class="logo-mark" viewBox="0 0 32 32" width="24" height="24">
            <rect width="32" height="32" rx="6" fill="#05070c"/>
            <path d="M7 10h18l-14 12h14" stroke="#00e5ff" stroke-width="2.8" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
          </svg>
          <span class="logo-text">ZQK <span class="logo-accent">Core</span></span>
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
        <h1>Zen Quantum Kernel (ZQK) Core Documentation</h1>
        <p class="hero-desc">
          The foundational, sovereign Knowledge Kernel for human-agent software engineering.
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
            <li><a href="README.html">Kernel README & Overview</a></li>
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
  --accent-green: #34d399;
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
  gap: 8px;
  font-size: 1.2rem;
  font-weight: 700;
  color: #fff;
  text-decoration: none;
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
  font-size: 0.85rem;
  color: var(--text-muted);
  margin-bottom: 1.5rem;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 0.75rem;
}

.breadcrumb a {
  color: var(--accent-blue);
  text-decoration: none;
}

.breadcrumb .current {
  color: var(--text-main);
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

def main():
    repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
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
