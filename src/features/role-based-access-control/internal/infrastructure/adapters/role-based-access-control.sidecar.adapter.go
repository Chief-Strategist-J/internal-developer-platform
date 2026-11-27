/*
Package adapters provides HTTP client integration for local sidecar Policy Decision Points (OPA / Cedar).
It translates RBAC evaluation requests into sidecar HTTP payloads and enforces strict timeouts.
*/
package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/internal/contracts"
)

type HTTPSidecarPDPAdapter struct {
	config     rbacContracts.SidecarPDPConfig
	httpClient *http.Client
}

func NewHTTPSidecarPDPAdapter(cfg rbacContracts.SidecarPDPConfig) *HTTPSidecarPDPAdapter {
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 250 * time.Millisecond
	}
	return &HTTPSidecarPDPAdapter{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type opaInputWrapper struct {
	Input rbacContracts.SidecarDecisionRequest `json:"input"`
}

type opaResponse struct {
	Result bool `json:"result"`
}

type cedarResponse struct {
	Decision string `json:"decision"`
}

func (a *HTTPSidecarPDPAdapter) EvaluateSidecar(ctx context.Context, req rbacContracts.SidecarDecisionRequest) (rbacContracts.SidecarDecisionResult, error) {
	base := strings.TrimRight(a.config.Endpoint, "/")
	path := strings.TrimLeft(a.config.PolicyPath, "/")
	if path == "" {
		if a.config.Engine == rbacContracts.SidecarEngineCedar {
			path = "v1/is_authorized"
		} else {
			path = "v1/data/rbac/allow"
		}
	}
	url := base + "/" + path

	var body []byte
	var err error
	if a.config.Engine == rbacContracts.SidecarEngineCedar {
		body, err = json.Marshal(req)
	} else {
		body, err = json.Marshal(opaInputWrapper{Input: req})
	}
	if err != nil {
		return rbacContracts.SidecarDecisionResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return rbacContracts.SidecarDecisionResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return rbacContracts.SidecarDecisionResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return rbacContracts.SidecarDecisionResult{}, fmt.Errorf("sidecar PDP returned status %d", resp.StatusCode)
	}

	if a.config.Engine == rbacContracts.SidecarEngineCedar {
		var cr cedarResponse
		if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
			return rbacContracts.SidecarDecisionResult{}, err
		}
		allowed := strings.EqualFold(cr.Decision, "Allow")
		return rbacContracts.SidecarDecisionResult{
			Allowed: allowed,
			Engine:  rbacContracts.SidecarEngineCedar,
			Reason:  "cedar sidecar decision: " + cr.Decision,
		}, nil
	}

	var or opaResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return rbacContracts.SidecarDecisionResult{}, err
	}
	reason := "opa sidecar denied"
	if or.Result {
		reason = "opa sidecar allowed"
	}
	return rbacContracts.SidecarDecisionResult{
		Allowed: or.Result,
		Engine:  rbacContracts.SidecarEngineOPA,
		Reason:  reason,
	}, nil
}

func (a *HTTPSidecarPDPAdapter) HealthCheck(ctx context.Context) error {
	base := strings.TrimRight(a.config.Endpoint, "/")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sidecar PDP health check failed with status %d", resp.StatusCode)
	}
	return nil
}
