import assert from "node:assert/strict";
import test from "node:test";
import { ServiceCatalogState } from "../state/service-catalog.state.js";

test("ServiceCatalogState: initializes with clean default properties", () => {
  const state = new ServiceCatalogState();
  assert.equal(state.isLoading, false);
  assert.equal(state.hasError, false);
  assert.equal(state.errorMessage, null);
  assert.equal(state.services.length, 0);
  assert.equal(state.selectedService, null);
  assert.equal(state.filterKeyword, "");
  assert.equal(state.filterLifecycle, "all");
});

test("ServiceCatalogState: reset clears all state variables back to initial", () => {
  const state = new ServiceCatalogState();
  state.isLoading = true;
  state.hasError = true;
  state.errorMessage = "Network timeout";
  state.filterKeyword = "billing";
  state.filterLifecycle = "production";
  state.services = [{
    id: "billing-svc",
    tenantId: "tenant-a",
    name: "Billing Service",
    owner: "bill@corp.io",
    lifecycle: "production",
    status: "ACTIVE",
    version: 1,
    createdAt: "2026-11-02T00:00:00Z",
    updatedAt: "2026-11-02T00:00:00Z"
  }];

  state.reset();

  assert.equal(state.isLoading, false);
  assert.equal(state.hasError, false);
  assert.equal(state.errorMessage, null);
  assert.equal(state.services.length, 0);
  assert.equal(state.filterKeyword, "");
  assert.equal(state.filterLifecycle, "all");
});
