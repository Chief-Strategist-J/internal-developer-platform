import { HealthStatus, ReadinessStatus } from "../../contracts/health-checks.types.js";

export interface HealthChecksRepositoryPort {
  getLiveness(): Promise<HealthStatus>;
  getReadiness(): Promise<ReadinessStatus>;
}
