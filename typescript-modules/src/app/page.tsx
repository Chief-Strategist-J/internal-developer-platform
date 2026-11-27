"use client";

import React, { useState, useMemo } from "react";
import {
  ServiceCatalogState,
  ServiceCatalogController,
  ServiceCatalogHttpAdapter,
  ServiceCatalogViewComponent
} from "../features/service-catalog/index";
import {
  RoleBasedAccessControlState,
  RoleBasedAccessControlController,
  RoleBasedAccessControlHttpAdapter,
  RoleBasedAccessControlViewComponent
} from "../features/role-based-access-control/index";

export default function DeveloperPortalPage() {
  const [, setTick] = useState(0);
  const rerender = () => setTick(t => t + 1);

  const backendUrl = process.env["NEXT_PUBLIC_BACKEND_URL"] || "";

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
