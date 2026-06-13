# Velo Model Lifecycle Orchestration with Metaflow

This directory contains a Python-based Metaflow pipeline that automates the model serving lifecycle across three repositories:
1. **`velo-core`**: Dynamically loaded via Python FFI to check binary compatibility and run benchmarks.
2. **`velo-sentinel`**: Verified using the Python SDK client (`sentinel_sdk`) to validate P99 SLA performance metrics.
3. **`velo-infra-playground`**: Used to generate and package Kubernetes `SentinelDeployment` Custom Resource configurations.

---

## 📂 Components & Files

*   **`flow.py`**: Implements the `VeloModelLifecycleFlow` execution stages:
    *   **FFI Step**: Dynamically loads and tests the native Rust shared library `libvelo_core.dylib` from `velo-core/target/debug/` using Python `ctypes`.
    *   **Packaging Step**: Evaluates quantization options against the input SLA and formats a Kubernetes Custom Resource JSON matching the `sentineldeployment_types.go` schema.
    *   **Verification Step**: Imports the `sentinel_sdk` client from `velo-sentinel/sdks/python` to query a local gateway, with full simulated fallback logic if offline.
*   **`requirements.txt`**: Python dependencies required for execution (`metaflow`, `grpcio`, `protobuf`).

---

## 🏛️ Pipeline Architecture

The pipeline executes the following stages:
1. **`start`**: Ingests the model metadata and SLA latency target parameters.
2. **`optimize`**: Dynamically loads the `velo-core` shared library via `ctypes`. It queries the FFI to initialize the engine, benchmarks different quantization options (`FP32`, `FP16`, `INT8`, `Q4_0`), and selects the best quantization profile that satisfies the SLA latency target.
3. **`package`**: Generates a Custom Resource manifest conforming to the `SentinelDeployment` spec.
4. **`deploy`**: Simulates applying the custom resource configuration and registry mappings.
5. **`verify`**: Invokes the `sentinel_sdk` Python client to perform sanity requests against the Gateway and asserts the P99 SLA.
6. **`end`**: Summarizes execution stats and logs metadata run records.

---

## 🚀 Getting Started

### 1. Install Dependencies
Install Python dependencies required for Metaflow, gRPC, and protobuf:
```bash
pip install -r requirements.txt
```

### 2. Run the Metaflow Pipeline
Run the flow with custom model and SLA target parameters:

```bash
# Run with default values
python flow.py run

# Run with a strict SLA latency target (triggers INT8/Q4_0 selection)
python flow.py run --model-name "Qwen2.5-1.5B" --sla-ms 100

# Run with a relaxed SLA latency target (allows FP16/FP32 selection)
python flow.py run --model-name "Llama-3-8B" --sla-ms 200
```

---

## 💡 Dynamic Fallbacks
To enable fully offline development and Apple Silicon local testing, the pipeline implements the following fallbacks matching the `velo-infra-playground` design:
*   **FFI Fallback**: If the `libvelo_core.dylib` shared library has not been compiled or Metal is unsupported on the host, the pipeline falls back to a simulated optimization benchmark.
*   **Gateway/SDK Fallback**: If the local Sentinel Gateway (`localhost:8080`) is not actively listening, the verification step falls back to a simulated performance test and validates SLA criteria against the profile metrics.
