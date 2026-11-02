import { BaseController } from "../../../shared/client-kernel/index";
import { RoleBasedAccessControlState } from "../state/role-based-access-control.state";
import { RoleBasedAccessControlApiPort } from "../adapter/role-based-access-control.adapter";
import { IssueTokenInputModel } from "../models/role-based-access-control.model";

export class RoleBasedAccessControlController extends BaseController<RoleBasedAccessControlState> {
  private readonly apiPort: RoleBasedAccessControlApiPort;

  constructor(state: RoleBasedAccessControlState, apiPort: RoleBasedAccessControlApiPort) {
    super(state);
    this.apiPort = apiPort;
  }

  public async login(input: IssueTokenInputModel, traceId?: string): Promise<boolean> {
    this.setPending(traceId);
    try {
      const tokenResult = await this.apiPort.issueToken(input, traceId);
      if (!tokenResult.success) {
        this.setFailure(tokenResult.error, traceId);
        return false;
      }
      this.state.token = tokenResult.data.accessToken;
      const identityResult = await this.apiPort.getCallerIdentity(tokenResult.data.accessToken, traceId);
      if (!identityResult.success) {
        this.setFailure(identityResult.error, traceId);
        return false;
      }
      this.state.identity = identityResult.data;
      this.setSuccess(traceId);
      return true;
    } catch (e) {
      this.setFailure(e instanceof Error ? e : new Error(String(e)), traceId);
      return false;
    }
  }

  public async evaluateAccess(resource: string, action: string, traceId?: string): Promise<boolean> {
    if (!this.state.token) {
      this.setFailure(new Error("No active session"), traceId);
      return false;
    }
    this.setPending(traceId);
    try {
      const result = await this.apiPort.evaluatePolicy(resource, action, this.state.token, traceId);
      if (result.success) {
        this.state.lastEvaluation = result.data;
        this.setSuccess(traceId);
        return result.data.granted;
      }
      this.setFailure(result.error, traceId);
      return false;
    } catch (e) {
      this.setFailure(e instanceof Error ? e : new Error(String(e)), traceId);
      return false;
    }
  }

  public async revokeCurrentSession(adminToken: string, traceId?: string): Promise<boolean> {
    if (!this.state.identity) {
      return false;
    }
    this.setPending(traceId);
    try {
      const result = await this.apiPort.revokeSession(this.state.identity.subjectId, adminToken, traceId);
      if (result.success) {
        this.state.isRevoked = true;
        this.setSuccess(traceId);
        return true;
      }
      this.setFailure(result.error, traceId);
      return false;
    } catch (e) {
      this.setFailure(e instanceof Error ? e : new Error(String(e)), traceId);
      return false;
    }
  }
}
