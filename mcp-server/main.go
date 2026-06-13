package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var engine *DiagnosticsEngine

func main() {
	mockFlag := flag.Bool("mock", false, "Force mock fallback mode")
	flag.Parse()

	// Initialize backend engine
	engine = NewDiagnosticsEngine(*mockFlag)

	// Create MCP server
	s := server.NewMCPServer(
		"Infrastructure Diagnostics Server",
		"1.0.0",
	)

	// 1. Tool: List Docker Containers
	listContainersTool := mcp.NewTool("list_docker_containers",
		mcp.WithDescription("List active and stopped Docker containers in the local runtime"),
	)
	s.AddTool(listContainersTool, listContainersHandler)

	// 2. Tool: Tail Container Logs
	tailLogsTool := mcp.NewTool("tail_container_logs",
		mcp.WithDescription("Tail log streams for a specific container (e.g. Triton, Sentinel)"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name or ID of the container"),
		),
		mcp.WithNumber("lines",
			mcp.Description("Number of lines to tail (default: 50)"),
		),
	)
	s.AddTool(tailLogsTool, tailLogsHandler)

	// 3. Tool: Verify Kubernetes Pods
	verifyPodsTool := mcp.NewTool("verify_k8s_pods",
		mcp.WithDescription("List Kubernetes pods and status in a specific namespace"),
		mcp.WithString("namespace",
			mcp.Description("Namespace to query (default: 'default')"),
		),
	)
	s.AddTool(verifyPodsTool, verifyPodsHandler)

	// Start Stdio listener
	fmt.Fprintf(os.Stderr, "[MCP Server] Initialized. Listening on STDIO...\n")
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "[MCP Server] Run error: %v\n", err)
		os.Exit(1)
	}
}

// Handlers

func listContainersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	res, err := engine.ListContainers(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(res), nil
}

func tailLogsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	nameVal, exists := request.Params.Arguments["name"]
	if !exists {
		return mcp.NewToolResultError("missing required argument: name"), nil
	}
	name, ok := nameVal.(string)
	if !ok {
		return mcp.NewToolResultError("argument 'name' must be a string"), nil
	}

	lines := 50
	linesVal, exists := request.Params.Arguments["lines"]
	if exists {
		switch v := linesVal.(type) {
		case float64:
			lines = int(v)
		case int:
			lines = v
		}
	}

	res, err := engine.TailLogs(ctx, name, lines)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(res), nil
}

func verifyPodsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := "default"
	nsVal, exists := request.Params.Arguments["namespace"]
	if exists {
		if s, ok := nsVal.(string); ok {
			ns = s
		}
	}

	res, err := engine.ListPods(ctx, ns)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(res), nil
}
