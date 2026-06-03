/*
Copyright 2024 The CAPBM Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package lb

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta1"
)

// F5Provider implements the Provider interface for F5 BIG-IP.
type F5Provider struct {
	config     *capbmv1.F5Config
	baseURL    string
	httpClient *http.Client
}

// F5Member represents an F5 pool member.
type F5Member struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Name    string `json:"name,omitempty"`
}

// F5PoolMembersResponse represents the response from F5 pool members API.
type F5PoolMembersResponse struct {
	Items []F5PoolMemberItem `json:"items"`
}

// F5PoolMemberItem represents a single pool member item in the response.
type F5PoolMemberItem struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	State   string `json:"state,omitempty"`
}

// NewF5Provider creates a new F5 provider.
func NewF5Provider(config *capbmv1.F5Config) (Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("F5 configuration is required")
	}
	if config.Host == "" {
		return nil, fmt.Errorf("F5 host is required")
	}
	if config.Partition == "" {
		config.Partition = "Common"
	}
	if config.Port == 0 {
		config.Port = 443
	}

	baseURL := fmt.Sprintf("https://%s:%d/mgmt/tm", config.Host, config.Port)

	return &F5Provider{
		config:  config,
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}, nil
}

// RegisterBackend adds a backend server to the F5 pool.
func (p *F5Provider) RegisterBackend(ctx context.Context, backend Backend) error {
	member := F5Member{
		Address: backend.IP,
		Port:    backend.Port,
		Name:    backend.Name,
	}

	poolPath := fmt.Sprintf("/ltm/pool/~%s~%s/members", p.config.Partition, p.config.PoolName)
	return p.doF5Request(ctx, http.MethodPost, poolPath, member, nil)
}

// UnregisterBackend removes a backend server from the F5 pool.
func (p *F5Provider) UnregisterBackend(ctx context.Context, backend Backend) error {
	encodedMember := url.PathEscape(fmt.Sprintf("%s:%d", backend.IP, backend.Port))
	poolPath := fmt.Sprintf("/ltm/pool/~%s~%s/members/~%s~%s",
		p.config.Partition, p.config.PoolName, p.config.Partition, encodedMember)

	return p.doF5Request(ctx, http.MethodDelete, poolPath, nil, nil)
}

// GetBackends returns the current list of backend servers.
func (p *F5Provider) GetBackends(ctx context.Context) ([]Backend, error) {
	poolPath := fmt.Sprintf("/ltm/pool/~%s~%s/members", p.config.Partition, p.config.PoolName)

	var response F5PoolMembersResponse
	err := p.doF5Request(ctx, http.MethodGet, poolPath, nil, &response)
	if err != nil {
		return nil, err
	}

	var backends []Backend
	for _, item := range response.Items {
		backends = append(backends, Backend{
			Name: item.Name,
			IP:   item.Address,
			Port: item.Port,
		})
	}

	return backends, nil
}

// HealthCheck checks if a backend is healthy.
func (p *F5Provider) HealthCheck(ctx context.Context, backend Backend) (bool, error) {
	encodedMember := url.PathEscape(fmt.Sprintf("%s:%d", backend.IP, backend.Port))
	poolPath := fmt.Sprintf("/ltm/pool/~%s~%s/members/~%s~%s",
		p.config.Partition, p.config.PoolName, p.config.Partition, encodedMember)

	var item F5PoolMemberItem
	err := p.doF5Request(ctx, http.MethodGet, poolPath, nil, &item)
	if err != nil {
		return false, err
	}

	return item.State == "up" || item.State == "checking", nil
}

// doF5Request executes an HTTP request to the F5 BIG-IP iControl REST API.
func (p *F5Provider) doF5Request(ctx context.Context, method, path string, body, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	reqURL := p.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(p.config.CredentialsRef.Name, "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("F5 API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil && method != http.MethodDelete {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		if len(bodyBytes) > 0 && !strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "null") {
			if err := json.Unmarshal(bodyBytes, result); err != nil {
				return fmt.Errorf("failed to unmarshal response: %w", err)
			}
		}
	}

	return nil
}
