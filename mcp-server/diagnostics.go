package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// DiagnosticsEngine abstracts cluster/container queries.
type DiagnosticsEngine struct {
	forceMock  bool
	dockerCli  *client.Client
	kubeClient *kubernetes.Clientset
}

// NewDiagnosticsEngine initializes clients, falling back to mocks if unavailable.
func NewDiagnosticsEngine(forceMock bool) *DiagnosticsEngine {
	engine := &DiagnosticsEngine{forceMock: forceMock}
	if forceMock {
		fmt.Fprintf(os.Stderr, "[MCP Engine] Running in forced MOCK mode.\n")
		return engine
	}

	// Initialize Docker client
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err == nil {
		// Ping to verify connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, pingErr := dockerCli.Ping(ctx); pingErr == nil {
			engine.dockerCli = dockerCli
			fmt.Fprintf(os.Stderr, "[MCP Engine] Docker SDK connected to local daemon.\n")
		} else {
			fmt.Fprintf(os.Stderr, "[MCP Engine] Docker daemon unreachable, falling back to mocks for container tools. Error: %v\n", pingErr)
		}
	} else {
		fmt.Fprintf(os.Stderr, "[MCP Engine] Docker client initialization failed, using mocks. Error: %v\n", err)
	}

	// Initialize Kubernetes client
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err == nil {
		clientset, err := kubernetes.NewForConfig(config)
		if err == nil {
			// Test connectivity by querying default namespace metadata
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, connErr := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1}); connErr == nil {
				engine.kubeClient = clientset
				fmt.Fprintf(os.Stderr, "[MCP Engine] Kubernetes client-go connected to cluster.\n")
			} else {
				fmt.Fprintf(os.Stderr, "[MCP Engine] Kubernetes cluster unreachable, falling back to mocks for pod tools. Error: %v\n", connErr)
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "[MCP Engine] Kubeconfig not found or invalid, using mocks. Error: %v\n", err)
	}

	return engine
}

// ListContainers returns Docker container status list.
func (e *DiagnosticsEngine) ListContainers(ctx context.Context) (string, error) {
	if e.forceMock || e.dockerCli == nil {
		return e.mockListContainers(), nil
	}

	containers, err := e.dockerCli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("failed to list Docker containers: %w", err)
	}

	if len(containers) == 0 {
		return "No Docker containers found in the local runtime.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-20s | %-30s | %-12s | %-15s | %-15s\n", "CONTAINER ID", "NAMES", "IMAGE", "STATUS", "PORTS"))
	sb.WriteString(strings.Repeat("─", 100) + "\n")

	for _, c := range containers {
		names := strings.Join(c.Names, ", ")
		ports := ""
		for _, p := range c.Ports {
			ports += fmt.Sprintf("%d->%d/%s ", p.PublicPort, p.PrivatePort, p.Type)
		}
		sb.WriteString(fmt.Sprintf("%-20s | %-30s | %-12s | %-15s | %-15s\n", 
			c.ID[:12], names, truncate(c.Image, 12), c.Status, strings.TrimSpace(ports)))
	}
	return sb.String(), nil
}

// TailLogs returns tail logs for a specific container.
func (e *DiagnosticsEngine) TailLogs(ctx context.Context, name string, lines int) (string, error) {
	if e.forceMock || e.dockerCli == nil {
		return e.mockTailLogs(name, lines), nil
	}

	// Try to find the container ID from name
	containers, err := e.dockerCli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("failed to search containers: %w", err)
	}

	targetID := ""
	for _, c := range containers {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name || c.ID[:12] == name || c.ID == name {
				targetID = c.ID
				break
			}
		}
		if targetID != "" {
			break
		}
	}

	if targetID == "" {
		return "", fmt.Errorf("container '%s' not found", name)
	}

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", lines),
	}

	reader, err := e.dockerCli.ContainerLogs(ctx, targetID, options)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve container logs: %w", err)
	}
	defer reader.Close()

	// Parse logs (handling Docker multiplexed stream header)
	buf := new(strings.Builder)
	hdr := make([]byte, 8)
	for {
		_, err := reader.Read(hdr)
		if err != nil {
			break
		}
		// Docker logs header contains stream type (1=stdout, 2=stderr) at index 0
		// and payload size at index 4-7
		size := uint32(hdr[4])<<24 | uint32(hdr[5])<<16 | uint32(hdr[6])<<8 | uint32(hdr[7])
		payload := make([]byte, size)
		_, err = reader.Read(payload)
		if err != nil {
			break
		}
		buf.Write(payload)
	}

	return buf.String(), nil
}

// ListPods returns Kubernetes pod status list in a namespace.
func (e *DiagnosticsEngine) ListPods(ctx context.Context, ns string) (string, error) {
	if e.forceMock || e.kubeClient == nil {
		return e.mockListPods(ns), nil
	}

	pods, err := e.kubeClient.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to query pods in namespace '%s': %w", ns, err)
	}

	if len(pods.Items) == 0 {
		return fmt.Sprintf("No pods found in namespace '%s'.", ns), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-35s | %-12s | %-12s | %-15s | %-15s\n", "POD NAME", "NAMESPACE", "STATUS", "IP", "NODE"))
	sb.WriteString(strings.Repeat("─", 100) + "\n")

	for _, p := range pods.Items {
		sb.WriteString(fmt.Sprintf("%-35s | %-12s | %-12s | %-15s | %-15s\n", 
			p.Name, p.Namespace, p.Status.Phase, p.Status.PodIP, p.Spec.NodeName))
	}
	return sb.String(), nil
}

