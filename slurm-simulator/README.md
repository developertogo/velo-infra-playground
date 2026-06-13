# Slurm REST API Simulator (`slurm-simulator`)

This is a mock implementation of the official Slurm REST API daemon (`slurmrestd`) version `v0.0.38`. It represents a High-Performance Computing (HPC) batch scheduling environment, tracking GPU/CPU resources across partitions and scheduling jobs dynamically.

---

## Features

- **Standard REST Endpoints**: Implements standard Slurm OpenAPI spec routes for pinging, node querying, partition status, job listings, and job submissions/cancellations.
- **Token-based Authentication**: Inspects requests for the official `X-SLURM-USER-TOKEN` header, returning standard `401 Unauthorized` responses if missing.
- **Dynamic Scheduler**: An in-memory scheduling daemon runs in the background. It transitions submitted jobs from `PENDING` to `RUNNING` if resources (CPUs, Memory, GPUs) are available on nodes in the requested partition.
- **Resource Reclaimer**: Automatically transitions jobs to `COMPLETED` when their simulated duration finishes, returning node resources back to the pool.
- **Standard response formatting**: Conformant with official Slurm v0.0.38 OpenAPI response JSON structures.

---

## File Structure

- [`store.go`](store.go): Implements the thread-safe ClusterStore which tracks mock compute nodes (`node-1` to `node-4`), partitions (`h100-partition`, `a100-partition`), and jobs.
- [`scheduler.go`](scheduler.go): Background goroutine scheduling pending jobs based on first-fit resource availability and reclaiming resources on job completion.
- [`handlers.go`](handlers.go): HTTP handler endpoints for REST routing, body decoding, input validation, and authorization check.
- [`main.go`](main.go): Application entrypoint initiating the cluster state, starting the scheduler daemon, and listening for HTTP connections.
- [`main_test.go`](main_test.go): Automated unit tests testing authorization, endpoints, and job scheduler resource allocations.

---

## REST API Specification

All endpoints require authentication via the `X-SLURM-USER-TOKEN` header.

| Method | Route | Description |
| :--- | :--- | :--- |
| **GET** | `/slurm/v0.0.38/ping` | Ping the mock slurmrestd daemon |
| **GET** | `/slurm/v0.0.38/nodes` | List all compute nodes in the cluster |
| **GET** | `/slurm/v0.0.38/partitions` | List all partitions (h100-partition, a100-partition) |
| **GET** | `/slurm/v0.0.38/jobs` | List all jobs |
| **GET** | `/slurm/v0.0.38/job/{job_id}` | Query detailed status of a specific job |
| **POST** | `/slurm/v0.0.38/job/submit` | Submit a new batch job to the queue |
| **DELETE** | `/slurm/v0.0.38/job/{job_id}` | Cancel/terminate a pending or running job |

---

## Quick Start

### Build and Run

To start the simulator locally:

```bash
# Start the simulator daemon on the default port 6820
go run ./slurm-simulator

# Configure a custom listening port
go run ./slurm-simulator -port 18080
```

### Running Tests

To run the automated tests:

```bash
go test -v ./...
```

---

## Usage Examples

Below are standard examples of querying the simulator using `curl`.

### 1. Ping the Daemon
```bash
curl -i http://localhost:6820/slurm/v0.0.38/ping \
  -H "X-SLURM-USER-TOKEN: secret_token"
```

### 2. View Compute Nodes
```bash
curl http://localhost:6820/slurm/v0.0.38/nodes \
  -H "X-SLURM-USER-TOKEN: secret_token" | jq .
```

### 3. Submit a Job
Submit a request requesting 4 H100 GPUs and 8 tasks on the `h100-partition` to run for 10 seconds:

```bash
curl -X POST http://localhost:6820/slurm/v0.0.38/job/submit \
  -H "X-SLURM-USER-TOKEN: secret_token" \
  -H "Content-Type: application/json" \
  -d '{
    "script": "#!/bin/bash\npython train.py",
    "job": {
      "name": "h100_training_run",
      "partition": "h100-partition",
      "tasks": 8,
      "cpus_per_task": 1,
      "tres_per_node": "gpu:h100:4",
      "time_limit": 10
    }
  }'
```

### 4. Query Job Status
Using the job ID returned by the submission endpoint (e.g. `10001`):

```bash
curl http://localhost:6820/slurm/v0.0.38/job/10001 \
  -H "X-SLURM-USER-TOKEN: secret_token" | jq .
```

### 5. Cancel a Job
```bash
curl -X DELETE http://localhost:6820/slurm/v0.0.38/job/10001 \
  -H "X-SLURM-USER-TOKEN: secret_token"
```
