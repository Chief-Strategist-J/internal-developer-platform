import { IncomingMessage, ServerResponse } from "http";
import { HealthChecksHandler } from "../handlers/health-checks.handler.js";

export class HealthChecksRouter {
  constructor(private readonly handler: HealthChecksHandler) {}

  async handle(req: IncomingMessage, res: ServerResponse): Promise<boolean> {
    const url = req.url || "/";
    if (req.method === "GET" && (url === "/healthz" || url === "/livez")) {
      await this.handler.handleLiveness(req, res);
      return true;
    }
    if (req.method === "GET" && url === "/readyz") {
      await this.handler.handleReadiness(req, res);
      return true;
    }
    return false;
  }
}
