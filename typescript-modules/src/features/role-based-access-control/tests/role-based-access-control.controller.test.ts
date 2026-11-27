import assert from "node:assert/strict";
import test from "node:test";
import { RoleBasedAccessControlController } from "../controller/role-based-access-control.controller";
import { RoleBasedAccessControlState } from "../state/role-based-access-control.state";
import { RoleBasedAccessControlApiPort } from "../adapter/role-based-access-control.adapter";
import {
  AccessGrantRecordModel,
  AccessGrantRequestModel,
  AuthTokenModel,
  CallerIdentityModel,
  IssueTokenInputModel,
  LeastPrivilegeAnalyzeInputModel,
  LeastPrivilegeReportModel,
  PolicyEvaluationModel,
  UserBindingInputModel
} from "../models/role-based-access-control.model";
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

  public async requestJITAccess(input: AccessGrantRequestModel): Promise<Result<AccessGrantRecordModel, Error>> {
    if (this.shouldFail) {
      return err(new Error("JIT grant denied"));
    }
    return ok({
      id: "grant-1",
      tenantId: input.tenantId,
      subjectId: input.subjectId,
      targetNamespace: input.targetNamespace,
      role: input.role,
      status: "ACTIVE",
      expiresAt: "2026-11-27T12:00:00Z"
    });
  }

  public async listActiveGrants(): Promise<Result<readonly AccessGrantRecordModel[], Error>> {
    return ok([]);
  }

  public async revokeJITAccess(_grantId: string): Promise<Result<boolean, Error>> {
    return this.shouldFail ? err(new Error("JIT revoke failed")) : ok(true);
  }

  public async bindUserRole(_input: UserBindingInputModel): Promise<Result<boolean, Error>> {
    return this.shouldFail ? err(new Error("Bind failed")) : ok(true);
  }

  public async claimExclusiveRole(_input: UserBindingInputModel): Promise<Result<boolean, Error>> {
    return this.shouldFail ? err(new Error("Claim conflict")) : ok(true);
  }

  public async validateRoleHierarchy(edges: Readonly<Record<string, readonly string[]>>): Promise<Result<boolean, Error>> {
    if (this.shouldFail || (edges["a"] && edges["a"].includes("b") && edges["b"] && edges["b"].includes("a"))) {
      return err(new Error("Cyclic dependency detected"));
    }
    return ok(true);
  }

  public async analyzeLeastPrivilege(_input: LeastPrivilegeAnalyzeInputModel): Promise<Result<LeastPrivilegeReportModel, Error>> {
    if (this.shouldFail) {
      return err(new Error("Analysis failed"));
    }
    return ok({
      findings: [{ role: "Developer", permission: { resource: "secrets", action: "delete" }, reason: "unused" }]
    });
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

test("RoleBasedAccessControlController: requestJITAccess, validateRoleHierarchy, and analyzeLeastPrivilege update state", async () => {
  const state = new RoleBasedAccessControlState();
  const mockPort = new MockRBACApiPort();
  const controller = new RoleBasedAccessControlController(state, mockPort);
  state.token = "jwt-token-12345";

  const jitOk = await controller.requestJITAccess({
    tenantId: "platform-ops",
    subjectId: "dev-jordan",
    targetNamespace: "prod-ns",
    role: "Developer",
    reason: "incident investigation",
    durationSeconds: 1800,
    approverRole: "PlatformAdmin",
    idempotencyKey: "jit-key-1"
  }, "trace-rbac-4");
  assert.equal(jitOk, true);
  assert.equal(state.activeGrants.length, 1);

  const validHierarchy = await controller.validateRoleHierarchy({ admin: ["dev"], dev: ["viewer"] }, "trace-rbac-5");
  assert.equal(validHierarchy, true);
  assert.equal(state.hierarchyValid, true);

  const cycleHierarchy = await controller.validateRoleHierarchy({ a: ["b"], b: ["a"] }, "trace-rbac-6");
  assert.equal(cycleHierarchy, false);
  assert.equal(state.hierarchyValid, false);

  const lpOk = await controller.analyzeLeastPrivilege({
    roles: { Developer: [{ resource: "secrets", action: "delete" }] },
    usage: [],
    staleWindowHours: 24
  }, "trace-rbac-7");
  assert.equal(lpOk, true);
  assert.equal(state.leastPrivilegeReport?.findings.length, 1);

  const claimOk = await controller.claimExclusiveRole({
    tenantId: "platform-ops",
    subjectId: "dev-jordan",
    roleName: "PlatformAdmin"
  }, "trace-rbac-8");
  assert.equal(claimOk, true);

  const revokeJitOk = await controller.revokeJITAccess("grant-1", "trace-rbac-9");
  assert.equal(revokeJitOk, true);
  assert.equal(state.activeGrants.length, 0);
});
