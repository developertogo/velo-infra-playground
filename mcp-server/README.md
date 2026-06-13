# Model Context Protocol Server (`mcp-server`)

This is a Model Context Protocol (MCP) server built in Go. It exposes container and Kubernetes cluster diagnostics tools directly to AI assistants (like Claude Desktop or local developer agents), allowing them to inspect active workloads and troubleshoot local infrastructure.

---

## Features

- **Diagnostic Tools**: Exposes three custom diagnostic tools:
  1. `list_docker_containers`: Lists active and stopped docker containers, their status, image details, and mapped ports.
  2. `tail_container_logs`: Retrieves log streams for Triton Inference Server, Sentinel gateways, or any other running container.
  3. `verify_k8s_pods`: Lists pods and state statuses in a specific Kubernetes namespace.
- **Dual Runtime Integration**:
  - **Live Mode**: If a local Docker daemon or a Kubernetes cluster is reachable, the server communicates with them natively using the official Docker Go SDK and Kubernetes `client-go`.
  - **Mock Mode**: If these services are unreachable, the server transparently falls back to high-fidelity mocks, returning simulated structures (Triton and Sentinel instances, log traces, and mock pod configurations).
- **Stdio Transport**: Communicates using the standard MCP JSON-RPC protocol over Stdio.

---

## File Structure

- [`diagnostics.go`](diagnostics.go): Coordinates live queries via client SDKs or falls back to simulated clusters/logs.
- [`main.go`](main.go): Registers tools and handlers, listens on stdio, and starts the JSON-RPC interface.
- [`main_test.go`](main_test.go): Automated unit tests testing diagnostic routines and validating tool call handlers.

---

## Tool Definitions

### 1. `list_docker_containers`
- **Description**: List active and stopped Docker containers in the local runtime.
- **Input Parameters**: None.

### 2. `tail_container_logs`
- **Description**: Tail log streams for a specific container (e.g. Triton, Sentinel).
- **Input Parameters**:
  - `name` (string, **required**): Name or ID of the target container.
  - `lines` (number, optional): Number of lines to tail (default: 50).

### 3. `verify_k8s_pods`
- **Description**: List Kubernetes pods and status in a specific namespace.
- **Input Parameters**:
  - `namespace` (string, optional): Namespace to query (default: `default`).

---

## Quick Start

### Build

```bash
# Build the binary
go build -o mcp-server-bin
```

### Running Tests

```bash
go test -v ./...
```

### Running Local Mock Mode

You can run the server in forced mock mode for local testing:

```bash
./mcp-server-bin -mock
```

---

## LLM Integration (e.g. Claude Desktop)

To hook this server into Claude Desktop, add it to your configuration file (typically located at `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "infra-diagnostics": {
      "command": "/Users/chung/sandbox/inference/velo-infra-playground/mcp-server/mcp-server-bin",
      "args": []
    }
  }
}
```
*(Optionally, add `"-mock"` under `"args"` if you want to force mock listings)*
