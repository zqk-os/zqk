import { useState, useEffect } from 'react';
import './App.css';

interface TerminalTab {
  id: string;
  label: string;
  command: string;
  output: string;
}

function App() {
  const [activeTab, setActiveTab] = useState<string>('install');
  const [copied, setCopied] = useState<boolean>(false);

  useEffect(() => {
    document.title = "ZQK OS | The Cellular Knowledge Operating System";
    const metaDesc = document.querySelector('meta[name="description"]');
    const descContent = "ZQK OS is a Cellular Knowledge Operating System for autonomous AI agent swarms and human hybrid teams.";
    if (metaDesc) {
      metaDesc.setAttribute("content", descContent);
    } else {
      const meta = document.createElement('meta');
      meta.name = "description";
      meta.content = descContent;
      document.head.appendChild(meta);
    }
  }, []);

  const terminalTabs: Record<string, TerminalTab> = {
    install: {
      id: 'install',
      label: '1. Install',
      command: 'curl -sSL https://zqk.dev/install.sh | bash',
      output: `==> Downloading zqk-core binary for darwin-arm64...
==> Verifying SHA256 cryptographic attestation... [OK]
==> Installed to /usr/local/bin/zqk (v3.0.0-community)
==> Run 'zqk system init' to seed a sovereign cellular kernel.`
    },
    init: {
      id: 'init',
      label: '2. Seed Cell',
      command: 'zqk system init --project-name core-service',
      output: `[zqk:cell] Initializing sovereign cellular node: core-service
[zqk:membrane] Establishing deterministic state boundary (.zqk/)
[zqk:dna] Minting standard library: Intent, WorkUnit, InvariantGate, ADR
[zqk:gantt] Seeded root objective graph: org -> mission -> vision -> plan
[zqk:ready] Kernel cell alive. Memory plane: Draft | Staged | Promoted`
    },
    onboard: {
      id: 'onboard',
      label: '3. Seat Agent',
      command: 'zqk system agent-onboard --format json',
      output: `{
  "agent_host": "cursor",
  "seating_status": "active",
  "cell_urn": "urn:zqk:cell-01:agent:claude-sonnet-3-7",
  "membrane_policy": "fail-closed",
  "directives_primed": [".cursor/rules/zqk-kernel.mdc", "AGENTS.md"]
}`
    },
    mcp: {
      id: 'mcp',
      label: '4. Connect MCP',
      command: 'zqk mcp install && zqk workflow whats-next',
      output: `[mcp] Registered ZQK Knowledge Kernel MCP server with local IDEs.
[workflow] Querying real-time kernel nervous system...
Priority Plan: PRI-AUTH-FEDERATION-001 (active)
Available WorkUnits:
  - BLI-104: Implement JWT invariant gate verification [planned]
  - BLI-105: Wire apoptotic timeout on stale lease [claimed]
Next Agentic Action: Claim BLI-104 and enter PlaneDraft.`
    }
  };

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div>
      {/* Navigation */}
      <header className="site-nav">
        <div className="site-container site-nav-inner">
          <div className="logo-brand">
            <span>⚡ ZQK OS</span>
          </div>
          <nav className="nav-links">
            <a href="#cellular-model">Cellular Architecture</a>
            <a href="#comparison">Why Cellular OS</a>
            <a href="#open-core">Open Core</a>
            <a href="https://docs.zqk.dev" target="_blank" rel="noreferrer">Documentation</a>
            <a href="https://github.com/zqk-os/zqk" target="_blank" rel="noreferrer" className="btn-secondary" style={{ padding: '8px 16px', fontSize: '0.85rem' }}>
              GitHub ↗
            </a>
          </nav>
        </div>
      </header>

      {/* Hero Section */}
      <section className="hero-section">
        <div className="site-container">
          <div className="hero-pill">
            <span>Cellular Knowledge Operating System</span>
          </div>
          <h1 className="hero-title" id="page-title">
            The Operating System for<br />Autonomous Agent Swarms
          </h1>
          <p className="hero-subtitle">
            <span className="hero-highlight">Your AI agents are coding blind.</span> Current frameworks glue loose prompt scripts to chaotic vector swamps. ZQK OS provides <strong>cellular isolation</strong>, a <strong>graph nervous system</strong>, and <strong>self-healing biological guardrails</strong> so agents govern themselves.
          </p>

          <p style={{ display: 'none' }}>
            What zqk is: an operating system for AI + human hybrid teams.
          </p>

          {/* Interactive Shell */}
          <div className="terminal-window">
            <div className="terminal-header">
              <div className="terminal-dots">
                <span className="dot dot-red"></span>
                <span className="dot dot-yellow"></span>
                <span className="dot dot-green"></span>
              </div>
              <div style={{ display: 'flex', gap: '8px' }}>
                {Object.keys(terminalTabs).map((key) => (
                  <button
                    key={key}
                    onClick={() => setActiveTab(key)}
                    style={{
                      background: activeTab === key ? 'rgba(0, 229, 255, 0.15)' : 'transparent',
                      border: 'none',
                      color: activeTab === key ? 'var(--accent-cyan)' : 'var(--text-muted)',
                      padding: '4px 10px',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      fontSize: '0.8rem',
                      fontFamily: 'monospace'
                    }}
                  >
                    {terminalTabs[key].label}
                  </button>
                ))}
              </div>
              <span className="terminal-tab">zsh — local cell</span>
            </div>

            <div className="terminal-body">
              <div className="terminal-cmd-row">
                <div>
                  <span className="terminal-prompt">$</span>
                  <span style={{ color: '#fff' }}>{terminalTabs[activeTab].command}</span>
                </div>
                <button
                  className="copy-btn"
                  onClick={() => handleCopy(terminalTabs[activeTab].command)}
                >
                  {copied ? 'Copied!' : 'Copy'}
                </button>
              </div>
              <pre style={{ margin: 0, color: 'var(--text-muted)', whiteSpace: 'pre-wrap' }}>
                {terminalTabs[activeTab].output}
              </pre>
            </div>
          </div>

          <div style={{ marginTop: '28px', display: 'flex', gap: '16px', justifyContent: 'center' }}>
            <a href="#quickstart" className="btn-primary">
              Get Started (5-minute value) →
            </a>
            <a href="https://docs.zqk.dev" target="_blank" rel="noreferrer" className="btn-secondary">
              Read the Manifesto
            </a>
          </div>
        </div>
      </section>

      {/* Cellular Biology 4 Pillars */}
      <section className="section" id="cellular-model" style={{ background: 'rgba(5, 8, 16, 0.4)' }}>
        <div className="site-container">
          <div className="section-header">
            <div className="section-tag">Biological Systems Architecture</div>
            <h2 className="section-title">The Cellular Computing Substrate</h2>
            <p className="section-desc">
              Swarms cannot scale on monolithic memory dumps or unconstrained file edits. ZQK structures autonomous computing into natural biological boundaries.
            </p>
          </div>

          <div className="bio-grid">
            <div className="glass-panel bio-card">
              <div className="bio-icon" style={{ color: 'var(--accent-cyan)' }}>🧬</div>
              <div className="bio-meta cell">The Cell (Sovereign Node)</div>
              <h3 className="bio-title">Bounded Local Kernel</h3>
              <p className="bio-desc">
                Each kernel is a sovereign domain expert over its local codebase. No bloated shared context or uncurated exhaust—each cell enforces its own physical reality, specs, and local Git CAS integrity.
              </p>
            </div>

            <div className="glass-panel bio-card">
              <div className="bio-icon" style={{ color: 'var(--accent-magenta)' }}>🛡️</div>
              <div className="bio-meta membrane">The Membrane (Boundaries)</div>
              <h3 className="bio-title">Deterministic Planes</h3>
              <p className="bio-desc">
                Agents operate freely in <code>PlaneDraft</code> with zero blast radius. Only state passing strict <code>InvariantGate</code> vetting and cryptographic attestations can cross the membrane into <code>PlanePromoted</code>.
              </p>
            </div>

            <div className="glass-panel bio-card">
              <div className="bio-icon" style={{ color: 'var(--accent-amber)' }}>⚡</div>
              <div className="bio-meta nervous">The Nervous System (Graph)</div>
              <h3 className="bio-title">Operational State Bus</h3>
              <p className="bio-desc">
                Not a passive SPARQL/vector data dump. The ZQK knowledge graph is an active synaptic signaling network governing real-time task execution, causal provenance lineage, and dependency trees.
              </p>
            </div>

            <div className="glass-panel bio-card">
              <div className="bio-icon" style={{ color: 'var(--accent-emerald)' }}>🌐</div>
              <div className="bio-meta organism">The Organism (Peer Mesh)</div>
              <h3 className="bio-title">Inter-Cellular Symbiosis</h3>
              <p className="bio-desc">
                Independent domain-expert kernels negotiate over a typed wire protocol. Applications are not monolithic codebases; they are emergent, self-healing swarms executing compound objectives.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Comparison: Vector Swamp vs Cellular OS */}
      <section className="section" id="comparison">
        <div className="site-container">
          <div className="section-header">
            <div className="section-tag">First-Principles Paradigm Shift</div>
            <h2 className="section-title">The Vector Swamp vs. Epistemic Hygiene</h2>
            <p className="section-desc">
              Why adding more agents to traditional frameworks accelerates hallucinations—and how cellular isolation guarantees convergence.
            </p>
          </div>

          <div className="comparison-container">
            <div className="comp-box negative">
              <h3 className="comp-title" style={{ color: '#f87171' }}>
                <span>✕</span> The Fragile Status Quo
              </h3>
              <div className="comp-item">
                <div className="comp-bullet bad">•</div>
                <div>
                  <div className="comp-text-title">The "Vector Swamp"</div>
                  <div className="comp-text-desc">Uncurated agent conversation logs dumped into vector databases. Cosine similarity guesses at truth, leading to catastrophic context rotting.</div>
                </div>
              </div>
              <div className="comp-item">
                <div className="comp-bullet bad">•</div>
                <div>
                  <div className="comp-text-title">Script Harnesses & Terminal Multiplexers</div>
                  <div className="comp-text-desc">Python scripts or tmux splits that treat agents as external black boxes without transactional state, memory boundaries, or rollbacks.</div>
                </div>
              </div>
              <div className="comp-item">
                <div className="comp-bullet bad">•</div>
                <div>
                  <div className="comp-text-title">Runaway Hallucination Blast Radius</div>
                  <div className="comp-text-desc">One hallucinating agent can directly corrupt code files, drop unverified commits, and derail the entire engineering swarm.</div>
                </div>
              </div>
            </div>

            <div className="comp-box positive">
              <h3 className="comp-title" style={{ color: 'var(--accent-cyan)' }}>
                <span>✓</span> The ZQK Cellular Architecture
              </h3>
              <div className="comp-item">
                <div className="comp-bullet good">•</div>
                <div>
                  <div className="comp-text-title">Epistemic Cellular Hygiene</div>
                  <div className="comp-text-desc">Every entity embeds causal lineage, actor attestation, and schema references. Only mathematically verified state is promoted to the graph.</div>
                </div>
              </div>
              <div className="comp-item">
                <div className="comp-bullet good">•</div>
                <div>
                  <div className="comp-text-title">Operating System Kernel Primitives</div>
                  <div className="comp-text-desc">First-class kernel objects, multi-plane memory isolation, and deterministic task scheduling built natively in high-performance Go.</div>
                </div>
              </div>
              <div className="comp-item">
                <div className="comp-bullet good">•</div>
                <div>
                  <div className="comp-text-title">Cellular Apoptosis & Self-Healing</div>
                  <div className="comp-text-desc">Failed invariant gates or drifting leases trigger automated quarantine (apoptosis). Failures are isolated to the single cell without infecting the organism.</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Open-Core Boundary Matrix */}
      <section className="section" id="open-core" style={{ background: 'rgba(5, 8, 16, 0.4)' }}>
        <div className="site-container">
          <div className="section-header">
            <div className="section-tag">Distribution Architecture</div>
            <h2 className="section-title">Open-Core Kernel vs. Enterprise Mesh</h2>
            <p className="section-desc">
              Following the classical operating system model: POSIX/Kernel primitives are 100% open-source; distributed multi-tenant mesh clustering and industrial governance are commercial.
            </p>
          </div>

          <div className="tier-grid">
            <div className="glass-panel tier-card">
              <div className="tier-header">
                <span className="tier-badge badge-open">Open-Source Community</span>
                <h3 style={{ fontSize: '1.6rem', marginBottom: '8px' }}>ZQK Core Kernel</h3>
                <p style={{ color: 'var(--text-muted)', fontSize: '0.95rem' }}>
                  Single-Node Sovereignty. Fully functional, zero-dependency autonomous kernel for developers and teams.
                </p>
              </div>
              <ul className="tier-features">
                <li><span>✓</span> Sovereign Go Microkernel & Local Git CAS</li>
                <li><span>✓</span> Multi-Plane Isolation (Draft → Staged → Promoted)</li>
                <li><span>✓</span> Model Context Protocol (MCP) Server & IDE Seating</li>
                <li><span>✓</span> Standard Core DNA Schemas (Intent, WorkUnit, Gates)</li>
                <li><span>✓</span> Deterministic Task Scheduler & Self-Healing Loops</li>
                <li><span>✓</span> Native AST Trigram Search (<code>zqk grep</code> / <code>zgrep</code>)</li>
              </ul>
              <a href="https://github.com/zqk-os/zqk" target="_blank" rel="noreferrer" className="btn-secondary" style={{ justifyContent: 'center' }}>
                Star on GitHub
              </a>
            </div>

            <div className="glass-panel tier-card" style={{ borderColor: 'rgba(99, 102, 241, 0.4)', boxShadow: '0 12px 40px rgba(99, 102, 241, 0.15)' }}>
              <div className="tier-header">
                <span className="tier-badge badge-enterprise">Commercial / Enterprise</span>
                <h3 style={{ fontSize: '1.6rem', marginBottom: '8px' }}>ZQK Enterprise Mesh</h3>
                <p style={{ color: 'var(--text-muted)', fontSize: '0.95rem' }}>
                  Organism Infrastructure. Fleet-wide coordination, multi-tenant directory, and industrial compliance.
                </p>
              </div>
              <ul className="tier-features">
                <li><span>✦</span> Multi-Node P2P Cellular Mesh & Peer Discovery</li>
                <li><span>✦</span> Cross-Cell Knowledge Sync & Semantic Drift Prevention</li>
                <li><span>✦</span> Fleet-Wide Apoptotic Quarantine & Distributed Rollback</li>
                <li><span>✦</span> Spec-Driven AST Synthesis & Automated Codegen Pipeline</li>
                <li><span>✦</span> High-Throughput Memgraph Replay & Lineage Sharding</li>
                <li><span>✦</span> Enterprise RBAC, Policy Enclaves & Compliance Auditing</li>
              </ul>
              <a href="mailto:contact@zqkos.com" className="btn-primary" style={{ justifyContent: 'center' }}>
                Talk to Engineering
              </a>
            </div>
          </div>
        </div>
      </section>

      {/* Quickstart / CTA */}
      <section className="section" id="quickstart">
        <div className="site-container" style={{ textAlign: 'center' }}>
          <div className="glass-panel" style={{ maxWidth: '800px', margin: '0 auto', padding: '48px' }}>
            <h2 style={{ fontSize: '2.2rem', marginBottom: '16px' }}>Ready to Run Living Software?</h2>
            <p style={{ color: 'var(--text-muted)', marginBottom: '32px', fontSize: '1.1rem' }}>
              Realize 5-minute value. Install the community kernel and connect Cursor, Claude Code, or Windsurf in less than 5 minutes.
            </p>
            <div style={{ background: '#0d1117', padding: '16px 24px', borderRadius: '8px', fontFamily: 'monospace', color: 'var(--accent-cyan)', display: 'inline-block', border: '1px solid rgba(0, 229, 255, 0.3)', marginBottom: '24px', fontSize: '1.05rem' }}>
              curl -sSL https://zqk.dev/install.sh | bash
            </div>
            <div>
              <p style={{ color: 'var(--text-muted)', fontSize: '0.9rem' }}>
                Need help? Check out our <a href="https://docs.zqk.dev" target="_blank" rel="noreferrer">documentation</a>, join our community Discord, or open a GitHub issue.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="site-footer">
        <div className="site-container">
          <div className="footer-grid">
            <div>
              <div className="logo-brand" style={{ marginBottom: '16px' }}>⚡ ZQK OS</div>
              <p style={{ color: 'var(--text-muted)', fontSize: '0.9rem', maxWidth: '320px' }}>
                The Cellular Knowledge Operating System for Autonomous Agent Swarms. Open-core, local-first, and biologically engineered for living software.
              </p>
            </div>
            <div>
              <h4 style={{ color: '#fff', marginBottom: '16px' }}>Architecture</h4>
              <p><a href="#cellular-model">Cellular Model</a></p>
              <p><a href="#comparison">Epistemic Hygiene</a></p>
              <p><a href="#open-core">Open Core Boundary</a></p>
              <p><a href="https://docs.zqk.dev/specs" target="_blank" rel="noreferrer">Kernel Specifications</a></p>
            </div>
            <div>
              <h4 style={{ color: '#fff', marginBottom: '16px' }}>Developers</h4>
              <p><a href="#quickstart">Quickstart (5 Min)</a></p>
              <p><a href="https://docs.zqk.dev" target="_blank" rel="noreferrer">Documentation</a></p>
              <p><a href="https://github.com/zqk-os/zqk" target="_blank" rel="noreferrer">GitHub Repository</a></p>
              <p><a href="https://modelcontextprotocol.io" target="_blank" rel="noreferrer">MCP Protocol</a></p>
            </div>
            <div>
              <h4 style={{ color: '#fff', marginBottom: '16px' }}>Community</h4>
              <p><a href="https://discord.gg/zqkos" target="_blank" rel="noreferrer">Discord Community</a></p>
              <p><a href="https://twitter.com/zqkos" target="_blank" rel="noreferrer">X / Twitter</a></p>
              <p><a href="mailto:community@zqkos.com">Contact Core Team</a></p>
            </div>
          </div>

          <div className="footer-bottom">
            <div>© {new Date().getFullYear()} ZQK OS Project. Apache 2.0 Open Core.</div>
            <div>Designed for Autonomous Living Software.</div>
          </div>
        </div>
      </footer>
    </div>
  );
}

export default App;
