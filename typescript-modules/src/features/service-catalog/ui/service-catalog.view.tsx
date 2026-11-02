"use client";

import React, { useState } from "react";
import { ServiceCatalogState } from "../state/service-catalog.state";
import { ServiceCatalogController } from "../controller/service-catalog.controller";
import { ServiceDescriptorModel } from "../models/service-catalog.model";

export interface ServiceCatalogViewProps {
  state: ServiceCatalogState;
  controller: ServiceCatalogController;
  token: string;
}

export function ServiceCatalogViewComponent({ state, controller, token }: ServiceCatalogViewProps) {
  const [newId, setNewId] = useState("");
  const [newName, setNewName] = useState("");
  const [newOwner, setNewOwner] = useState("");
  const [newLifecycle, setNewLifecycle] = useState<"production" | "staging" | "experimental" | "deprecated">("production");

  const filtered = state.services.filter(s => {
    const matchKeyword = !state.filterKeyword ||
      s.name.toLowerCase().includes(state.filterKeyword.toLowerCase()) ||
      s.id.toLowerCase().includes(state.filterKeyword.toLowerCase());
    const matchLifecycle = state.filterLifecycle === "all" || s.lifecycle === state.filterLifecycle;
    return matchKeyword && matchLifecycle;
  });

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newId || !newName || !newOwner) return;
    await controller.registerNewService({
      id: newId,
      tenantId: "tenant-fintech",
      name: newName,
      owner: newOwner,
      lifecycle: newLifecycle
    }, token);
    setNewId("");
    setNewName("");
    setNewOwner("");
  };

  return (
    <section className="card">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "16px" }}>
        <h2 style={{ fontSize: "18px", fontWeight: 600 }}>Enterprise Service Catalog</h2>
        <div style={{ display: "flex", gap: "8px" }}>
          <input
            type="text"
            placeholder="Filter services..."
            value={state.filterKeyword}
            onChange={(e) => controller.setFilter(e.target.value, state.filterLifecycle)}
            style={{
              background: "var(--bg-canvas)",
              border: "1px solid var(--border-subtle)",
              color: "var(--text-primary)",
              padding: "6px 12px",
              borderRadius: "4px"
            }}
          />
          <button
            type="button"
            className="btn-primary"
            onClick={() => controller.fetchCatalog(token)}
          >
            Refresh
          </button>
        </div>
      </div>

      {state.isLoading && (
        <div style={{ padding: "24px", textAlign: "center", color: "var(--text-secondary)" }}>
          Loading catalog records from platform API...
        </div>
      )}

      {state.hasError && (
        <div style={{ padding: "16px", background: "rgba(239, 68, 68, 0.1)", border: "1px solid rgba(239, 68, 68, 0.3)", borderRadius: "6px", color: "var(--color-danger)" }}>
          Error: {state.errorMessage}
        </div>
      )}

      {!state.isLoading && !state.hasError && filtered.length === 0 && (
        <div style={{ padding: "24px", textAlign: "center", color: "var(--text-muted)" }}>
          No services registered in this tenant. Register your first service descriptor below.
        </div>
      )}

      {!state.isLoading && filtered.length > 0 && (
        <div className="table-responsive">
          <table>
            <thead>
              <tr>
                <th>Service ID</th>
                <th>Name</th>
                <th>Tenant</th>
                <th>Owner</th>
                <th>Lifecycle</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((s: ServiceDescriptorModel) => (
                <tr key={s.id}>
                  <td className="font-mono">{s.id}</td>
                  <td className="font-medium">{s.name}</td>
                  <td>{s.tenantId}</td>
                  <td>{s.owner}</td>
                  <td>
                    <span className={`badge badge-${s.lifecycle}`}>{s.lifecycle}</span>
                  </td>
                  <td>{s.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div style={{ marginTop: "32px", borderTop: "1px solid var(--border-subtle)", paddingTop: "20px" }}>
        <h3 style={{ fontSize: "15px", marginBottom: "12px", fontWeight: 600 }}>Register Service Descriptor</h3>
        <form onSubmit={handleRegister} style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr 140px auto", gap: "12px", alignItems: "center" }}>
          <input
            type="text"
            placeholder="Service ID (slug)"
            required
            value={newId}
            onChange={(e) => setNewId(e.target.value)}
            style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
          />
          <input
            type="text"
            placeholder="Display Name"
            required
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
          />
          <input
            type="email"
            placeholder="Owner Email"
            required
            value={newOwner}
            onChange={(e) => setNewOwner(e.target.value)}
            style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
          />
          <select
            value={newLifecycle}
            onChange={(e) => setNewLifecycle(e.target.value as any)}
            style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
          >
            <option value="production">production</option>
            <option value="staging">staging</option>
            <option value="experimental">experimental</option>
            <option value="deprecated">deprecated</option>
          </select>
          <button type="submit" className="btn-primary" style={{ padding: "8px 20px" }}>
            Register
          </button>
        </form>
      </div>
    </section>
  );
}
