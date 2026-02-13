package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
)

// MCPProxy connects MCP tool calls to the REST API on vire-server.
type MCPProxy struct {
	serverURL   string
	httpClient  *http.Client
	logger      *common.Logger
	userHeaders http.Header
}

// NewMCPProxy creates a new MCP proxy targeting the given server URL.
// User and Navexa config are converted to X-Vire-* headers injected on every request.
func NewMCPProxy(serverURL string, logger *common.Logger, userCfg UserConfig, navexaCfg NavexaConfig) *MCPProxy {
	headers := make(http.Header)
	if len(userCfg.Portfolios) > 0 {
		headers.Set("X-Vire-Portfolios", strings.Join(userCfg.Portfolios, ","))
	}
	if userCfg.DisplayCurrency != "" {
		headers.Set("X-Vire-Display-Currency", userCfg.DisplayCurrency)
	}
	if navexaCfg.APIKey != "" {
		headers.Set("X-Vire-Navexa-Key", navexaCfg.APIKey)
	}

	return &MCPProxy{
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: 300 * time.Second, // Match server WriteTimeout
		},
		logger:      logger,
		userHeaders: headers,
	}
}

// applyUserHeaders copies user context headers onto an outgoing request.
func (p *MCPProxy) applyUserHeaders(req *http.Request) {
	for key, vals := range p.userHeaders {
		for _, v := range vals {
			req.Header.Set(key, v)
		}
	}
}

// get performs a GET request and returns the response body.
func (p *MCPProxy) get(path string) ([]byte, error) {
	// Log request (Debug)
	p.logger.Debug().
		Str("method", "GET").
		Str("path", path).
		Msg("MCP Proxy Request")

	req, err := http.NewRequest(http.MethodGet, p.serverURL+path, nil)
	if err != nil {
		return nil, err
	}
	p.applyUserHeaders(req)

	start := time.Now()
	resp, err := p.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		p.logger.Error().Err(err).Str("path", path).Dur("duration", duration).Msg("MCP Proxy Request Failed")
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log response (Debug)
	p.logger.Debug().
		Str("status", resp.Status).
		Int("status_code", resp.StatusCode).
		Dur("duration", duration).
		Str("response", string(body)).
		Msg("MCP Proxy Response")

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
	// Log request (Debug)
	p.logger.Debug().
		Str("method", "DELETE").
		Str("path", path).
		Msg("MCP Proxy Request")

	req, err := http.NewRequest(http.MethodDelete, p.serverURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.applyUserHeaders(req)

	start := time.Now()
	resp, err := p.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		p.logger.Error().Err(err).Str("path", path).Dur("duration", duration).Msg("MCP Proxy Request Failed")
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log response (Debug)
	p.logger.Debug().
		Str("status", resp.Status).
		Int("status_code", resp.StatusCode).
		Dur("duration", duration).
		Str("response", string(body)).
		Msg("MCP Proxy Response")

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
	// Log request (Debug)
	p.logger.Debug().
		Str("method", method).
		Str("path", path).
		Str("data", fmt.Sprintf("%v", data)).
		Msg("MCP Proxy Request")

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
	p.applyUserHeaders(req)

	start := time.Now()
	resp, err := p.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		p.logger.Error().Err(err).Str("path", path).Dur("duration", duration).Msg("MCP Proxy Request Failed")
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log response (Debug)
	p.logger.Debug().
		Str("status", resp.Status).
		Int("status_code", resp.StatusCode).
		Dur("duration", duration).
		Str("response", string(body)). // Be careful with large bodies
		Msg("MCP Proxy Response")

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
