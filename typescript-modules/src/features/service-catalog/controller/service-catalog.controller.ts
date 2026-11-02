import { BaseController } from "../../../shared/client-kernel/index";
import { ServiceCatalogState } from "../state/service-catalog.state";
import { ServiceCatalogApiPort } from "../adapter/service-catalog.adapter";
import { RegisterServiceInputModel, ServiceDescriptorModel } from "../models/service-catalog.model";

export class ServiceCatalogController extends BaseController<ServiceCatalogState> {
  private readonly apiPort: ServiceCatalogApiPort;

  constructor(state: ServiceCatalogState, apiPort: ServiceCatalogApiPort) {
    super(state);
    this.apiPort = apiPort;
  }

  public setFilter(keyword: string, lifecycle: string): void {
    this.state.filterKeyword = keyword;
    this.state.filterLifecycle = lifecycle;
  }

  public selectService(service: ServiceDescriptorModel | null): void {
    this.state.selectedService = service;
  }

  public async fetchCatalog(token: string, traceId?: string): Promise<boolean> {
    this.setPending(traceId);
    try {
      const result = await this.apiPort.listServices(token, traceId);
      if (result.success) {
        this.state.services = result.data;
        this.setSuccess(traceId);
        return true;
      }
      this.setFailure(result.error, traceId);
      return false;
    } catch (e) {
      this.setFailure(e instanceof Error ? e : new Error(String(e)), traceId);
      return false;
    }
  }

  public async registerNewService(input: RegisterServiceInputModel, token: string, traceId?: string): Promise<ServiceDescriptorModel | null> {
    this.setPending(traceId);
    try {
      const result = await this.apiPort.registerService(input, token, traceId);
      if (result.success) {
        this.state.services = [result.data, ...this.state.services];
        this.state.selectedService = result.data;
        this.setSuccess(traceId);
        return result.data;
      }
      this.setFailure(result.error, traceId);
      return null;
    } catch (e) {
      this.setFailure(e instanceof Error ? e : new Error(String(e)), traceId);
      return null;
    }
  }
}
