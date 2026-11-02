import assert from "node:assert/strict";
import test from "node:test";
import { RoleBasedAccessControlState } from "../state/role-based-access-control.state";

test("RoleBasedAccessControlState: initializes with empty unauthenticated state", () => {
  const state = new RoleBasedAccessControlState();
  assert.equal(state.isLoading, false);
  assert.equal(state.hasError, false);
  assert.equal(state.token, null);
  assert.equal(state.identity, null);
  assert.equal(state.lastEvaluation, null);
  assert.equal(state.isRevoked, false);
});

test("RoleBasedAccessControlState: reset clears all authentication variables", () => {
  const state = new RoleBasedAccessControlState();
  state.token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9";
  state.identity = {
    subjectId: "dev-user-01",
    tenantId: "tenant-fintech",
    roles: ["Developer"],
    issuer: "internal-developer-platform",
    expiresAt: "2026-11-02T20:00:00Z"
  };
  state.isRevoked = true;
  state.isLoading = true;

  state.reset();

  assert.equal(state.token, null);
  assert.equal(state.identity, null);
  assert.equal(state.isRevoked, false);
  assert.equal(state.isLoading, false);
  assert.equal(state.hasError, false);
});
