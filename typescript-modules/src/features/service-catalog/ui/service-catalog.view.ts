import { ServiceCatalogState } from "../state/service-catalog.state.js";
import { ServiceDescriptorModel } from "../models/service-catalog.model.js";

export class ServiceCatalogView {
  public render(state: ServiceCatalogState): string {
    if (state.isLoading) {
      return this.renderLoading();
    }
    if (state.hasError) {
      return this.renderError(state.errorMessage ?? "Unknown error occurred");
    }
    const filtered = this.getFilteredServices(state);
    if (filtered.length === 0) {
      return this.renderEmpty();
    }
    return this.renderContent(filtered, state.selectedService);
  }

  private getFilteredServices(state: ServiceCatalogState): ServiceDescriptorModel[] {
    return state.services.filter(s => {
      const matchKeyword = !state.filterKeyword ||
        s.name.toLowerCase().includes(state.filterKeyword.toLowerCase()) ||
        s.id.toLowerCase().includes(state.filterKeyword.toLowerCase());
      const matchLifecycle = state.filterLifecycle === "all" || s.lifecycle === state.filterLifecycle;
      return matchKeyword && matchLifecycle;
    });
  }

  private renderLoading(): string {
    return `<section aria-busy="true" aria-label="Loading service catalog" class="catalog-loading">
  <div class="skeleton-card"></div>
  <div class="skeleton-card"></div>
  <div class="skeleton-card"></div>
</section>`;
  }

  private renderError(message: string): string {
    return `<section role="alert" class="catalog-error">
  <h2 class="error-title">Failed to load service catalog</h2>
  <p class="error-desc">${this.escapeHtml(message)}</p>
  <button type="button" class="btn-retry" onclick="window.catalogController.fetchCatalog()">Retry</button>
</section>`;
  }

  private renderEmpty(): string {
    return `<section class="catalog-empty">
  <h2 class="empty-title">No services found</h2>
  <p class="empty-desc">No registered service descriptors matched the current criteria.</p>
  <button type="button" class="btn-create" onclick="window.catalogController.openCreateModal()">Register First Service</button>
</section>`;
  }

  private renderContent(services: ServiceDescriptorModel[], selected: ServiceDescriptorModel | null): string {
    const rows = services.map(s => {
      const isSelected = selected && selected.id === s.id;
      return `<tr class="catalog-row ${isSelected ? "selected" : ""}" data-id="${this.escapeHtml(s.id)}">
  <td class="col-id font-mono">${this.escapeHtml(s.id)}</td>
  <td class="col-name font-medium">${this.escapeHtml(s.name)}</td>
  <td class="col-tenant">${this.escapeHtml(s.tenantId)}</td>
  <td class="col-owner">${this.escapeHtml(s.owner)}</td>
  <td class="col-lifecycle"><span class="badge badge-${this.escapeHtml(s.lifecycle)}">${this.escapeHtml(s.lifecycle)}</span></td>
  <td class="col-status"><span class="status-indicator status-${this.escapeHtml(s.status)}">${this.escapeHtml(s.status)}</span></td>
  <td class="col-actions">
    <button type="button" class="btn-view" onclick="window.catalogController.selectService('${this.escapeHtml(s.id)}')">Details</button>
  </td>
</tr>`;
    }).join("\n");

    return `<section class="catalog-table-container">
  <header class="catalog-header">
    <h2 class="section-title">Enterprise Service Catalog</h2>
    <span class="count-badge">${services.length} services</span>
  </header>
  <div class="table-responsive">
    <table class="catalog-table">
      <thead>
        <tr>
          <th>Service ID</th>
          <th>Name</th>
          <th>Tenant</th>
          <th>Owner</th>
          <th>Lifecycle</th>
          <th>Status</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        ${rows}
      </tbody>
    </table>
  </div>
</section>`;
  }

  private escapeHtml(str: string): string {
    return str
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#039;");
  }
}
