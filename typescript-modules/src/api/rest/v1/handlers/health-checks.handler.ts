import { IncomingMessage, ServerResponse } from "http";
import { HealthChecksService } from "../../../../features/health-checks/service/health-checks.service.js";
import { ApiResponseEnvelope } from "../../../../shared/types/envelope.types.js";

export class HealthChecksHandler {
  constructor(private readonly service: HealthChecksService) {}

  async handleLiveness(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const traceId = (req.headers["x-trace-id"] as string) || "00-typescript-live-00";
    try {
      const data = await this.service.checkLiveness();
      const envelope: ApiResponseEnvelope<typeof data> = {
        success: true,
        statusCode: 200,
        data,
        error: null,
        meta: {
          timestamp: new Date().toISOString(),
          traceId
        }
      };
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(envelope));
    } catch {
      res.writeHead(500, { "Content-Type": "application/json" });
      res.end(JSON.stringify({
        success: false,
        statusCode: 500,
        data: null,
        error: { code: "HEALTH_CHECK_FAILED", message: "Liveness probe failed" },
        meta: { timestamp: new Date().toISOString(), traceId }
      }));
    }
  }

  async handleReadiness(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const traceId = (req.headers["x-trace-id"] as string) || "00-typescript-ready-00";
    try {
      const data = await this.service.checkReadiness();
      const statusCode = data.status === "READY" ? 200 : 503;
      const envelope: ApiResponseEnvelope<typeof data> = {
        success: data.status === "READY",
        statusCode,
        data,
        error: null,
        meta: {
          timestamp: new Date().toISOString(),
          traceId
        }
      };
      res.writeHead(statusCode, { "Content-Type": "application/json" });
      res.end(JSON.stringify(envelope));
    } catch {
      res.writeHead(503, { "Content-Type": "application/json" });
      res.end(JSON.stringify({
        success: false,
        statusCode: 503,
        data: null,
        error: { code: "SERVICE_UNAVAILABLE", message: "Readiness probe failed" },
        meta: { timestamp: new Date().toISOString(), traceId }
      }));
    }
  }
}
