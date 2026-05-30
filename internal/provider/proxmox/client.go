package proxmox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	taskPollInterval  = 2 * time.Second
	taskPollTimeout   = 5 * time.Minute
	cloneVMIDAttempts = 5
)

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("proxmox API error (status %d): %s", e.StatusCode, e.Body)
}

type Client struct {
	config Config
	http   *http.Client
}

func NewClient(config Config) *Client {
	transport := &http.Transport{}
	if config.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		config: config,
		http:   &http.Client{Transport: transport},
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := fmt.Sprintf("%s/api2/json%s", c.config.APIURL, path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.config.TokenID, c.config.TokenSecret))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return respBody, nil
}

func (c *Client) doFormRequest(ctx context.Context, method, path string, form url.Values) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/api2/json%s", c.config.APIURL, path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.config.TokenID, c.config.TokenSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return respBody, nil
}

func (c *Client) waitForTask(ctx context.Context, upid string) error {
	parts := strings.Split(upid, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid UPID: %s", upid)
	}
	node := parts[1]

	deadline := time.Now().Add(taskPollTimeout)
	for time.Now().Before(deadline) {
		path := fmt.Sprintf("/nodes/%s/tasks/%s/status", node, url.PathEscape(upid))
		resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return fmt.Errorf("failed to poll task status: %w", err)
		}

		var result struct {
			Data struct {
				Status   string `json:"status"`
				ExitCode string `json:"exitstatus"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp, &result); err != nil {
			return fmt.Errorf("failed to parse task status: %w", err)
		}

		if result.Data.Status == "stopped" {
			if result.Data.ExitCode != "OK" {
				return fmt.Errorf("task failed with exit status: %s", result.Data.ExitCode)
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(taskPollInterval):
		}
	}

	return fmt.Errorf("timed out waiting for task %s", upid)
}

func (c *Client) NextVMID(ctx context.Context) (int, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/cluster/nextid", nil)
	if err != nil {
		return 0, err
	}

	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, fmt.Errorf("failed to parse next VMID response: %w", err)
	}

	var vmID int
	if err := json.Unmarshal(result.Data, &vmID); err == nil {
		return vmID, nil
	}

	var vmIDString string
	if err := json.Unmarshal(result.Data, &vmIDString); err != nil {
		return 0, fmt.Errorf("failed to parse next VMID value: %w", err)
	}
	vmID, err = strconv.Atoi(vmIDString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse next VMID value: %w", err)
	}

	return vmID, nil
}

func (c *Client) CloneVM(ctx context.Context, templateVMID int, name string, full bool) (int, error) {
	var lastErr error
	for attempt := 0; attempt < cloneVMIDAttempts; attempt++ {
		vmID, err := c.NextVMID(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get next VMID: %w", err)
		}

		if err := c.cloneVMWithID(ctx, templateVMID, vmID, name, full); err != nil {
			if !isVMIDConflict(err) {
				return 0, err
			}
			lastErr = err
			continue
		}

		return vmID, nil
	}

	return 0, fmt.Errorf("failed to clone VM after %d VMID allocation attempts: %w", cloneVMIDAttempts, lastErr)
}

func (c *Client) cloneVMWithID(ctx context.Context, templateVMID, newVMID int, name string, full bool) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/clone", c.config.Node, templateVMID)
	fullVal := 0
	if full {
		fullVal = 1
	}
	body := map[string]interface{}{
		"newid": newVMID,
		"name":  name,
		"full":  fullVal,
	}
	if full {
		body["storage"] = c.config.StoragePool
	}
	resp, err := c.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse clone response: %w", err)
	}
	if result.Data == "" {
		return fmt.Errorf("missing clone task UPID")
	}

	if err := c.waitForTask(ctx, result.Data); err != nil {
		return fmt.Errorf("clone task failed: %w", err)
	}

	return nil
}

func isVMIDConflict(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return messageLooksLikeVMIDConflict(apiErr.Body)
	}

	return messageLooksLikeVMIDConflict(err.Error())
}

func messageLooksLikeVMIDConflict(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "already exists") ||
		(strings.Contains(message, "vmid") && strings.Contains(message, "exists"))
}

func (c *Client) ConfigureVM(ctx context.Context, vmID int, config map[string]string) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/config", c.config.Node, vmID)
	formData := url.Values{}
	for k, v := range config {
		formData.Set(k, v)
	}

	_, err := c.doFormRequest(ctx, http.MethodPut, path, formData)
	return err
}

func (c *Client) StartVM(ctx context.Context, vmID int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/start", c.config.Node, vmID)
	resp, err := c.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil
	}
	if result.Data != "" {
		return c.waitForTask(ctx, result.Data)
	}
	return nil
}

func (c *Client) StopVM(ctx context.Context, vmID int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", c.config.Node, vmID)
	resp, err := c.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil
	}
	if result.Data != "" {
		return c.waitForTask(ctx, result.Data)
	}
	return nil
}

func (c *Client) DeleteVM(ctx context.Context, vmID int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d", c.config.Node, vmID)
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) GetVMStatus(ctx context.Context, vmID int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/current", c.config.Node, vmID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}

	return result.Data.Status, nil
}

func (c *Client) GetVMIPAddress(ctx context.Context, vmID int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/agent/network-get-interfaces", c.config.Node, vmID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("guest agent query failed: %w", err)
	}

	var result struct {
		Data struct {
			Result []struct {
				Name        string `json:"name"`
				IPAddresses []struct {
					IPAddress string `json:"ip-address"`
					Prefix    int    `json:"prefix"`
					Type      string `json:"ip-address-type"`
				} `json:"ip-addresses"`
				HardwareAddress string `json:"hardware-address"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("failed to parse guest agent response: %w", err)
	}

	for _, iface := range result.Data.Result {
		if iface.Name == "lo" {
			continue
		}
		for _, addr := range iface.IPAddresses {
			if addr.Type == "ipv4" && addr.IPAddress != "" && addr.IPAddress != "127.0.0.1" {
				return addr.IPAddress, nil
			}
		}
	}

	return "", fmt.Errorf("no IPv4 address found for VM %d", vmID)
}
