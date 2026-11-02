import { RoleBasedAccessControlState } from "../state/role-based-access-control.state.js";

export class RoleBasedAccessControlView {
  public render(state: RoleBasedAccessControlState): string {
    if (state.isLoading) {
      return this.renderLoading();
    }
    if (state.hasError) {
      return this.renderError(state.errorMessage ?? "Authentication error");
    }
    if (state.isRevoked) {
      return this.renderRevoked();
    }
    if (!state.token || !state.identity) {
      return this.renderUnauthenticated();
    }
    return this.renderAuthenticated(state);
  }

  private renderLoading(): string {
    return `<section aria-busy="true" aria-label="Authenticating session" class="auth-loading">
  <div class="skeleton-pill"></div>
  <div class="skeleton-text"></div>
</section>`;
  }

  private renderError(message: string): string {
    return `<section role="alert" class="auth-error">
  <h3 class="auth-error-title">Authentication Failed</h3>
  <p class="auth-error-msg">${this.escapeHtml(message)}</p>
  <button type="button" class="btn-retry" onclick="window.authController.retry()">Try Again</button>
</section>`;
  }

  private renderRevoked(): string {
    return `<section class="auth-revoked">
  <div class="revoked-badge">Session Revoked</div>
  <p class="revoked-desc">Your authentication token has been invalidated by security operations.</p>
  <button type="button" class="btn-login" onclick="window.authController.showLogin()">Re-authenticate</button>
</section>`;
  }

  private renderUnauthenticated(): string {
    return `<section class="auth-unauthenticated">
  <h2 class="auth-heading">Developer Portal Sign-In</h2>
  <form class="auth-form" onsubmit="window.authController.handleSubmit(event)">
    <label for="input-subject" class="form-label">Subject ID</label>
    <input id="input-subject" name="subjectId" type="text" required class="input-control" placeholder="e.g. dev-user-123" />
    <label for="input-tenant" class="form-label">Tenant ID</label>
    <input id="input-tenant" name="tenantId" type="text" required class="input-control" placeholder="e.g. fintech-core" />
    <button type="submit" class="btn-submit">Authenticate</button>
  </form>
</section>`;
  }

  private renderAuthenticated(state: RoleBasedAccessControlState): string {
    const identity = state.identity!;
    const rolesList = identity.roles.map(r => `<span class="role-pill">${this.escapeHtml(r)}</span>`).join(" ");

    let evalSection = "";
    if (state.lastEvaluation) {
      const isGranted = state.lastEvaluation.granted;
      evalSection = `<div class="eval-result eval-${isGranted ? "granted" : "denied"}">
  <span class="eval-status">${isGranted ? "Access Granted" : "Access Denied"}</span>
  <span class="eval-reason">${this.escapeHtml(state.lastEvaluation.reason)}</span>
</div>`;
    }

    return `<section class="auth-identity-card">
  <header class="identity-header">
    <h3 class="identity-title">Active Session</h3>
    <span class="session-badge active">Valid</span>
  </header>
  <div class="identity-grid">
    <div class="identity-field">
      <span class="field-label">Subject</span>
      <span class="field-value font-mono">${this.escapeHtml(identity.subjectId)}</span>
    </div>
    <div class="identity-field">
      <span class="field-label">Tenant</span>
      <span class="field-value">${this.escapeHtml(identity.tenantId)}</span>
    </div>
    <div class="identity-field">
      <span class="field-label">Assigned Roles</span>
      <div class="roles-container">${rolesList}</div>
    </div>
    <div class="identity-field">
      <span class="field-label">Expires At</span>
      <span class="field-value font-mono">${this.escapeHtml(identity.expiresAt)}</span>
    </div>
  </div>
  ${evalSection}
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
