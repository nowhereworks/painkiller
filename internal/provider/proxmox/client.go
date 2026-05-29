package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	config Config
	http   *http.Client
}

func NewClient(config Config) *Client {
	return &Client{
		config: config,
		http:   &http.Client{},
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
		return nil, fmt.Errorf("proxmox API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) CloneVM(ctx context.Context, templateVMID int, newVMID int, name string) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/clone", c.config.Node, templateVMID)
	body := map[string]interface{}{
		"newid":   newVMID,
		"name":    name,
		"storage": c.config.StoragePool,
		"full":    1,
	}
	_, err := c.doRequest(ctx, http.MethodPost, path, body)
	return err
}

func (c *Client) ConfigureVM(ctx context.Context, vmID int, config map[string]string) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/config", c.config.Node, vmID)
	formData := url.Values{}
	for k, v := range config {
		formData.Set(k, v)
	}

	reqURL := fmt.Sprintf("%s/api2/json%s", c.config.APIURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.config.TokenID, c.config.TokenSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("proxmox API error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) StartVM(ctx context.Context, vmID int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/start", c.config.Node, vmID)
	_, err := c.doRequest(ctx, http.MethodPost, path, nil)
	return err
}

func (c *Client) StopVM(ctx context.Context, vmID int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", c.config.Node, vmID)
	_, err := c.doRequest(ctx, http.MethodPost, path, nil)
	return err
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
