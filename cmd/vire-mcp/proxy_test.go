package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestProxy_ForwardsJSONRPC verifies the stdio proxy reads JSON-RPC from stdin,
// POSTs to the server's /mcp endpoint, and writes the response to stdout.
func TestProxy_ForwardsJSONRPC(t *testing.T) {
	// Create a mock MCP server that echoes back a JSON-RPC response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/mcp" {
			t.Errorf("Expected path /mcp, got %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
			return
		}

		// Verify it's valid JSON-RPC
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("Request body is not valid JSON: %v", err)
			return
		}

		// Return a JSON-RPC response
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"serverInfo": map[string]interface{}{
					"name":    "vire",
					"version": "test",
				},
				"capabilities": map[string]interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	// Create proxy pointed at mock server
	proxy := &StdioProxy{serverURL: mockServer.URL + "/mcp"}

	// Set up stdin/stdout pipes
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	// Run proxy in background
	done := make(chan error, 1)
	go func() {
		done <- proxy.RunWithIO(stdinReader, stdoutWriter)
	}()

	// Send a JSON-RPC initialize request
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n"
	if _, err := io.WriteString(stdinWriter, initReq); err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}

	// Read the response from stdout
	scanner := bufio.NewScanner(stdoutReader)
	readDone := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			readDone <- scanner.Text()
		} else {
			readDone <- ""
		}
	}()

	select {
	case line := <-readDone:
		if line == "" {
			t.Fatal("Empty response from proxy")
		}

		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("Response is not valid JSON: %v (line: %s)", err, line)
		}

		if resp["jsonrpc"] != "2.0" {
			t.Errorf("Expected jsonrpc=2.0, got %v", resp["jsonrpc"])
		}
		if resp["result"] == nil {
			t.Error("Expected result in response, got nil")
		}

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for proxy response")
	}

	// Close stdin to signal EOF
	stdinWriter.Close()
}

// TestProxy_HandlesServerUnavailable verifies the proxy returns an error
// response when the server is not reachable.
func TestProxy_HandlesServerUnavailable(t *testing.T) {
	// Point proxy at a non-existent server
	proxy := &StdioProxy{serverURL: "http://localhost:1/mcp"}

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- proxy.RunWithIO(stdinReader, stdoutWriter)
	}()

	// Send a request
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	io.WriteString(stdinWriter, req)

	// Read response — should be a JSON-RPC error, not a crash
	scanner := bufio.NewScanner(stdoutReader)
	readDone := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			readDone <- scanner.Text()
		} else {
			readDone <- ""
		}
	}()

	select {
	case line := <-readDone:
		if line == "" {
			t.Fatal("Empty response — proxy may have crashed")
		}

		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("Response is not valid JSON: %v", err)
		}

		// Should contain an error field
		if resp["error"] == nil {
			t.Error("Expected JSON-RPC error response when server is unavailable")
		}

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout — proxy hung when server is unavailable")
	}

	stdinWriter.Close()
}

// TestProxy_ExitsOnStdinClose verifies the proxy exits cleanly when stdin closes.
func TestProxy_ExitsOnStdinClose(t *testing.T) {
	proxy := &StdioProxy{serverURL: "http://localhost:4242/mcp"}

	stdinReader, stdinWriter := io.Pipe()
	_, stdoutWriter := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- proxy.RunWithIO(stdinReader, stdoutWriter)
	}()

	// Close stdin immediately
	stdinWriter.Close()

	select {
	case err := <-done:
		// Should exit cleanly (nil or EOF-like error)
		if err != nil && !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "closed") {
			t.Errorf("Expected clean exit on stdin close, got error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Proxy did not exit within 5s after stdin close")
	}
}

// TestProxy_PreservesContentType verifies the proxy sets Content-Type: application/json
// on outgoing requests.
func TestProxy_PreservesContentType(t *testing.T) {
	ctChan := make(chan string, 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctChan <- r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{},
		})
	}))
	defer mockServer.Close()

	proxy := &StdioProxy{serverURL: mockServer.URL + "/mcp"}

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	go func() {
		proxy.RunWithIO(stdinReader, stdoutWriter)
	}()

	// Drain stdout
	go io.Copy(io.Discard, stdoutReader)

	req := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}` + "\n"
	io.WriteString(stdinWriter, req)

	select {
	case ct := <-ctChan:
		if ct != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %q", ct)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for proxy request to arrive at server")
	}

	stdinWriter.Close()
}
