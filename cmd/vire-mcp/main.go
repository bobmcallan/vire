package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// StdioProxy forwards JSON-RPC messages from stdin to an HTTP MCP server
// and writes responses to stdout.
type StdioProxy struct {
	serverURL  string
	httpClient *http.Client
}

func main() {
	serverURL := os.Getenv("VIRE_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:4242"
	}

	proxy := &StdioProxy{
		serverURL: serverURL + "/mcp",
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}

	if err := proxy.RunWithIO(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "proxy error: %v\n", err)
		os.Exit(1)
	}
}

// RunWithIO reads newline-delimited JSON-RPC from r, forwards each message
// to the HTTP server, and writes the response to w.
func (p *StdioProxy) RunWithIO(r io.Reader, w io.Writer) error {
	if p.httpClient == nil {
		p.httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	scanner := bufio.NewScanner(r)
	// Allow large messages (up to 10MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		resp, err := p.forward(line)
		if err != nil {
			// Extract the request ID if possible for the error response
			id := extractID(line)
			errResp := jsonRPCError(id, -32000, err.Error())
			w.Write(errResp)
			w.Write([]byte("\n"))
			continue
		}

		w.Write(resp)
		w.Write([]byte("\n"))
	}

	return scanner.Err()
}

// forward sends a JSON-RPC message to the HTTP server and returns the response body.
func (p *StdioProxy) forward(body []byte) ([]byte, error) {
	resp, err := p.httpClient.Post(p.serverURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("server request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	return bytes.TrimSpace(respBody), nil
}

// extractID pulls the "id" field from a JSON-RPC request for error responses.
func extractID(msg []byte) json.RawMessage {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(msg, &req); err != nil || req.ID == nil {
		return json.RawMessage("null")
	}
	return req.ID
}

// jsonRPCError creates a JSON-RPC error response.
func jsonRPCError(id json.RawMessage, code int, message string) []byte {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(resp)
	return data
}
