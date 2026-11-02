export class ApplicationError extends Error {
  public readonly statusCode: number;
  public readonly code: string;
  public readonly details?: Record<string, unknown> | undefined;

  constructor(message: string, statusCode = 500, code = "INTERNAL_SERVER_ERROR", details?: Record<string, unknown> | undefined) {
    super(message);
    this.name = "ApplicationError";
    this.statusCode = statusCode;
    this.code = code;
    this.details = details;
    Object.setPrototypeOf(this, ApplicationError.prototype);
  }
}
