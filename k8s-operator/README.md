# Sentinel Kubernetes Operator (`k8s-operator`)

This is a custom Kubernetes controller built in Go using the official `sigs.k8s.io/controller-runtime` and `client-go` libraries. It automates scaling of the `velo-sentinel` gateway worker nodes by monitoring the request queue depth of the Sentinel gateway and scaling associated Deployments dynamically.

---

## Features

- **Custom Resource Definition (CRD)**: Defines the custom schema type `SentinelDeployment` (`velo.infra/v1alpha1`).
- **Dynamic Scale Reconciler**: Periodically fetches the gRPC queue depth metrics of the Sentinel gateway, dynamically calculates the target replica count based on configured thresholds, and adjusts target worker Deployment replicas.
- **Scaling Bounds (Min/Max)**: Restricts replica allocations to respect user-defined `minReplicas` and `maxReplicas` bounds.
- **High-Fidelity Mock Mode**: If the gateway is unreachable or configured with the `"mock"` address, the controller simulates live queue depth fluctuations, enabling testing and development on offline clusters or local environments (like Apple Silicon macOS).

---

## File Structure

- [`api/v1alpha1/`](api/v1alpha1/): Contains the custom API resource types, schema registration, and DeepCopy helpers.
- [`internal/controller/`](internal/controller/): Implements the reconciler loop matching active queue depths to replicas.
- [`config/crd/`](config/crd/): Declares the CustomResourceDefinition (CRD) YAML manifest.
- [`main.go`](main.go): Manager entrypoint initializing schemes, starting metrics/probes, and running the reconciler event loop.
- [`internal/controller/sentineldeployment_controller_test.go`](internal/controller/sentineldeployment_controller_test.go): Automated unit tests using a controller-runtime fake client to assert scaling logic, bounds enforcement, and status reports.

---

## CRD Specification (`SentinelDeployment`)

### Spec Fields

- `gatewayAddress` (string, **required**): Address of the Sentinel gateway server (e.g. `velo-sentinel:9000` or `"mock"`).
- `targetDeployment` (string, **required**): Name of the target worker Deployment to scale.
- `minReplicas` (int32, optional): Min scale count (default: 1).
- `maxReplicas` (int32, optional): Max scale count (default: 10).
- `scaleThreshold` (int32, optional): Number of requests per worker pod representing scale-up boundaries (default: 5).

### Status Fields

- `replicas` (int32): Last observed replica count of the worker deployment.
- `activeQueueDepth` (int32): Last queried queue depth value from the gateway.
- `lastScaleTime` (string): Timestamp of the last scaling event.

---

## Quick Start

### 1. Build

```bash
go build -o k8s-operator-bin
```

### 2. Run Tests

```bash
go test -v ./...
```

### 3. Deploy to Cluster (e.g. `kind` / `colima`)

If you have a local Kubernetes cluster active:

```bash
# 1. Apply the Custom Resource Definition
kubectl apply -f config/crd/velo.infra_sentineldeployments.yaml

# 2. Run the Operator locally (uses your kubeconfig context)
go run ./main.go
```

---

## Example Custom Resource

Save the following as `sentinel-deploy.yaml` and apply it (`kubectl apply -f sentinel-deploy.yaml`):

```yaml
apiVersion: velo.infra/v1alpha1
kind: SentinelDeployment
metadata:
  name: sentinel-autoscaler
  namespace: default
spec:
  gatewayAddress: mock
  minReplicas: 2
  maxReplicas: 8
  scaleThreshold: 5
  targetDeployment: sentinel-worker
```
