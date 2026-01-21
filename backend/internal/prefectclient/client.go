// Package prefectclient provides a client for interacting with Prefect Server API.
package prefectclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a Prefect API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Prefect client.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// DeploymentInfo represents a Prefect Deployment.
type DeploymentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DeploymentDetails represents detailed information about a Prefect Deployment.
type DeploymentDetails struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Version           string                 `json:"version,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Tags              []string               `json:"tags,omitempty"`
	Parameters        map[string]interface{} `json:"parameters,omitempty"`
	ParameterSchema   map[string]interface{} `json:"parameter_openapi_schema,omitempty"`
	FlowID            string                 `json:"flow_id,omitempty"`
	Entrypoint        string                 `json:"entrypoint,omitempty"`
	WorkPoolName      string                 `json:"work_pool_name,omitempty"`
	CreatedAt         string                 `json:"created,omitempty"`
	UpdatedAt         string                 `json:"updated,omitempty"`
}

// FlowRunResponse represents a Prefect Flow Run.
type FlowRunResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	State     *StateResponse `json:"state,omitempty"`
	StateType string         `json:"state_type,omitempty"`
}

// StateResponse represents the state of a flow run.
type StateResponse struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Message string `json:"message,omitempty"`
}

// GetDeploymentByName finds a deployment by flow name and deployment name.
func (c *Client) GetDeploymentByName(ctx context.Context, flowName, deploymentName string) (*DeploymentInfo, error) {
	url := fmt.Sprintf("%s/api/deployments/filter", c.baseURL)

	// Prefect 3.x filter API
	body := map[string]interface{}{
		"deployments": map[string]interface{}{
			"name": map[string]interface{}{
				"any_": []string{deploymentName},
			},
		},
		"limit": 10,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deployment query failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var deployments []DeploymentInfo
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(deployments) == 0 {
		return nil, fmt.Errorf("deployment not found: %s", deploymentName)
	}

	return &deployments[0], nil
}

// CreateFlowRun creates a new flow run for a deployment.
func (c *Client) CreateFlowRun(ctx context.Context, deploymentID string, params map[string]interface{}) (*FlowRunResponse, error) {
	url := fmt.Sprintf("%s/api/deployments/%s/create_flow_run", c.baseURL, deploymentID)

	body := map[string]interface{}{}
	if params != nil {
		body["parameters"] = params
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create flow run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create flow run failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var flowRun FlowRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&flowRun); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &flowRun, nil
}

// GetFlowRun gets the status of a flow run.
func (c *Client) GetFlowRun(ctx context.Context, flowRunID string) (*FlowRunResponse, error) {
	url := fmt.Sprintf("%s/api/flow_runs/%s", c.baseURL, flowRunID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get flow run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get flow run failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var flowRun FlowRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&flowRun); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &flowRun, nil
}

// HealthCheck checks if the Prefect server is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/health", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("prefect server unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prefect server health check failed: status %d", resp.StatusCode)
	}

	return nil
}

// ListDeployments lists all deployments, optionally filtered by tags.
func (c *Client) ListDeployments(ctx context.Context, tagFilters []string) ([]DeploymentDetails, error) {
	url := fmt.Sprintf("%s/api/deployments/filter", c.baseURL)

	// Build filter body
	body := map[string]interface{}{
		"limit":  100, // Get up to 100 deployments
		"offset": 0,
	}

	// Add tag filter if specified
	if len(tagFilters) > 0 {
		body["deployments"] = map[string]interface{}{
			"tags": map[string]interface{}{
				"any_": tagFilters,
			},
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list deployments failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var deployments []DeploymentDetails
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return deployments, nil
}

// GetDeployment gets a single deployment by ID.
func (c *Client) GetDeployment(ctx context.Context, deploymentID string) (*DeploymentDetails, error) {
	url := fmt.Sprintf("%s/api/deployments/%s", c.baseURL, deploymentID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get deployment failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var deployment DeploymentDetails
	if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &deployment, nil
}
