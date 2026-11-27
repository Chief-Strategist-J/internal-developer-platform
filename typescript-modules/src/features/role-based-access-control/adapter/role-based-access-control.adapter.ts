import { Result, ok, err } from "../../../shared/client-kernel/index";
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

export interface RoleBasedAccessControlApiPort {
  issueToken(input: IssueTokenInputModel, traceId?: string): Promise<Result<AuthTokenModel, Error>>;
  getCallerIdentity(token: string, traceId?: string): Promise<Result<CallerIdentityModel, Error>>;
  evaluatePolicy(resource: string, action: string, token: string, traceId?: string): Promise<Result<PolicyEvaluationModel, Error>>;
  revokeSession(subjectId: string, adminToken: string, traceId?: string): Promise<Result<boolean, Error>>;
  requestJITAccess(input: AccessGrantRequestModel, token: string, traceId?: string): Promise<Result<AccessGrantRecordModel, Error>>;
  revokeJITAccess(grantId: string, token: string, traceId?: string): Promise<Result<boolean, Error>>;
  listActiveGrants(tenantId: string, subjectId: string, token: string, traceId?: string): Promise<Result<readonly AccessGrantRecordModel[], Error>>;
  bindUserRole(input: UserBindingInputModel, token: string, traceId?: string): Promise<Result<boolean, Error>>;
  claimExclusiveRole(input: UserBindingInputModel, token: string, traceId?: string): Promise<Result<boolean, Error>>;
  validateRoleHierarchy(edges: Readonly<Record<string, readonly string[]>>, token: string, traceId?: string): Promise<Result<boolean, Error>>;
  analyzeLeastPrivilege(input: LeastPrivilegeAnalyzeInputModel, token: string, traceId?: string): Promise<Result<LeastPrivilegeReportModel, Error>>;
}

export class RoleBasedAccessControlHttpAdapter implements RoleBasedAccessControlApiPort {
  private readonly baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  public async issueToken(input: IssueTokenInputModel, traceId?: string): Promise<Result<AuthTokenModel, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/token`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Trace-Id": traceId ?? "00-fe-rbac-token"
        },
        body: JSON.stringify(input)
      });
      if (!response.ok) {
        return err(new Error(`Token issuance failed: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: AuthTokenModel };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async getCallerIdentity(token: string, traceId?: string): Promise<Result<CallerIdentityModel, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/me`, {
        method: "GET",
        headers: {
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-me"
        }
      });
      if (!response.ok) {
        return err(new Error(`Failed to retrieve identity: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: CallerIdentityModel };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async evaluatePolicy(resource: string, action: string, token: string, traceId?: string): Promise<Result<PolicyEvaluationModel, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/evaluate`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-eval"
        },
        body: JSON.stringify({ resource, action })
      });
      if (!response.ok) {
        return err(new Error(`Policy evaluation failed: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: PolicyEvaluationModel };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async revokeSession(subjectId: string, adminToken: string, traceId?: string): Promise<Result<boolean, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/revoke`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${adminToken}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-revoke"
        },
        body: JSON.stringify({ subjectId })
      });
      if (!response.ok) {
        return err(new Error(`Revocation failed: HTTP ${response.status}`));
      }
      return ok(true);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async requestJITAccess(input: AccessGrantRequestModel, token: string, traceId?: string): Promise<Result<AccessGrantRecordModel, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/jit/grants`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-jit"
        },
        body: JSON.stringify(input)
      });
      if (!response.ok) {
        return err(new Error(`JIT access request failed: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: AccessGrantRecordModel };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async revokeJITAccess(grantId: string, token: string, traceId?: string): Promise<Result<boolean, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/jit/grants/${encodeURIComponent(grantId)}/revoke`, {
        method: "POST",
        headers: {
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-jit-revoke"
        }
      });
      if (!response.ok) {
        return err(new Error(`JIT revoke failed: HTTP ${response.status}`));
      }
      return ok(true);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async listActiveGrants(tenantId: string, subjectId: string, token: string, traceId?: string): Promise<Result<readonly AccessGrantRecordModel[], Error>> {
    try {
      const query = new URLSearchParams({ tenantId, subjectId }).toString();
      const response = await fetch(`${this.baseUrl}/api/v1/auth/jit/grants?${query}`, {
        method: "GET",
        headers: {
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-grants"
        }
      });
      if (!response.ok) {
        return err(new Error(`List grants failed: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: readonly AccessGrantRecordModel[] };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async bindUserRole(input: UserBindingInputModel, token: string, traceId?: string): Promise<Result<boolean, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/bindings`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-bind"
        },
        body: JSON.stringify(input)
      });
      if (!response.ok) {
        return err(new Error(`Bind user role failed: HTTP ${response.status}`));
      }
      return ok(true);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async claimExclusiveRole(input: UserBindingInputModel, token: string, traceId?: string): Promise<Result<boolean, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/bindings/claim`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-claim"
        },
        body: JSON.stringify(input)
      });
      if (!response.ok) {
        return err(new Error(`Claim exclusive role failed: HTTP ${response.status}`));
      }
      return ok(true);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async validateRoleHierarchy(edges: Readonly<Record<string, readonly string[]>>, token: string, traceId?: string): Promise<Result<boolean, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/rbac/roles/validate-hierarchy`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-hierarchy"
        },
        body: JSON.stringify({ edges })
      });
      if (!response.ok) {
        return err(new Error(`Hierarchy validation failed: HTTP ${response.status}`));
      }
      return ok(true);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async analyzeLeastPrivilege(input: LeastPrivilegeAnalyzeInputModel, token: string, traceId?: string): Promise<Result<LeastPrivilegeReportModel, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/rbac/least-privilege/analyze`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-rbac-lp"
        },
        body: JSON.stringify(input)
      });
      if (!response.ok) {
        return err(new Error(`Least privilege analysis failed: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: LeastPrivilegeReportModel };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }
}
