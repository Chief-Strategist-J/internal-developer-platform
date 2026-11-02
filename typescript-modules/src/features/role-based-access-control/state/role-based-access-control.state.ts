import { BaseState } from "../../../shared/client-kernel/index.js";
import { CallerIdentityModel, PolicyEvaluationModel } from "../models/role-based-access-control.model.js";

export class RoleBasedAccessControlState extends BaseState {
  public token: string | null = null;
  public identity: CallerIdentityModel | null = null;
  public lastEvaluation: PolicyEvaluationModel | null = null;
  public isRevoked = false;

  public reset(): void {
    this.token = null;
    this.identity = null;
    this.lastEvaluation = null;
    this.isRevoked = false;
    this.isLoading = false;
    this.hasError = false;
    this.errorMessage = null;
    this.lastTraceId = null;
  }
}
