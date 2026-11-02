export interface CallerIdentityModel {
  readonly subjectId: string;
  readonly tenantId: string;
  readonly roles: readonly string[];
  readonly issuer: string;
  readonly expiresAt: string;
}

export interface AuthTokenModel {
  readonly accessToken: string;
  readonly tokenType: string;
  readonly expiresIn: number;
  readonly subjectId: string;
  readonly tenantId: string;
  readonly roles: readonly string[];
}

export interface PolicyEvaluationModel {
  readonly granted: boolean;
  readonly reason: string;
}

export interface IssueTokenInputModel {
  readonly subjectId: string;
  readonly tenantId: string;
  readonly roles: readonly string[];
}
