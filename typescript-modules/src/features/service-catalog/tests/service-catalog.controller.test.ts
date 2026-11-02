import assert from "node:assert/strict";
import test from "node:test";
import { ServiceCatalogController } from "../controller/service-catalog.controller";
import { ServiceCatalogState } from "../state/service-catalog.state";
import { ServiceCatalogApiPort } from "../adapter/service-catalog.adapter";
import { ServiceDescriptorModel, RegisterServiceInputModel } from "../models/service-catalog.model";
import { Result, ok, err } from "../../../shared/client-kernel/index";

class MockServiceCatalogApiPort implements ServiceCatalogApiPort {
  public mockServices: ServiceDescriptorModel[] = [];
  public shouldFail = false;

  public async listServices(): Promise<Result<ServiceDescriptorModel[], Error>> {
    if (this.shouldFail) {
      return err(new Error("Database connection failure"));
    }
    return ok(this.mockServices);
  }

  public async getServiceById(id: string): Promise<Result<ServiceDescriptorModel, Error>> {
    const found = this.mockServices.find(s => s.id === id);
    if (!found) {
      return err(new Error("Not found"));
    }
    return ok(found);
  }

  public async registerService(input: RegisterServiceInputModel): Promise<Result<ServiceDescriptorModel, Error>> {
    if (this.shouldFail) {
      return err(new Error("Duplicate service id"));
    }
    const created: ServiceDescriptorModel = {
      ...input,
      status: "ACTIVE",
      version: 1,
      createdAt: "2026-11-02T00:00:00Z",
      updatedAt: "2026-11-02T00:00:00Z"
    };
    this.mockServices.push(created);
    return ok(created);
  }
}

test("ServiceCatalogController: fetchCatalog mutates state on success", async () => {
  const state = new ServiceCatalogState();
  const mockPort = new MockServiceCatalogApiPort();
  mockPort.mockServices = [{
    id: "orders-api",
    tenantId: "retail",
    name: "Orders API",
    owner: "retail-team@fintech.io",
    lifecycle: "production",
    status: "ACTIVE",
    version: 1,
    createdAt: "2026-11-02T00:00:00Z",
    updatedAt: "2026-11-02T00:00:00Z"
  }];

  const controller = new ServiceCatalogController(state, mockPort);
  const success = await controller.fetchCatalog("fake-jwt-token", "trace-101");

  assert.equal(success, true);
  assert.equal(state.isLoading, false);
  assert.equal(state.hasError, false);
  assert.equal(state.services.length, 1);
  assert.equal(state.services[0]?.id, "orders-api");
});

test("ServiceCatalogController: fetchCatalog mutates state on error", async () => {
  const state = new ServiceCatalogState();
  const mockPort = new MockServiceCatalogApiPort();
  mockPort.shouldFail = true;

  const controller = new ServiceCatalogController(state, mockPort);
  const success = await controller.fetchCatalog("fake-jwt-token", "trace-102");

  assert.equal(success, false);
  assert.equal(state.isLoading, false);
  assert.equal(state.hasError, true);
  assert.equal(state.errorMessage, "Database connection failure");
});

test("ServiceCatalogController: registerNewService prepends to state list", async () => {
  const state = new ServiceCatalogState();
  const mockPort = new MockServiceCatalogApiPort();
  const controller = new ServiceCatalogController(state, mockPort);

  const input: RegisterServiceInputModel = {
    id: "payments-engine",
    tenantId: "retail",
    name: "Payments Engine",
    owner: "pay@fintech.io",
    lifecycle: "production"
  };

  const created = await controller.registerNewService(input, "fake-jwt-token", "trace-103");

  assert.notEqual(created, null);
  assert.equal(state.services.length, 1);
  assert.equal(state.services[0]?.id, "payments-engine");
  assert.equal(state.selectedService?.id, "payments-engine");
});
