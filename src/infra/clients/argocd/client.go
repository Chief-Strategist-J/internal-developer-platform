package argocd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/shared/constants"
	apperrors "internal-developer-platform/src/shared/errors"
)

type ArgoCDClient struct {
	httpClient *http.Client
	baseURL    string
	authToken  string
	executor   *resilience.ResilienceExecutor
}

func NewArgoCDClient(config ArgoCDClientConfig, resConfig resilience.ResilienceConfig) *ArgoCDClient {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: config.InsecureSkipVerify},
	}

	return &ArgoCDClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		baseURL:   strings.TrimRight(config.BaseURL, "/"),
		authToken: config.AuthToken,
		executor:  resilience.NewResilienceExecutor(resConfig),
	}
}

func (c *ArgoCDClient) TriggerSync(ctx context.Context, appName string, revision string, prune bool, dryRun bool) (*ApplicationStatusResponse, error) {
	if appName == "" {
		return nil, apperrors.NewValidationError("appName is required")
	}

	url := fmt.Sprintf("%s"+EndpointApplicationSync, c.baseURL, appName)
	payload := SyncRequestPayload{
		Revision: revision,
		Prune:    prune,
		DryRun:   dryRun,
	}

	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.NewValidationError("failed to serialize sync request payload: " + err.Error())
	}

	h := sha256.New()
	h.Write([]byte(appName + ":" + revision + fmt.Sprintf(":%t:%t", prune, dryRun)))
	idempotencyKey := hex.EncodeToString(h.Sum(nil))

	var statusResp ApplicationStatusResponse
	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		req, reqErr := http.NewRequestWithContext(opCtx, http.MethodPost, url, bytes.NewReader(rawBody))
		if reqErr != nil {
			return apperrors.NewConflictError("failed to construct http request: " + reqErr.Error())
		}

		c.attachHeaders(req, idempotencyKey)

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return apperrors.NewDownstreamUnavailableError("argocd api network failure: " + respErr.Error())
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return c.mapHTTPError(resp)
		}

		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return apperrors.NewDownstreamUnavailableError("failed to read response body: " + readErr.Error())
		}

		if len(bodyBytes) > 0 {
			if parseErr := json.Unmarshal(bodyBytes, &statusResp); parseErr != nil {
				return apperrors.NewDownstreamUnavailableError("failed to parse argocd response: " + parseErr.Error())
			}
		}

		return nil
	})

	if execErr != nil {
		return nil, execErr
	}

	return &statusResp, nil
}

func (c *ArgoCDClient) GetApplication(ctx context.Context, appName string) (*ApplicationStatusResponse, error) {
	if appName == "" {
		return nil, apperrors.NewValidationError("appName is required")
	}

	url := fmt.Sprintf("%s"+EndpointApplicationGet, c.baseURL, appName)
	var statusResp ApplicationStatusResponse

	execErr := c.executor.Execute(ctx, true, func(opCtx context.Context) error {
		req, reqErr := http.NewRequestWithContext(opCtx, http.MethodGet, url, nil)
		if reqErr != nil {
			return apperrors.NewConflictError("failed to construct http request: " + reqErr.Error())
		}

		c.attachHeaders(req, "")

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return apperrors.NewDownstreamUnavailableError("argocd api network failure: " + respErr.Error())
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return c.mapHTTPError(resp)
		}

		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return apperrors.NewDownstreamUnavailableError("failed to read response body: " + readErr.Error())
		}

		if parseErr := json.Unmarshal(bodyBytes, &statusResp); parseErr != nil {
			return apperrors.NewDownstreamUnavailableError("failed to parse argocd response: " + parseErr.Error())
		}

		return nil
	})

	if execErr != nil {
		return nil, execErr
	}

	return &statusResp, nil
}

func (c *ArgoCDClient) attachHeaders(req *http.Request, idempotencyKey string) {
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.Header.Set(constants.HeaderAccept, constants.ContentTypeJSON)
	if c.authToken != "" {
		req.Header.Set(constants.HeaderAuthorization, "Bearer "+c.authToken)
	}
	if idempotencyKey != "" {
		req.Header.Set(constants.HeaderXIdempotencyKey, idempotencyKey)
	}
}

func (c *ArgoCDClient) mapHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	msg := fmt.Sprintf("argocd api error HTTP %d: %s", resp.StatusCode, string(body))

	switch resp.StatusCode {
	case http.StatusNotFound:
		return apperrors.NewNotFoundError(msg)
	case http.StatusConflict:
		return apperrors.NewConflictError(msg)
	case http.StatusUnauthorized:
		return apperrors.NewUnauthorizedError(msg)
	case http.StatusForbidden:
		return apperrors.NewForbiddenError(msg)
	case http.StatusTooManyRequests:
		return apperrors.NewDownstreamRateLimitedError(msg)
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return apperrors.NewDownstreamUnavailableError(msg)
	default:
		return apperrors.NewDownstreamUnavailableError(msg)
	}
}

func (c *ArgoCDClient) Resilience() *resilience.ResilienceExecutor {
	return c.executor
}
