# Sentinel Infrastructure Lab (`velo-infra-playground`)

This repository is a Go-centric systems engineering playground designed to run, test, and validate infrastructure control-plane components natively on macOS (Apple Silicon). It simulates high-performance computing (HPC) scheduling, bare-metal hardware actions, collective GPU communications, custom Kubernetes orchestration, and AI-agent diagnostic interfaces.

All projects compile natively on Apple Silicon, are fully unit-tested with comprehensive edge cases, and are individually documented.

---

## Project Directory

| Project | Location | Key Capabilities | Core Tech | Testing Success |
| :--- | :--- | :--- | :--- | :--- |
| **1. Kubernetes Operator** | [`k8s-operator/`](k8s-operator) | Custom CRD (`SentinelDeployment`), queue-based scaling reconciler, Min/Max limits, CRD manifests | `sigs.k8s.io/controller-runtime`, `client-go` | **PASS** (fake-client integration) |
| **2. MCP Server** | [`mcp-server/`](mcp-server) | Exposed tools (list containers, tail logs, verify pods) with live SDK checks and fallback mocks | `mcp-go`, Docker SDK, `client-go` | **PASS** (handlers, args, mock logs) |
| **3. Slurm Simulator** | [`slurm-simulator/`](slurm-simulator) | Mock `slurmrestd` v0.0.38 API, JWT validation, store, background job state reconciler | standard `net/http` router | **PASS** (reclaiming, constraints, invalid) |
| **4. Redfish Simulator** | [`redfish-simulator/`](redfish-simulator) | Root schema, chassis reset, boot patching, reactive physics (Off: 0 RPM fans, 22°C temps, 15W draw) | standard `net/http` router | **PASS** (patching, resets, physics metrics) |
| **5. NCCL Visualizer** | [`nccl-visualizer/`](nccl-visualizer) | Interactive Ring/Tree topology collectives, GPU goroutines, NVLink channel queues, terminal UI | goroutines, channels, ANSI renderer | **PASS** (Ring & Tree AllReduce sums) |

All modules are integrated via [`go.work`](go.work) at the workspace root, configured for `Go 1.25.0` to support modern toolchains.

---

## Workspace Quick Start

### 1. Compile Everything

You can verify compilation for all workspace modules simultaneously:

```bash
# Verify workspace-level dependencies are aligned
go work sync

# Compile the projects
go build -v ./k8s-operator
go build -v ./mcp-server
go build -v ./slurm-simulator
go build -v ./redfish-simulator
go build -v ./nccl-visualizer
```

### 2. Run All Tests

You can execute the entire suite of unit and integration tests across all modules from the root directory with a single command:

```bash
go test -v ./...
```

### 3. Deploying External Services (Sentinel & Core)

The `sentinel/` and `core/` folders inside `velo-infra-playground` contain only deployment assets (Dockerfiles and Kubernetes manifests).

To build the Docker images, you have two options:

#### Option A: Build Directly from Remote GitHub Repositories (Recommended)
You can build the containers without cloning them locally by passing the remote Git URLs directly as the Docker build contexts:

```bash
# 1. Build Sentinel (Java 25) from the remote Git repository
docker build -f sentinel/Dockerfile.deploy -t velo-sentinel:latest https://github.com/developertogo/velo-sentinel.git

# 2. Build Core (Rust) from the remote Git repository
docker build -f core/Dockerfile.deploy -t velo-core:latest https://github.com/developertogo/velo-core.git
```

#### Option B: Build from Local Clones
If you want to edit the Java or Rust source code locally, clone the repositories as siblings to `velo-infra-playground`:

```bash
# Clone sibling repositories
git clone https://github.com/developertogo/velo-sentinel
git clone https://github.com/developertogo/velo-core
```

Then build from the root of `velo-infra-playground` pointing to the local sibling contexts:

```bash
# 1. Build Sentinel (Java 25) using local context
docker build -f sentinel/Dockerfile.deploy -t velo-sentinel:latest ../velo-sentinel

# 2. Build Core (Rust) using local context
docker build -f core/Dockerfile.deploy -t velo-core:latest ../velo-core
```

---

## Sub-Project Documentation

For specific details on config parameters, API routes, usage instructions, and sample client requests, refer to the individual project documentation:

- 📖 [Kubernetes Operator README](k8s-operator/README.md)
- 📖 [MCP Server README](mcp-server/README.md)
- 📖 [Slurm Simulator README](slurm-simulator/README.md)
- 📖 [Redfish Simulator README](redfish-simulator/README.md)
- 📖 [NCCL Visualizer README](nccl-visualizer/README.md)
