import { useEffect } from 'react';
import './App.css';

function App() {
  useEffect(() => {
    document.title = "ZQK OS | The Zen Quantum Kernel";
    const metaDesc = document.querySelector('meta[name="description"]');
    if (metaDesc) {
      metaDesc.setAttribute("content", "ZQK OS is an operating system for AI + human hybrid teams.");
    } else {
      const meta = document.createElement('meta');
      meta.name = "description";
      meta.content = "ZQK OS is an operating system for AI + human hybrid teams.";
      document.head.appendChild(meta);
    }
  }, []);

  return (
    <div className="dashboard-layout">
      <main className="main-content" style={{ padding: '40px', maxWidth: '800px', margin: '0 auto', textAlign: 'center' }}>
        <header className="header animate-fade-in stagger-2" style={{ flexDirection: 'column', alignItems: 'center', gap: '20px' }}>
          <div>
            <h1 id="page-title">ZQK OS</h1>
            <p style={{ color: 'var(--text-muted)', fontSize: '1.2rem' }}>What zqk is: an operating system for AI + human hybrid teams.</p>
          </div>
        </header>

        <section className="glass-panel animate-fade-in stagger-3" style={{ marginTop: '40px' }}>
          <h2>Get Started</h2>
          <p style={{ color: 'var(--text-muted)', marginBottom: '20px' }}>Install path (CTA):</p>
          <div style={{ background: '#1e1e1e', padding: '16px', borderRadius: '8px', fontFamily: 'monospace', color: '#00ffcc', display: 'inline-block' }}>
            curl -sSL https://zqk.dev/install.sh | bash
          </div>
        </section>

        <section className="metrics-grid" style={{ marginTop: '40px' }}>
          <div className="glass-panel animate-fade-in stagger-3" style={{ animationDelay: '0.4s' }}>
            <div className="metric-label" style={{ fontSize: '1.2rem', marginBottom: '10px' }}>5-minute value</div>
            <div style={{ color: 'var(--text-muted)', fontSize: '0.9rem' }}>
              Install ZQK and realize value in less than 5 minutes. Orchestrate tasks instantly.
            </div>
          </div>
          
          <div className="glass-panel animate-fade-in stagger-3" style={{ animationDelay: '0.5s' }}>
            <div className="metric-label" style={{ fontSize: '1.2rem', marginBottom: '10px' }}>Need Help?</div>
            <div style={{ color: 'var(--text-muted)', fontSize: '0.9rem' }}>
              Check out our <a href="https://docs.zqk.dev" style={{ color: 'var(--accent-cyan)' }}>documentation</a> or join the community Discord.
            </div>
          </div>
        </section>
      </main>
    </div>
  );
}

export default App;
