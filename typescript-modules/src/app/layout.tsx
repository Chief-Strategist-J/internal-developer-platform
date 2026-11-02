import type { Metadata } from "next";
import React from "react";

export const metadata: Metadata = {
  title: "Internal Developer Platform | Developer Portal",
  description: "Enterprise self-service engineering portal and service catalog"
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <head>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <style dangerouslySetInnerHTML={{ __html: `
          :root {
            --bg-canvas: #090d16;
            --bg-surface: #111726;
            --bg-surface-elevated: #1b2438;
            --border-subtle: #24304c;
            --text-primary: #f0f4fc;
            --text-secondary: #94a3b8;
            --text-muted: #64748b;
            --color-primary: #3b82f6;
            --color-primary-hover: #2563eb;
            --color-success: #10b981;
            --color-warning: #f59e0b;
            --color-danger: #ef4444;
            --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            --font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
          }
          * { box-sizing: border-box; margin: 0; padding: 0; }
          body {
            background-color: var(--bg-canvas);
            color: var(--text-primary);
            font-family: var(--font-sans);
            line-height: 1.5;
            -webkit-font-smoothing: antialiased;
          }
          .font-mono { font-family: var(--font-mono); }
          .font-medium { font-weight: 500; }
          .container { max-width: 1200px; margin: 0 auto; padding: 32px 24px; }
          .header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 32px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 24px; }
          .brand-title { font-size: 24px; font-weight: 700; color: var(--text-primary); }
          .brand-subtitle { font-size: 14px; color: var(--text-secondary); margin-top: 4px; }
          .card { background: var(--bg-surface); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 24px; margin-bottom: 24px; }
          .btn-primary { background: var(--color-primary); color: #fff; border: none; padding: 8px 16px; border-radius: 6px; font-weight: 500; cursor: pointer; }
          .btn-primary:hover { background: var(--color-primary-hover); }
          .badge { padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; text-transform: uppercase; }
          .badge-production { background: rgba(16, 185, 129, 0.2); color: var(--color-success); border: 1px solid rgba(16, 185, 129, 0.4); }
          .badge-staging { background: rgba(245, 158, 11, 0.2); color: var(--color-warning); border: 1px solid rgba(245, 158, 11, 0.4); }
          .badge-experimental { background: rgba(59, 130, 246, 0.2); color: var(--color-primary); border: 1px solid rgba(59, 130, 246, 0.4); }
          .table-responsive { overflow-x: auto; margin-top: 16px; }
          table { width: 100%; border-collapse: collapse; text-align: left; }
          th { padding: 12px 16px; border-bottom: 1px solid var(--border-subtle); color: var(--text-secondary); font-size: 13px; text-transform: uppercase; font-weight: 600; }
          td { padding: 12px 16px; border-bottom: 1px solid var(--border-subtle); font-size: 14px; }
          tr:hover { background: var(--bg-surface-elevated); }
        `}} />
      </head>
      <body>
        <main className="container">
          <header className="header">
            <div>
              <h1 className="brand-title">Internal Developer Platform</h1>
              <p className="brand-subtitle">Automated Golden Paths, Service Catalog & Fine-Grained Policy Control</p>
            </div>
            <div className="font-mono text-secondary" style={{ fontSize: "13px" }}>
              API: v1 | Ready
            </div>
          </header>
          {children}
        </main>
      </body>
    </html>
  );
}
