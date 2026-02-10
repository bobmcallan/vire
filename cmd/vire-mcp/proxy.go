package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MCPProxy connects MCP tool calls to the REST API on vire-server.
type MCPProxy struct {
	serverURL  string
	httpClient *http.Client
}

// NewMCPProxy creates a new MCP proxy targeting the given server URL.
func NewMCPProxy(serverURL string) *MCPProxy {
	return &MCPProxy{
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: 300 * time.Second, // Match server WriteTimeout
		},
	}
}

// get performs a GET request and returns the response body.
func (p *MCPProxy) get(path string) ([]byte, error) {
	resp, err := p.httpClient.Get(p.serverURL + path)
	if err != nil {
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// post performs a POST request with JSON body and returns the response body.
func (p *MCPProxy) post(path string, data interface{}) ([]byte, error) {
	return p.doJSON(http.MethodPost, path, data)
}

// put performs a PUT request with JSON body and returns the response body.
func (p *MCPProxy) put(path string, data interface{}) ([]byte, error) {
	return p.doJSON(http.MethodPut, path, data)
}

// patch performs a PATCH request with JSON body and returns the response body.
func (p *MCPProxy) patch(path string, data interface{}) ([]byte, error) {
	return p.doJSON(http.MethodPatch, path, data)
}

// del performs a DELETE request and returns the response body.
func (p *MCPProxy) del(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodDelete, p.serverURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	return body, nil
}

// doJSON performs an HTTP request with JSON body.
func (p *MCPProxy) doJSON(method, path string, data interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, p.serverURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
