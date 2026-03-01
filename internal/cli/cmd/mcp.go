package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/nolouch/opengocode/internal/config"
	"github.com/nolouch/opengocode/internal/mcp"
	"github.com/spf13/cobra"
)

func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP (Model Context Protocol) servers",
		Long:  "Manage MCP servers: list, add, authenticate, and remove connections",
	}

	cmd.AddCommand(mcpListCmd())
	cmd.AddCommand(mcpAuthCmd())
	cmd.AddCommand(mcpLogoutCmd())
	cmd.AddCommand(mcpStatusCmd())

	return cmd
}

func mcpListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured MCP servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			servers := cfg.MCPServers()
			if len(servers) == 0 {
				fmt.Println("No MCP servers configured")
				return nil
			}

			fmt.Println("Configured MCP servers:")
			fmt.Println()

			for name, config := range servers {
				status := "disabled"
				if config.Enabled {
					status = "enabled"
				}

				fmt.Printf("  %s (%s)\n", name, status)
				fmt.Printf("    Type: %s\n", config.Type)

				if config.Type == mcp.ServerTypeRemote {
					fmt.Printf("    URL: %s\n", config.URL)
					if config.OAuth != nil && config.OAuth.Enabled {
						fmt.Printf("    OAuth: enabled\n")
					}
				} else {
					fmt.Printf("    Command: %v\n", config.Command)
				}
				fmt.Println()
			}

			return nil
		},
	}
}

func mcpAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth [server-name]",
		Short: "Authenticate with an MCP server using OAuth",
		Long:  "Perform OAuth authentication flow for a remote MCP server",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			servers := cfg.MCPServers()

			// If no server name provided, list available servers
			if len(args) == 0 {
				fmt.Println("Available MCP servers:")
				for name, config := range servers {
					if config.Type == mcp.ServerTypeRemote && config.OAuth != nil {
						fmt.Printf("  - %s\n", name)
					}
				}
				fmt.Println("\nUsage: gcode mcp auth <server-name>")
				return nil
			}

			serverName := args[0]
			config, ok := servers[serverName]
			if !ok {
				return fmt.Errorf("server %q not found in configuration", serverName)
			}

			if config.Type != mcp.ServerTypeRemote {
				return fmt.Errorf("server %q is not a remote server", serverName)
			}

			if config.OAuth == nil || !config.OAuth.Enabled {
				return fmt.Errorf("server %q does not have OAuth configured", serverName)
			}

			fmt.Printf("Authenticating with %s...\n", serverName)
			token, err := mcp.Authenticate(serverName, config)
			if err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}

			fmt.Printf("✓ Successfully authenticated with %s\n", serverName)
			fmt.Printf("Access token: %s...\n", token[:min(20, len(token))])

			return nil
		},
	}

	return cmd
}

func mcpLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <server-name>",
		Short: "Remove stored OAuth credentials for an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverName := args[0]

			if err := mcp.RemoveAuth(serverName); err != nil {
				return fmt.Errorf("logout failed: %w", err)
			}

			fmt.Printf("✓ Removed credentials for %s\n", serverName)
			return nil
		},
	}
}

func mcpStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of all MCP servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			servers := cfg.MCPServers()
			if len(servers) == 0 {
				fmt.Println("No MCP servers configured")
				return nil
			}

			// Create manager to get tools
			mgr := mcp.NewManager(servers)
			ctx := context.Background()

			fmt.Println("MCP Server Status:")
			fmt.Println()

			for name, config := range servers {
				fmt.Printf("  %s\n", name)

				if !config.Enabled {
					fmt.Println("    Status: disabled")
					fmt.Println()
					continue
				}

				fmt.Printf("    Type: %s\n", config.Type)
				if config.Type == mcp.ServerTypeRemote {
					fmt.Printf("    URL: %s\n", config.URL)
				}
				fmt.Println()
			}

			// Show aggregated tool count
			allTools := mgr.Tools(ctx)
			fmt.Printf("Total tools available: %d\n", len(allTools))

			return nil
		},
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// loadConfig loads the configuration file
func loadConfig() (*config.Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	return config.Load(wd)
}
