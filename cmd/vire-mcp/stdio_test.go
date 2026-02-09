package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/common"
)

// newStdioTestHarness creates a Vire MCP server with mock services connected
// via stdio transport (io.Pipe). This tests the actual JSON-RPC over stdin/stdout
// path that Claude Desktop and Claude CLI will use.
func newStdioTestHarness(t *testing.T) *testHarness {
	t.Helper()

	logger := common.NewLogger("error")
	mockPS := &mockPortfolioService{}
	mockSM := newMockStorageManager()

	mcpServer := server.NewMCPServer(
		"vire-test",
		"test",
		server.WithToolCapabilities(true),
	)

	// Register tools under test
	defaultPortfolio := "TestSMSF"
	mcpServer.AddTool(createGetVersionTool(), handleGetVersion())
	mcpServer.AddTool(createGetPortfolioHistoryTool(), handleGetPortfolioHistory(mockPS, mockSM, defaultPortfolio, logger))
	mcpServer.AddTool(createSetDefaultPortfolioTool(), handleSetDefaultPortfolio(mockSM, mockPS, defaultPortfolio, logger))

	// Create stdio server and connect via pipes
	stdioServer := server.NewStdioServer(mcpServer)

	// Pipe layout:
	//   clientOut -> serverIn  (client writes, server reads stdin)
	//   serverOut -> clientIn  (server writes stdout, client reads)
	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()

	// Start stdio server in background
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- stdioServer.Listen(ctx, serverIn, serverOut)
	}()

	// Create client transport using NewIO (connects to existing pipes, no subprocess)
	// NewIO(input=what client reads, output=what client writes, logging=stderr)
	stdioTransport := transport.NewIO(clientIn, clientOut, io.NopCloser(strings.NewReader("")))
	if err := stdioTransport.Start(context.Background()); err != nil {
		cancel()
		t.Fatalf("Failed to start stdio transport: %v", err)
	}

	c := client.NewClient(stdioTransport)

	// Initialize MCP protocol handshake
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "claude-desktop-test",
		Version: "1.0.0",
	}
	initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer initCancel()
	if _, err := c.Initialize(initCtx, initReq); err != nil {
		cancel()
		c.Close()
		t.Fatalf("Failed to initialize MCP via stdio: %v", err)
	}

	h := &testHarness{
		t:             t,
		client:        c,
		mcpServer:     mcpServer,
		mockPortfolio: mockPS,
		mockStorage:   mockSM,
		logger:        logger,
	}

	t.Cleanup(func() {
		c.Close()
		cancel()
		// Drain server error
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
		}
	})

	return h
}

// TestStdio_InitializeAndVersion verifies the stdio transport can handle
// the MCP initialize handshake and a simple tool call.
func TestStdio_InitializeAndVersion(t *testing.T) {
	h := newStdioTestHarness(t)

	result, err := h.callTool("get_version", nil)
	if err != nil {
		t.Fatalf("get_version over stdio failed: %v", err)
	}

	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "Vire MCP Server") {
		t.Errorf("Expected version output to contain 'Vire MCP Server', got: %s", text)
	}
	if !strings.Contains(text, "Status: OK") {
		t.Errorf("Expected 'Status: OK' in version output")
	}
}

// TestStdio_ListTools verifies tool discovery works over stdio transport.
func TestStdio_ListTools(t *testing.T) {
	h := newStdioTestHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	toolsResult, err := h.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools over stdio failed: %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{"get_version", "get_portfolio_history", "set_default_portfolio"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("Expected tool '%s' not found in listTools response", name)
		}
	}
}

