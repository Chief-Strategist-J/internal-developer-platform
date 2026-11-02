import { HealthStatus, ReadinessStatus } from "../contracts/health-checks.types.js";
import { HealthChecksRepositoryPort } from "../infrastructure/ports/health-checks.repository.port.js";

export class HealthChecksService {
  constructor(private readonly repository: HealthChecksRepositoryPort) {}

  async checkLiveness(): Promise<HealthStatus> {
    return this.repository.getLiveness();
  }

  async checkReadiness(): Promise<ReadinessStatus> {
    return this.repository.getReadiness();
  }
}
