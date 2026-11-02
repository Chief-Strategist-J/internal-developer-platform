import { HealthStatus, ReadinessStatus } from "../../contracts/health-checks.types.js";
import { HealthChecksRepositoryPort } from "../ports/health-checks.repository.port.js";

export class HealthChecksRepositoryAdapter implements HealthChecksRepositoryPort {
  private readonly startTime: number = Date.now();

  async getLiveness(): Promise<HealthStatus> {
    const uptimeSeconds = Math.floor((Date.now() - this.startTime) / 1000);
    return {
      status: "UP",
      version: "1.0.0",
      uptimeSeconds,
      timestamp: new Date().toISOString()
    };
  }

  async getReadiness(): Promise<ReadinessStatus> {
    return {
      status: "READY",
      dependencies: {
        database: "UP",
        cache: "UP",
        telemetry: "UP"
      },
      timestamp: new Date().toISOString()
    };
  }
}