// Mock Implementations for Offline M3 Execution

func (e *DiagnosticsEngine) mockListContainers() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[MOCK DATA] Current Active Docker Containers:\n\n"))
	sb.WriteString(fmt.Sprintf("%-20s | %-30s | %-25s | %-15s | %-20s\n", "CONTAINER ID", "NAMES", "IMAGE", "STATUS", "PORTS"))
	sb.WriteString(strings.Repeat("─", 120) + "\n")
	sb.WriteString(fmt.Sprintf("%-20s | %-30s | %-25s | %-15s | %-20s\n", "a78f23bc9d1e", "triton-inference-server", "nvcr.io/nvidia/triton:24.04", "Up 4 hours", "8000->8000/tcp"))
	sb.WriteString(fmt.Sprintf("%-20s | %-30s | %-25s | %-15s | %-20s\n", "5e8dfd91204a", "velo-sentinel-gateway", "velo-sentinel:latest", "Up 4 hours", "9000->9000/tcp"))
	sb.WriteString(fmt.Sprintf("%-20s | %-30s | %-25s | %-15s | %-20s\n", "12a34b56c78d", "slurm-simulator", "slurm-simulator:latest", "Up 2 hours", "6820->6820/tcp"))
	sb.WriteString(fmt.Sprintf("%-20s | %-30s | %-25s | %-15s | %-20s\n", "9ef01a23b45c", "redfish-simulator", "redfish-simulator:latest", "Up 2 hours", "8000->8000/tcp"))
	return sb.String()
}

func (e *DiagnosticsEngine) mockTailLogs(name string, lines int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[MOCK DATA] Tail logs for container '%s' (last %d lines):\n\n", name, lines))

	if strings.Contains(name, "triton") {
		sb.WriteString("I0613 08:00:00.102 Triton Inference Server v2.44.0 starting...\n")
		sb.WriteString("I0613 08:00:01.340 Creating GPU backends (mocked on Apple Silicon)\n")
		sb.WriteString("I0613 08:00:02.051 loaded model 'resnet50_onnx' version 1 successfully\n")
		sb.WriteString("I0613 08:00:02.052 loaded model 'inception_graphdef' version 1 successfully\n")
		sb.WriteString("I0613 08:00:03.110 HTTP Server listening on port 8000\n")
		sb.WriteString("I0613 08:00:03.111 gRPC Server listening on port 8001\n")
		sb.WriteString("I0613 08:05:12.440 Received inference batch (size=16) for 'resnet50_onnx'\n")
		sb.WriteString("I0613 08:05:12.445 Inference latency: 4.8ms\n")
	} else if strings.Contains(name, "sentinel") {
		sb.WriteString("[Sentinel] 2026-06-13T08:00:00.124Z Sentinel Gateway starting on port 9000\n")
		sb.WriteString("[Sentinel] 2026-06-13T08:00:00.250Z Registered downstream gRPC Triton client at localhost:8001\n")
		sb.WriteString("[Sentinel] 2026-06-13T08:05:12.438Z Routing incoming request to Triton 'resnet50_onnx' model\n")
		sb.WriteString("[Sentinel] 2026-06-13T08:12:00.100Z Health check: Triton is reachable. State=OK\n")
		sb.WriteString("[Sentinel] 2026-06-13T08:30:15.549Z Queue depth metric: 0 active requests\n")
	} else if strings.Contains(name, "slurm") {
		sb.WriteString("[Slurm Simulator] Starting slurmrestd mock daemon on :6820...\n")
		sb.WriteString("[Scheduler] Scheduled Job 10001 (gpu_intensive) on node-1\n")
		sb.WriteString("[Scheduler] Job 10001 (gpu_intensive) COMPLETED on node node-1\n")
	} else {
		sb.WriteString(fmt.Sprintf("I0613 08:00:00.000 Initialized %s diagnostic logs\n", name))
		sb.WriteString(fmt.Sprintf("I0613 08:10:00.000 Container %s status checked: Health=OK\n", name))
	}
	return sb.String()
}

func (e *DiagnosticsEngine) mockListPods(ns string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[MOCK DATA] Kubernetes Pods in namespace '%s':\n\n", ns))
	sb.WriteString(fmt.Sprintf("%-35s | %-12s | %-12s | %-15s | %-15s\n", "POD NAME", "NAMESPACE", "STATUS", "IP", "NODE"))
	sb.WriteString(strings.Repeat("─", 100) + "\n")
	sb.WriteString(fmt.Sprintf("%-35s | %-12s | %-12s | %-15s | %-15s\n", "velo-sentinel-6f89b9d99-abc12", ns, "Running", "10.244.0.5", "colima-node"))
	sb.WriteString(fmt.Sprintf("%-35s | %-12s | %-12s | %-15s | %-15s\n", "triton-worker-0", ns, "Running", "10.244.0.6", "colima-node"))
	sb.WriteString(fmt.Sprintf("%-35s | %-12s | %-12s | %-15s | %-15s\n", "triton-worker-1", ns, "Pending", "<none>", "<none>"))
	return sb.String()
}

// helper
func truncate(str string, length int) string {
	if len(str) <= length {
		return str
	}
	return str[:length]
}
