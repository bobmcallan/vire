package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/common"
)

func main() {
	stdio := flag.Bool("stdio", false, "Use stdio transport (for Claude Desktop)")
	serverFlag := flag.String("server", "", "Vire server URL (default: $VIRE_SERVER_URL or http://localhost:4242)")
	flag.Parse()

	// Load version
	common.LoadVersionFromFile()

	serverURL := *serverFlag
	if serverURL == "" {
		serverURL = os.Getenv("VIRE_SERVER_URL")
	}
	if serverURL == "" {
		serverURL = "http://localhost:4242"
	}

	proxy := NewMCPProxy(serverURL)

	// Create MCP server with tool definitions
	mcpServer := server.NewMCPServer(
		"vire",
		common.GetVersion(),
		server.WithToolCapabilities(true),
	)

	// Register all MCP tools
	registerTools(mcpServer, proxy)

	if *stdio {
		// Stdio transport — reads stdin, writes stdout
		if err := server.ServeStdio(mcpServer); err != nil {
			fmt.Fprintf(os.Stderr, "stdio server error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Streamable HTTP transport — listens on :4243
		port := os.Getenv("VIRE_MCP_PORT")
		if port == "" {
			port = "4243"
		}

		httpServer := server.NewStreamableHTTPServer(mcpServer,
			server.WithStateLess(true),
		)

		fmt.Fprintf(os.Stderr, "Starting MCP Streamable HTTP on :%s\n", port)
		if err := httpServer.Start(":" + port); err != nil {
			fmt.Fprintf(os.Stderr, "http server error: %v\n", err)
			os.Exit(1)
		}
	}
}
