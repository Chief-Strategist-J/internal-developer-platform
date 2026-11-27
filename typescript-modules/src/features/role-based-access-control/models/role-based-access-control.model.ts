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

export interface AccessGrantRequestModel {
  readonly tenantId: string;
  readonly subjectId: string;
  readonly targetNamespace: string;
  readonly role: string;
  readonly reason: string;
  readonly durationSeconds: number;
  readonly approverRole: string;
  readonly idempotencyKey: string;
}

export interface AccessGrantRecordModel {
  readonly id: string;
  readonly tenantId: string;
  readonly subjectId: string;
  readonly targetNamespace: string;
  readonly role: string;
  readonly status: string;
  readonly expiresAt: string;
}

export interface UserBindingInputModel {
  readonly tenantId: string;
  readonly subjectId: string;
  readonly roleName: string;
}

export interface LeastPrivilegeAnalyzeInputModel {
  readonly roles: Readonly<Record<string, readonly { readonly resource: string; readonly action: string }[]>>;
  readonly usage: readonly { readonly role: string; readonly resource: string; readonly action: string; readonly hitCount: number }[];
  readonly staleWindowHours: number;
}

export interface LeastPrivilegeReportModel {
  readonly findings: readonly { readonly role: string; readonly permission: { readonly resource: string; readonly action: string }; readonly reason: string }[];
}
