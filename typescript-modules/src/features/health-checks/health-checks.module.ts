import { IncomingMessage, ServerResponse } from "http";
import { HealthChecksRepositoryAdapter } from "./infrastructure/adapters/health-checks.repository.adapter.js";
import { HealthChecksService } from "./service/health-checks.service.js";
import { HealthChecksHandler } from "./api/rest/v1/health-checks.rest.handler.js";
import { HealthChecksRouter } from "./api/rest/v1/health-checks.router.js";

export interface HealthChecksModule {
  readonly name: "health-checks";
  handle(req: IncomingMessage, res: ServerResponse): Promise<boolean>;
}

export function buildHealthChecksModule(): HealthChecksModule {
  const adapter = new HealthChecksRepositoryAdapter();
  const service = new HealthChecksService(adapter);
  const handler = new HealthChecksHandler(service);
  const router = new HealthChecksRouter(handler);
  return {
    name: "health-checks",
    handle: (req, res) => router.handle(req, res)
  };
}
