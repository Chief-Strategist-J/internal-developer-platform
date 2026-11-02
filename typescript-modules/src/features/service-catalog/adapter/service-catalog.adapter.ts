import { ok, err, Result } from "../../../shared/client-kernel/index";
import { RegisterServiceInputModel, ServiceDescriptorModel } from "../models/service-catalog.model";

export interface ServiceCatalogApiPort {
  listServices(token: string, traceId?: string): Promise<Result<ServiceDescriptorModel[], Error>>;
  getServiceById(id: string, token: string, traceId?: string): Promise<Result<ServiceDescriptorModel, Error>>;
  registerService(input: RegisterServiceInputModel, token: string, traceId?: string): Promise<Result<ServiceDescriptorModel, Error>>;
}

export class ServiceCatalogHttpAdapter implements ServiceCatalogApiPort {
  private readonly baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  public async listServices(token: string, traceId?: string): Promise<Result<ServiceDescriptorModel[], Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/services`, {
        method: "GET",
        headers: {
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-catalog-list"
        }
      });
      if (!response.ok) {
        return err(new Error(`Failed to list services: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: ServiceDescriptorModel[] };
      return ok(envelope.data ?? []);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async getServiceById(id: string, token: string, traceId?: string): Promise<Result<ServiceDescriptorModel, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/services/${encodeURIComponent(id)}`, {
        method: "GET",
        headers: {
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-catalog-get"
        }
      });
      if (!response.ok) {
        return err(new Error(`Service not found: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: ServiceDescriptorModel };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }

  public async registerService(input: RegisterServiceInputModel, token: string, traceId?: string): Promise<Result<ServiceDescriptorModel, Error>> {
    try {
      const response = await fetch(`${this.baseUrl}/api/v1/services`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`,
          "X-Trace-Id": traceId ?? "00-fe-catalog-create"
        },
        body: JSON.stringify(input)
      });
      if (!response.ok) {
        return err(new Error(`Failed to register service: HTTP ${response.status}`));
      }
      const envelope = await response.json() as { data: ServiceDescriptorModel };
      return ok(envelope.data);
    } catch (e) {
      return err(e instanceof Error ? e : new Error(String(e)));
    }
  }
}