// TestStdio_ToolCallWithArguments verifies tool calls with parameters work over stdio.
func TestStdio_ToolCallWithArguments(t *testing.T) {
	h := newStdioTestHarness(t)

	h.mockPortfolio.listPortfoliosFn = func(ctx context.Context) ([]string, error) {
		return []string{"SMSF", "Personal"}, nil
	}

	result, err := h.callTool("set_default_portfolio", map[string]any{
		"portfolio_name": "Personal",
	})
	if err != nil {
		t.Fatalf("set_default_portfolio over stdio failed: %v", err)
	}

	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "Personal") {
		t.Errorf("Expected 'Personal' in response, got: %s", text)
	}
}

// TestStdio_MultipleSequentialCalls verifies multiple tool calls work in sequence
// over the same stdio connection without corruption.
func TestStdio_MultipleSequentialCalls(t *testing.T) {
	h := newStdioTestHarness(t)

	for i := 0; i < 5; i++ {
		result, err := h.callTool("get_version", nil)
		if err != nil {
			t.Fatalf("Call %d: get_version failed: %v", i, err)
		}

		text := h.getTextContent(result, 0)
		if !strings.Contains(text, "Vire MCP Server") {
			t.Errorf("Call %d: unexpected response: %s", i, text)
		}
	}
}

// TestStdio_GracefulShutdownOnStdinClose verifies the server exits cleanly
// when stdin is closed (simulating the client disconnecting).
func TestStdio_GracefulShutdownOnStdinClose(t *testing.T) {
	mcpServer := server.NewMCPServer(
		"vire-test",
		"test",
		server.WithToolCapabilities(true),
	)
	mcpServer.AddTool(createGetVersionTool(), handleGetVersion())

	stdioServer := server.NewStdioServer(mcpServer)

	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()

	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		errCh <- stdioServer.Listen(ctx, serverIn, serverOut)
	}()

	// Drain server output so writes don't block
	go func() {
		io.Copy(io.Discard, clientIn)
	}()

	// Send an initialize request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test",
				"version": "1.0.0",
			},
		},
	}
	reqBytes, _ := json.Marshal(initReq)
	reqBytes = append(reqBytes, '\n')
	clientOut.Write(reqBytes)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Close stdin — server should exit cleanly
	clientOut.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Server returned error on stdin close (expected nil): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Server did not exit within 5s after stdin close")
	}
}

// TestStdio_ConcurrentToolCalls verifies that multiple goroutines can make
// tool calls simultaneously over the same stdio connection without responses
// getting cross-wired. This simulates Claude sending parallel tool calls.
func TestStdio_ConcurrentToolCalls(t *testing.T) {
	h := newStdioTestHarness(t)

	h.mockPortfolio.listPortfoliosFn = func(ctx context.Context) ([]string, error) {
		return []string{"SMSF", "Personal"}, nil
	}

	const goroutines = 10
	const callsPerGoroutine = 5

	var wg sync.WaitGroup
	errors := make(chan error, goroutines*callsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for c := 0; c < callsPerGoroutine; c++ {
				// Alternate between two different tools to increase the chance
				// of cross-wiring if there's a multiplexing bug
				if c%2 == 0 {
					result, err := h.callTool("get_version", nil)
					if err != nil {
						errors <- fmt.Errorf("goroutine %d call %d (get_version): %w", id, c, err)
						continue
					}
					text := result.Content[0].(mcp.TextContent).Text
					if !strings.Contains(text, "Vire MCP Server") {
						errors <- fmt.Errorf("goroutine %d call %d: expected 'Vire MCP Server', got: %s", id, c, text[:min(len(text), 100)])
					}
				} else {
					result, err := h.callTool("set_default_portfolio", map[string]any{
						"portfolio_name": "Personal",
					})
					if err != nil {
						errors <- fmt.Errorf("goroutine %d call %d (set_default_portfolio): %w", id, c, err)
						continue
					}
					text := result.Content[0].(mcp.TextContent).Text
					if !strings.Contains(text, "Personal") {
						errors <- fmt.Errorf("goroutine %d call %d: expected 'Personal', got: %s", id, c, text[:min(len(text), 100)])
					}
				}
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}
