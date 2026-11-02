import { Result, ok, err } from "../../../shared/client-kernel/index.js";
import { AuthTokenModel, CallerIdentityModel, IssueTokenInputModel, PolicyEvaluationModel } from "../models/role-based-access-control.model.js";

export interface RoleBasedAccessControlApiPort {
  issueToken(input: IssueTokenInputModel, traceId?: string): Promise<Result<AuthTokenModel, Error>>;
  getCallerIdentity(token: string, traceId?: string): Promise<Result<CallerIdentityModel, Error>>;
  evaluatePolicy(resource: string, action: string, token: string, traceId?: string): Promise<Result<PolicyEvaluationModel, Error>>;
  revokeSession(subjectId: string, adminToken: string, traceId?: string): Promise<Result<boolean, Error>>;
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
}
