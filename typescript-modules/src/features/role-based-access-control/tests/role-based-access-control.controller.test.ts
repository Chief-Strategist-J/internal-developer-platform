import assert from "node:assert/strict";
import test from "node:test";
import { RoleBasedAccessControlController } from "../controller/role-based-access-control.controller";
import { RoleBasedAccessControlState } from "../state/role-based-access-control.state";
import { RoleBasedAccessControlApiPort } from "../adapter/role-based-access-control.adapter";
import { AuthTokenModel, CallerIdentityModel, IssueTokenInputModel, PolicyEvaluationModel } from "../models/role-based-access-control.model";
import { Result, ok, err } from "../../../shared/client-kernel/index";

class MockRBACApiPort implements RoleBasedAccessControlApiPort {
  public shouldFail = false;
  public mockToken: AuthTokenModel = {
    accessToken: "jwt-token-12345",
    tokenType: "Bearer",
    expiresIn: 3600,
    subjectId: "dev-jordan",
    tenantId: "platform-ops",
    roles: ["Developer"]
  };
  public mockIdentity: CallerIdentityModel = {
    subjectId: "dev-jordan",
    tenantId: "platform-ops",
    roles: ["Developer"],
    issuer: "internal-developer-platform",
    expiresAt: "2026-11-02T22:00:00Z"
  };

  public async issueToken(_input: IssueTokenInputModel): Promise<Result<AuthTokenModel, Error>> {
    if (this.shouldFail) {
      return err(new Error("Invalid credentials"));
    }
    return ok(this.mockToken);
  }

  public async getCallerIdentity(_token: string): Promise<Result<CallerIdentityModel, Error>> {
    if (this.shouldFail) {
      return err(new Error("Signature expired"));
    }
    return ok(this.mockIdentity);
  }

  public async evaluatePolicy(resource: string, action: string): Promise<Result<PolicyEvaluationModel, Error>> {
    if (this.shouldFail) {
      return err(new Error("Evaluation error"));
    }
    const granted = !(resource === "infrastructure" && action === "write");
    return ok({
      granted,
      reason: granted ? "permission granted" : "insufficient role privilege"
    });
  }

  public async revokeSession(_subjectId: string): Promise<Result<boolean, Error>> {
    if (this.shouldFail) {
      return err(new Error("Revocation store unreachable"));
    }
    return ok(true);
  }
}

test("RoleBasedAccessControlController: login updates token and identity on success", async () => {
  const state = new RoleBasedAccessControlState();
  const mockPort = new MockRBACApiPort();
  const controller = new RoleBasedAccessControlController(state, mockPort);

  const success = await controller.login({
    subjectId: "dev-jordan",
    tenantId: "platform-ops",
    roles: ["Developer"]
  }, "trace-rbac-1");

  assert.equal(success, true);
  assert.equal(state.isLoading, false);
  assert.equal(state.hasError, false);
  assert.equal(state.token, "jwt-token-12345");
  assert.equal(state.identity?.subjectId, "dev-jordan");
});

test("RoleBasedAccessControlController: evaluateAccess mutates lastEvaluation", async () => {
  const state = new RoleBasedAccessControlState();
  const mockPort = new MockRBACApiPort();
  const controller = new RoleBasedAccessControlController(state, mockPort);

  state.token = "jwt-token-12345";
  state.identity = mockPort.mockIdentity;

  const isGranted = await controller.evaluateAccess("service-catalog", "read", "trace-rbac-2");

  assert.equal(isGranted, true);
  assert.equal(state.lastEvaluation?.granted, true);
  assert.equal(state.lastEvaluation?.reason, "permission granted");
});

test("RoleBasedAccessControlController: revokeCurrentSession marks state as revoked", async () => {
  const state = new RoleBasedAccessControlState();
  const mockPort = new MockRBACApiPort();
  const controller = new RoleBasedAccessControlController(state, mockPort);

  state.identity = mockPort.mockIdentity;

  const success = await controller.revokeCurrentSession("admin-super-token", "trace-rbac-3");

  assert.equal(success, true);
  assert.equal(state.isRevoked, true);
});
