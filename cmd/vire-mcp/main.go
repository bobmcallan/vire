package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/viper"

	"github.com/bobmcallan/vire/internal/common"
)

type ServerConfig struct {
	Name      string `mapstructure:"name"`
	Port      string `mapstructure:"port"`
	ServerURL string `mapstructure:"server_url"`
}

type Config struct {
	Server  ServerConfig         `mapstructure:"server"`
	Logging common.LoggingConfig `mapstructure:"logging"`
}

var cfg Config

func init() {
	viper.SetConfigName("vire-mcp")
	viper.SetConfigType("toml")
	viper.AddConfigPath(filepath.Join(".", "vire-mcp"))
	viper.AddConfigPath("/etc/vire-mcp")
	viper.AddConfigPath("$HOME/.config/vire-mcp/vire-mcp")

	// Set defaults
	viper.SetDefault("server.name", "Vire-MCP")
	viper.SetDefault("server.port", "4243")
	viper.SetDefault("server.server_url", "http://vire-server:4242")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.outputs", []string{"console", "file"})
	viper.SetDefault("logging.file_path", "logs/vire-mcp.log")
	viper.SetDefault("logging.max_size_mb", 100)
	viper.SetDefault("logging.max_backups", 3)

	// Read config files (in order, last one wins)
	viper.ReadInConfig()

	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}
}

func main() {
	stdio := flag.Bool("stdio", false, "Use stdio transport (for Claude Desktop)")
	flag.Parse()

	// Load version
	common.LoadVersionFromFile()

	// Setup logging
	logger := common.NewLoggerFromConfig(cfg.Logging)

	serverURL := cfg.Server.ServerURL
	proxy := NewMCPProxy(serverURL, logger)

	// Create MCP server with tool definitions
	mcpServer := server.NewMCPServer(
		cfg.Server.Name,
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
		return
	}

	port := cfg.Server.Port

	// Streamable HTTP transport — listens on configured port
	httpServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithStateLess(true),
	)

	log.Printf("Starting MCP Streamable HTTP on :%s", port)
	fmt.Fprintf(os.Stderr, "Starting MCP Streamable HTTP on :%s\n", port)

	if err := httpServer.Start(":" + port); err != nil {
		fmt.Fprintf(os.Stderr, "http server error: %v\n", err)
		os.Exit(1)
	}
}
