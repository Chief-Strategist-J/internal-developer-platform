"use client";

import React, { useState } from "react";
import { RoleBasedAccessControlState } from "../state/role-based-access-control.state";
import { RoleBasedAccessControlController } from "../controller/role-based-access-control.controller";

export interface RBACViewProps {
  state: RoleBasedAccessControlState;
  controller: RoleBasedAccessControlController;
}

export function RoleBasedAccessControlViewComponent({ state, controller }: RBACViewProps) {
  const [subjectId, setSubjectId] = useState("developer-sarah");
  const [tenantId, setTenantId] = useState("tenant-fintech");
  const [role, setRole] = useState("Developer");

  const [evalResource, setEvalResource] = useState("service-catalog");
  const [evalAction, setEvalAction] = useState("write");

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    await controller.login({
      subjectId,
      tenantId,
      roles: [role]
    });
  };

  const handleEvaluate = async (e: React.FormEvent) => {
    e.preventDefault();
    await controller.evaluateAccess(evalResource, evalAction);
  };

  return (
    <section className="card">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "16px" }}>
        <h2 style={{ fontSize: "18px", fontWeight: 600 }}>Access Control & Identity Context</h2>
        <span className={`badge ${state.token ? "badge-production" : "badge-experimental"}`}>
          {state.token ? "Authenticated" : "Unauthenticated"}
        </span>
      </div>

      {!state.token ? (
        <form onSubmit={handleLogin} style={{ display: "grid", gridTemplateColumns: "1fr 1fr 160px auto", gap: "12px", alignItems: "center" }}>
          <input
            type="text"
            placeholder="Subject ID"
            value={subjectId}
            onChange={(e) => setSubjectId(e.target.value)}
            style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
          />
          <input
            type="text"
            placeholder="Tenant ID"
            value={tenantId}
            onChange={(e) => setTenantId(e.target.value)}
            style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
          />
          <select
            value={role}
            onChange={(e) => setRole(e.target.value)}
            style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
          >
            <option value="Developer">Developer</option>
            <option value="PlatformAdmin">PlatformAdmin</option>
            <option value="Viewer">Viewer</option>
          </select>
          <button type="submit" className="btn-primary" style={{ padding: "8px 20px" }}>
            Issue Token
          </button>
        </form>
      ) : (
        <div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))", gap: "16px", marginBottom: "20px" }}>
            <div>
              <div style={{ fontSize: "12px", color: "var(--text-secondary)", textTransform: "uppercase" }}>Subject ID</div>
              <div className="font-mono" style={{ fontSize: "14px", marginTop: "4px" }}>{state.identity?.subjectId}</div>
            </div>
            <div>
              <div style={{ fontSize: "12px", color: "var(--text-secondary)", textTransform: "uppercase" }}>Tenant ID</div>
              <div style={{ fontSize: "14px", marginTop: "4px" }}>{state.identity?.tenantId}</div>
            </div>
            <div>
              <div style={{ fontSize: "12px", color: "var(--text-secondary)", textTransform: "uppercase" }}>Assigned Roles</div>
              <div style={{ marginTop: "4px" }}>
                {state.identity?.roles.map(r => (
                  <span key={r} className="badge badge-staging" style={{ marginRight: "4px" }}>{r}</span>
                ))}
              </div>
            </div>
          </div>

          <div style={{ borderTop: "1px solid var(--border-subtle)", paddingTop: "16px", marginTop: "16px" }}>
            <h3 style={{ fontSize: "14px", marginBottom: "12px", fontWeight: 600 }}>Policy Decision Point (PDP) Evaluator</h3>
            <form onSubmit={handleEvaluate} style={{ display: "flex", gap: "12px", alignItems: "center" }}>
              <select
                value={evalResource}
                onChange={(e) => setEvalResource(e.target.value)}
                style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
              >
                <option value="service-catalog">service-catalog</option>
                <option value="infrastructure">infrastructure</option>
                <option value="*">all (*)</option>
              </select>
              <select
                value={evalAction}
                onChange={(e) => setEvalAction(e.target.value)}
                style={{ background: "var(--bg-canvas)", border: "1px solid var(--border-subtle)", color: "var(--text-primary)", padding: "8px 12px", borderRadius: "4px" }}
              >
                <option value="read">read</option>
                <option value="write">write</option>
                <option value="admin">admin</option>
              </select>
              <button type="submit" className="btn-primary">
                Evaluate Permission
              </button>
            </form>

            {state.lastEvaluation && (
              <div style={{
                marginTop: "12px",
                padding: "10px 16px",
                borderRadius: "4px",
                background: state.lastEvaluation.granted ? "rgba(16, 185, 129, 0.1)" : "rgba(239, 68, 68, 0.1)",
                border: `1px solid ${state.lastEvaluation.granted ? "rgba(16, 185, 129, 0.3)" : "rgba(239, 68, 68, 0.3)"}`,
                color: state.lastEvaluation.granted ? "var(--color-success)" : "var(--color-danger)"
              }}>
                <strong>Decision: {state.lastEvaluation.granted ? "GRANTED" : "DENIED"}</strong> — {state.lastEvaluation.reason}
              </div>
            )}
          </div>

          <div style={{ borderTop: "1px solid var(--border-subtle)", paddingTop: "16px", marginTop: "16px" }}>
            <h3 style={{ fontSize: "14px", marginBottom: "12px", fontWeight: 600 }}>JIT Elevation & Governance</h3>
            <div style={{ display: "flex", gap: "12px", flexWrap: "wrap" }}>
              <button
                type="button"
                className="btn-primary"
                onClick={() => controller.requestJITAccess({
                  tenantId: state.identity?.tenantId ?? tenantId,
                  subjectId: state.identity?.subjectId ?? subjectId,
                  targetNamespace: "prod-core",
                  role: "PlatformAdmin",
                  reason: "incident break-glass elevation",
                  durationSeconds: 1800,
                  approverRole: "PlatformAdmin",
                  idempotencyKey: `jit-${Date.now()}`
                })}
              >
                Request JIT Grant
              </button>
              <button
                type="button"
                className="btn-primary"
                onClick={() => controller.validateRoleHierarchy({ PlatformAdmin: ["Developer"], Developer: ["Viewer"] })}
              >
                Validate Role DAG
              </button>
              <button
                type="button"
                className="btn-primary"
                onClick={() => controller.analyzeLeastPrivilege({
                  roles: { Developer: [{ resource: "service-catalog", action: "read" }] },
                  usage: [],
                  staleWindowHours: 24
                })}
              >
                Analyze Least Privilege
              </button>
            </div>
            {state.activeGrants.length > 0 && (
              <ul style={{ marginTop: "12px", paddingLeft: "16px" }}>
                {state.activeGrants.map((grant) => (
                  <li key={grant.id} style={{ marginBottom: "6px" }}>
                    <span>{grant.role} ({grant.targetNamespace}) — {grant.status}</span>
                    <button
                      type="button"
                      style={{ marginLeft: "10px" }}
                      onClick={() => controller.revokeJITAccess(grant.id)}
                    >
                      Revoke
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {state.hierarchyValid !== null && (
              <div style={{ marginTop: "8px", fontSize: "13px" }}>
                Role Hierarchy DAG: {state.hierarchyValid ? "Valid" : "Invalid"}
              </div>
            )}
            {state.leastPrivilegeReport && (
              <div style={{ marginTop: "8px", fontSize: "13px" }}>
                Least Privilege Findings: {state.leastPrivilegeReport.findings.length}
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
