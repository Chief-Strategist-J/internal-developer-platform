export interface HealthStatus {
  status: "UP" | "DOWN";
  version: string;
  uptimeSeconds: number;
  timestamp: string;
}

export interface ReadinessStatus {
  status: "READY" | "NOT_READY";
  dependencies: Record<string, "UP" | "DOWN">;
  timestamp: string;
}
