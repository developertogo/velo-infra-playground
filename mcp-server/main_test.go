package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// Helper to extract text from a CallToolResult content safely.
func getResponseText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	switch c := result.Content[0].(type) {
	case mcp.TextContent:
		return c.Text
	case *mcp.TextContent:
		return c.Text
	}
	return ""
}

func TestDiagnosticsEngineMock(t *testing.T) {
	// Initialize in mock mode
	engine = NewDiagnosticsEngine(true)

	// 1. Test listing containers
	containersText, err := engine.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("Expected no error from ListContainers, got: %v", err)
	}
	if !strings.Contains(containersText, "triton-inference-server") || !strings.Contains(containersText, "velo-sentinel-gateway") {
		t.Errorf("Expected mock containers in list, got: %s", containersText)
	}

	// 2. Test tailing Triton logs
	tritonLogs, err := engine.TailLogs(context.Background(), "triton-inference-server", 5)
	if err != nil {
		t.Fatalf("Expected no error from TailLogs, got: %v", err)
	}
	if !strings.Contains(tritonLogs, "Triton Inference Server") || !strings.Contains(tritonLogs, "loaded model 'resnet50_onnx'") {
		t.Errorf("Expected Triton-specific logs, got: %s", tritonLogs)
	}

	// 3. Test tailing Sentinel logs
	sentinelLogs, err := engine.TailLogs(context.Background(), "velo-sentinel-gateway", 5)
	if err != nil {
		t.Fatalf("Expected no error from TailLogs, got: %v", err)
	}
	if !strings.Contains(sentinelLogs, "Sentinel Gateway starting") || !strings.Contains(sentinelLogs, "Triton is reachable") {
		t.Errorf("Expected Sentinel-specific logs, got: %s", sentinelLogs)
	}

	// 4. Test listing pods
	podsText, err := engine.ListPods(context.Background(), "custom-ns")
	if err != nil {
		t.Fatalf("Expected no error from ListPods, got: %v", err)
	}
	if !strings.Contains(podsText, "triton-worker-0") || !strings.Contains(podsText, "velo-sentinel") {
		t.Errorf("Expected mock pods list, got: %s", podsText)
	}
}

func TestToolHandlers(t *testing.T) {
	// Setup mock engine
	engine = NewDiagnosticsEngine(true)

	// 1. Test listContainersHandler
	reqList := mcp.CallToolRequest{}
	resList, err := listContainersHandler(context.Background(), reqList)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	resText := getResponseText(resList)
	if !strings.Contains(resText, "triton-inference-server") {
		t.Errorf("Unexpected result from listContainersHandler: %s", resText)
	}

	// 2. Test tailLogsHandler with valid arguments
	reqLogsValid := mcp.CallToolRequest{}
	reqLogsValid.Params.Arguments = map[string]interface{}{
		"name":  "triton-inference-server",
		"lines": 10.0, // json numbers are parsed as float64
	}
	resLogs, err := tailLogsHandler(context.Background(), reqLogsValid)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	resLogsText := getResponseText(resLogs)
	if !strings.Contains(resLogsText, "Triton Inference Server") {
		t.Errorf("Unexpected result from tailLogsHandler: %s", resLogsText)
	}

	// 3. Test tailLogsHandler with missing required name argument
	reqLogsInvalid := mcp.CallToolRequest{}
	reqLogsInvalid.Params.Arguments = map[string]interface{}{
		"lines": 10.0,
	}
	resLogsBad, err := tailLogsHandler(context.Background(), reqLogsInvalid)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	if !resLogsBad.IsError {
		t.Error("Expected error result for missing name parameter")
	}
	resLogsBadText := getResponseText(resLogsBad)
	if !strings.Contains(resLogsBadText, "missing required argument") {
		t.Errorf("Expected missing argument error message, got: %s", resLogsBadText)
	}

	// 4. Test verifyPodsHandler with optional namespace
	reqPods := mcp.CallToolRequest{}
	reqPods.Params.Arguments = map[string]interface{}{
		"namespace": "test-namespace",
	}
	resPods, err := verifyPodsHandler(context.Background(), reqPods)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	resPodsText := getResponseText(resPods)
	if !strings.Contains(resPodsText, "test-namespace") {
		t.Errorf("Expected namespace 'test-namespace' in output, got: %s", resPodsText)
	}
}

func TestToolHandlersInvalidArguments(t *testing.T) {
	engine = NewDiagnosticsEngine(true)

	// 1. Test tailLogsHandler with name of wrong type (e.g. integer)
	reqLogsBadType := mcp.CallToolRequest{}
	reqLogsBadType.Params.Arguments = map[string]interface{}{
		"name": 123.0, // name should be a string
	}
	res, err := tailLogsHandler(context.Background(), reqLogsBadType)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	if !res.IsError {
		t.Error("Expected error result for invalid name type")
	}
	resText := getResponseText(res)
	if !strings.Contains(resText, "must be a string") {
		t.Errorf("Expected 'must be a string' error, got: %s", resText)
	}

	// 2. Test verifyPodsHandler with namespace of wrong type (e.g. boolean)
	reqPodsBadType := mcp.CallToolRequest{}
	reqPodsBadType.Params.Arguments = map[string]interface{}{
		"namespace": true,
	}
	resPods, err := verifyPodsHandler(context.Background(), reqPodsBadType)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	resPodsText := getResponseText(resPods)
	// It should default back to "default" if namespace type assertion fails
	if !strings.Contains(resPodsText, "default") {
		t.Errorf("Expected namespace to fall back to 'default', got: %s", resPodsText)
	}
}

func TestLogTailingEdgeCases(t *testing.T) {
	engine = NewDiagnosticsEngine(true)

	// 1. Request logs for an unknown container name
	unknownLogs, err := engine.TailLogs(context.Background(), "unknown-container", 10)
	if err != nil {
		t.Fatalf("Expected no error from TailLogs for unknown container in mock mode, got: %v", err)
	}
	if !strings.Contains(unknownLogs, "unknown-container") {
		t.Errorf("Expected logs to reference container name, got: %s", unknownLogs)
	}

	// 2. Test tailLogsHandler with alternative int lines argument type
	reqIntLines := mcp.CallToolRequest{}
	reqIntLines.Params.Arguments = map[string]interface{}{
		"name":  "triton-inference-server",
		"lines": 25, // pure int instead of float64
	}
	res, err := tailLogsHandler(context.Background(), reqIntLines)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	resText := getResponseText(res)
	if !strings.Contains(resText, "Triton Inference Server") {
		t.Errorf("Unexpected result: %s", resText)
	}
}

func TestEngineInitBehavior(t *testing.T) {
	// Initialize in mock mode
	engineMock := NewDiagnosticsEngine(true)
	if engineMock.dockerCli != nil || engineMock.kubeClient != nil {
		t.Error("Expected clients to be nil in mock mode")
	}

	// Initialize in live mode (will fallback automatically if local daemons are not running)
	// This tests that client connection logic is executed without crashing
	engineLive := NewDiagnosticsEngine(false)
	if engineLive == nil {
		t.Fatal("Expected NewDiagnosticsEngine to return a non-nil pointer in live mode")
	}
}

