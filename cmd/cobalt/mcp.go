package main

import (
	"os"

	"github.com/basalt-ai/cobalt-go/pkg/mcp"
)

// runMCPServer starts the MCP server over stdio.
func runMCPServer() {
	srv := mcp.New(os.Stdin, os.Stdout)
	if err := srv.Serve(rootCmd.Context()); err != nil {
		os.Exit(1)
	}
}
