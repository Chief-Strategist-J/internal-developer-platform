import { BaseState } from "../../../shared/client-kernel/index";
import { ServiceDescriptorModel } from "../models/service-catalog.model";

export class ServiceCatalogState extends BaseState {
  public services: ServiceDescriptorModel[] = [];
  public selectedService: ServiceDescriptorModel | null = null;
  public filterKeyword = "";
  public filterLifecycle = "all";

  public reset(): void {
    this.services = [];
    this.selectedService = null;
    this.filterKeyword = "";
    this.filterLifecycle = "all";
    this.isLoading = false;
    this.hasError = false;
    this.errorMessage = null;
    this.lastTraceId = null;
  }
}
