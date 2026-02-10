package app

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/common"
)

// testHarness provides an in-process MCP client connected to a Vire server
// with mock services. Tests can configure mock behavior before calling tools.
type testHarness struct {
	t             *testing.T
	client        *client.Client
	mcpServer     *server.MCPServer
	mockPortfolio *mockPortfolioService
	mockStorage   *mockStorageManager
	logger        *common.Logger
}

// newTestHarness creates a Vire MCP server with mock services and an in-process client.
// The client is already initialized and ready to call tools.
func newTestHarness(t *testing.T) *testHarness {
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
	mcpServer.AddTool(createGetPortfolioTool(), handleGetPortfolio(mockPS, mockSM, defaultPortfolio, logger))
	mcpServer.AddTool(createGetPortfolioHistoryTool(), handleGetPortfolioHistory(mockPS, mockSM, defaultPortfolio, logger))
	mcpServer.AddTool(createSetDefaultPortfolioTool(), handleSetDefaultPortfolio(mockSM, mockPS, defaultPortfolio, logger))
	mcpServer.AddTool(createSyncPortfolioTool(), handleSyncPortfolio(mockPS, mockSM, defaultPortfolio, logger))
	mcpServer.AddTool(createListPortfoliosTool(), handleListPortfolios(mockPS, logger))

	// Create in-process client
	c, err := client.NewInProcessClient(mcpServer)
	if err != nil {
		t.Fatalf("Failed to create in-process client: %v", err)
	}

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}

	// Initialize MCP protocol handshake
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "claude-desktop-test",
		Version: "1.0.0",
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		c.Close()
		t.Fatalf("Failed to initialize MCP: %v", err)
	}

	h := &testHarness{
		t:             t,
		client:        c,
		mcpServer:     mcpServer,
		mockPortfolio: mockPS,
		mockStorage:   mockSM,
		logger:        logger,
	}

	t.Cleanup(h.close)
	return h
}

// callTool invokes an MCP tool by name with the given arguments.
func (h *testHarness) callTool(name string, args map[string]any) (*mcp.CallToolResult, error) {
	h.t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return h.client.CallTool(context.Background(), req)
}

// getTextContent extracts text from a content block at the given index.
// Fails the test if index is out of range or content is not text.
func (h *testHarness) getTextContent(result *mcp.CallToolResult, index int) string {
	h.t.Helper()
	if index >= len(result.Content) {
		h.t.Fatalf("Content index %d out of range (have %d blocks)", index, len(result.Content))
	}
	tc, ok := result.Content[index].(mcp.TextContent)
	if !ok {
		h.t.Fatalf("Content[%d] is %T, not TextContent", index, result.Content[index])
	}
	return tc.Text
}

func (h *testHarness) close() {
	if h.client != nil {
		h.client.Close()
	}
}
