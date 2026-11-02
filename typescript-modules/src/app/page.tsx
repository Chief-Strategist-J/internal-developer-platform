"use client";

import React, { useState, useMemo } from "react";
import { ServiceCatalogState } from "../features/service-catalog/state/service-catalog.state";
import { ServiceCatalogController } from "../features/service-catalog/controller/service-catalog.controller";
import { ServiceCatalogHttpAdapter } from "../features/service-catalog/adapter/service-catalog.adapter";
import { ServiceCatalogViewComponent } from "../features/service-catalog/ui/service-catalog.view";

import { RoleBasedAccessControlState } from "../features/role-based-access-control/state/role-based-access-control.state";
import { RoleBasedAccessControlController } from "../features/role-based-access-control/controller/role-based-access-control.controller";
import { RoleBasedAccessControlHttpAdapter } from "../features/role-based-access-control/adapter/role-based-access-control.adapter";
import { RoleBasedAccessControlViewComponent } from "../features/role-based-access-control/ui/role-based-access-control.view";

export default function DeveloperPortalPage() {
  const [, setTick] = useState(0);
  const rerender = () => setTick(t => t + 1);

  const backendUrl = process.env["NEXT_PUBLIC_BACKEND_URL"] || "http://127.0.0.1:8080";

  const { rbacState, rbacController } = useMemo(() => {
    const state = new RoleBasedAccessControlState();
    const adapter = new RoleBasedAccessControlHttpAdapter(backendUrl);
    const controller = new RoleBasedAccessControlController(state, adapter);
    const origLogin = controller.login.bind(controller);
    controller.login = async (...args) => {
      const res = await origLogin(...args);
      rerender();
      return res;
    };
    const origEval = controller.evaluateAccess.bind(controller);
    controller.evaluateAccess = async (...args) => {
      const res = await origEval(...args);
      rerender();
      return res;
    };
    return { rbacState: state, rbacController: controller };
  }, [backendUrl]);

  const { catalogState, catalogController } = useMemo(() => {
    const state = new ServiceCatalogState();
    const adapter = new ServiceCatalogHttpAdapter(backendUrl);
    const controller = new ServiceCatalogController(state, adapter);
    const origFetch = controller.fetchCatalog.bind(controller);
    controller.fetchCatalog = async (...args) => {
      const res = await origFetch(...args);
      rerender();
      return res;
    };
    const origReg = controller.registerNewService.bind(controller);
    controller.registerNewService = async (...args) => {
      const res = await origReg(...args);
      rerender();
      return res;
    };
    return { catalogState: state, catalogController: controller };
  }, [backendUrl]);

  return (
    <div>
      <RoleBasedAccessControlViewComponent
        state={rbacState}
        controller={rbacController}
      />

      <ServiceCatalogViewComponent
        state={catalogState}
        controller={catalogController}
        token={rbacState.token || ""}
      />
    </div>
  );
}
