export interface ServiceDescriptorModel {
  readonly id: string;
  readonly tenantId: string;
  readonly name: string;
  readonly owner: string;
  readonly lifecycle: "experimental" | "staging" | "production" | "deprecated";
  readonly status: "ACTIVE" | "MAINTENANCE" | "DEPRECATED" | "TERMINATED";
  readonly version: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface RegisterServiceInputModel {
  readonly id: string;
  readonly tenantId: string;
  readonly name: string;
  readonly owner: string;
  readonly lifecycle: "experimental" | "staging" | "production" | "deprecated";
}
