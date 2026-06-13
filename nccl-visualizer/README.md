# NCCL Collective Communication Simulator (`nccl-visualizer`)

This is a logical collective communication simulator built in Go. It models multi-GPU communication topologies (Ring and Tree) using concurrent **goroutines** (simulating GPU nodes) and **Go channels** (simulating physical interconnect lines like NVLink or PCIe).

---

## Features

- **Goroutine Simulation**: Each GPU runs inside its own isolated goroutine and synchronizes with others step-by-step.
- **Channel Interconnects**: Message payloads migrate across virtual GPU nodes over buffered Go channels.
- **Topology Support**:
  - **Ring Topology**: Replicates Ring AllReduce (Reduce-Scatter followed by All-Gather).
  - **Tree Topology**: Replicates a binary tree reduction (upward to root) followed by broadcast (downward to leaves).
- **Interactive Terminal Dashboard**: Renders a colorized console UI with real-time buffer states, topology flow diagram, step-by-step action log, and progress metrics.
- **Auto-Play Mode**: Allows auto-running steps with a 750ms delay.

---

## File Structure

- [`types.go`](types.go): Defines data structures representing GPU states, messages, logs, and topologies.
- [`simulator.go`](simulator.go): Core engine spawning GPU goroutines and coordinating step-by-step ring/tree communication via buffered Go channels.
- [`renderer.go`](renderer.go): Renders the interactive terminal dashboard, including the dynamic ASCII topology diagrams and memory tables.
- [`main.go`](main.go): Application entrypoint handling CLI flags, keyboard controls, and orchestrating the rendering/simulator loops.
- [`simulator_test.go`](simulator_test.go): Automated unit tests verifying the correctness of the Ring and Tree AllReduce sum computations.

---

## Quick Start

### Build and Run

To run the simulator natively on macOS:

```bash
# Run the simulator with the default Ring topology
go run ./nccl-visualizer

# Run using the Tree topology
go run ./nccl-visualizer -topo tree

# Configure a custom number of nodes (default is 4)
go run ./nccl-visualizer -nodes 4
```

### Running Tests

To run the automated correctness tests:

```bash
go test -v ./...
```

### Controls

- **`[Space/Enter]`**: Advance by one step.
- **`[a]`**: Toggle Auto-Play mode (advances automatically every 750ms).
- **`[r]`**: Restart simulation (available once completed).
- **`[q]`**: Quit simulator.

---

## How it Works

### 1. Ring AllReduce
1. **Reduce-Scatter Phase** ($N-1$ steps):
   - At step $s$, GPU $i$ sends block $(i-s) \bmod N$ to GPU $i+1$.
   - GPU $i+1$ receives it and adds it to its own block $(i-s) \bmod N$.
   - At the end of this phase, each GPU $i$ holds the fully reduced sum of block $(i+1) \bmod N$.
2. **All-Gather Phase** ($N-1$ steps):
   - At step $s$, GPU $i$ sends the fully reduced block $(i+1-s) \bmod N$ to GPU $i+1$.
   - GPU $i+1$ receives it and overwrites its local block with the fully reduced value.
   - At the end of this phase, all GPUs contain the fully reduced sum across all blocks.

### 2. Tree AllReduce
1. **Upward Reduce Phase** ($D_{max}$ steps, where $D_{max}$ is tree height):
   - Leaf nodes send their buffers up to parent nodes.
   - Parent nodes receive buffers, accumulate values block-by-block, and forward their accumulated buffers up the tree to the Root (GPU 0).
2. **Downward Broadcast Phase** ($D_{max}$ steps):
   - Root broadcasts its fully reduced buffer down to Level 1 nodes.
   - Level 1 nodes overwrite their local buffers and forward them down to Level 2 nodes (leaves).
