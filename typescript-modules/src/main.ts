import http, { IncomingMessage, ServerResponse } from "http";
import { HealthChecksRepositoryAdapter } from "./features/health-checks/infrastructure/adapters/health-checks.repository.adapter.js";
import { HealthChecksService } from "./features/health-checks/service/health-checks.service.js";
import { HealthChecksHandler } from "./api/rest/v1/handlers/health-checks.handler.js";
import { HealthChecksRouter } from "./api/rest/v1/routers/health-checks.router.js";
import { SYSTEM_CONSTANTS } from "./shared/constants/system.constants.js";

const healthAdapter = new HealthChecksRepositoryAdapter();
const healthService = new HealthChecksService(healthAdapter);
const healthHandler = new HealthChecksHandler(healthService);
const healthRouter = new HealthChecksRouter(healthHandler);

const adminServer = http.createServer(async (req: IncomingMessage, res: ServerResponse) => {
  const handled = await healthRouter.handle(req, res);
  if (!handled) {
    res.writeHead(404, { "Content-Type": "application/json" });
    res.end(JSON.stringify({
      success: false,
      statusCode: 404,
      data: null,
      error: { code: "NOT_FOUND", message: "Resource not found on admin server" },
      meta: { timestamp: new Date().toISOString(), traceId: "00-typescript-admin-404" }
    }));
  }
});

const publicServer = http.createServer(async (req: IncomingMessage, res: ServerResponse) => {
  const handled = await healthRouter.handle(req, res);
  if (!handled) {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({
      success: true,
      statusCode: 200,
      data: {
        service: SYSTEM_CONSTANTS.SERVICE_NAME,
        status: "RUNNING"
      },
      error: null,
      meta: {
        timestamp: new Date().toISOString(),
        traceId: "00-typescript-public-root"
      }
    }));
  }
});

const port = Number(process.env["PORT"]) || SYSTEM_CONSTANTS.DEFAULT_PORT;
const adminPort = Number(process.env["ADMIN_PORT"]) || SYSTEM_CONSTANTS.DEFAULT_ADMIN_PORT;

adminServer.listen(adminPort, SYSTEM_CONSTANTS.DEFAULT_ADMIN_HOST, () => {
  process.stdout.write(`Admin server listening on http://${SYSTEM_CONSTANTS.DEFAULT_ADMIN_HOST}:${adminPort}\n`);
});

publicServer.listen(port, SYSTEM_CONSTANTS.DEFAULT_HOST, () => {
  process.stdout.write(`Public server listening on http://${SYSTEM_CONSTANTS.DEFAULT_HOST}:${port}\n`);
});

const shutdown = (signal: string) => {
  process.stdout.write(`Received ${signal}, commencing graceful shutdown...\n`);
  adminServer.close(() => {
    publicServer.close(() => {
      process.exit(0);
    });
  });
  setTimeout(() => {
    process.exit(1);
  }, SYSTEM_CONSTANTS.SERVER_DRAIN_TIMEOUT_MS);
};

process.on("SIGTERM", () => shutdown("SIGTERM"));
process.on("SIGINT", () => shutdown("SIGINT"));
