package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// SlurmHandler handles HTTP routing and actions on the ClusterStore.
type SlurmHandler struct {
	store *ClusterStore
}

// NewSlurmHandler returns a handler configured with the store.
func NewSlurmHandler(store *ClusterStore) *SlurmHandler {
	return &SlurmHandler{store: store}
}

// authMiddleware checks for JWT token presence.
func (h *SlurmHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-SLURM-USER-TOKEN")
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(SlurmResponseHeader{
				Errors: []SlurmError{
					{
						ErrorNo:  401,
						ErrorMsg: "Unauthorized: Missing authentication token in 'X-SLURM-USER-TOKEN' header",
					},
				},
				Warnings: []string{},
			})
			return
		}
		next(w, r)
	}
}

// RegisterRoutes registers all REST paths to the multiplexer.
func (h *SlurmHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /slurm/v0.0.38/ping", h.authMiddleware(h.Ping))
	mux.HandleFunc("GET /slurm/v0.0.38/nodes", h.authMiddleware(h.GetNodes))
	mux.HandleFunc("GET /slurm/v0.0.38/partitions", h.authMiddleware(h.GetPartitions))
	mux.HandleFunc("GET /slurm/v0.0.38/jobs", h.authMiddleware(h.GetJobs))
	mux.HandleFunc("GET /slurm/v0.0.38/job/{job_id}", h.authMiddleware(h.GetJob))
	mux.HandleFunc("POST /slurm/v0.0.38/job/submit", h.authMiddleware(h.SubmitJob))
	mux.HandleFunc("DELETE /slurm/v0.0.38/job/{job_id}", h.authMiddleware(h.CancelJob))
}

// Ping implements GET /slurm/v0.0.38/ping
func (h *SlurmHandler) Ping(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SlurmPingResponse{
		SlurmResponseHeader: SlurmResponseHeader{Errors: []SlurmError{}, Warnings: []string{}},
		Ping:                "pong",
	})
}

// GetNodes implements GET /slurm/v0.0.38/nodes
func (h *SlurmHandler) GetNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	nodes := h.store.GetNodes()
	json.NewEncoder(w).Encode(SlurmNodesResponse{
		SlurmResponseHeader: SlurmResponseHeader{Errors: []SlurmError{}, Warnings: []string{}},
		Nodes:               nodes,
	})
}

// GetPartitions implements GET /slurm/v0.0.38/partitions
func (h *SlurmHandler) GetPartitions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	partitions := h.store.GetPartitions()
	json.NewEncoder(w).Encode(SlurmPartitionsResponse{
		SlurmResponseHeader: SlurmResponseHeader{Errors: []SlurmError{}, Warnings: []string{}},
		Partitions:          partitions,
	})
}

// GetJobs implements GET /slurm/v0.0.38/jobs
func (h *SlurmHandler) GetJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobs := h.store.GetJobs()
	json.NewEncoder(w).Encode(SlurmJobsResponse{
		SlurmResponseHeader: SlurmResponseHeader{Errors: []SlurmError{}, Warnings: []string{}},
		Jobs:                jobs,
	})
}

// GetJob implements GET /slurm/v0.0.38/job/{job_id}
func (h *SlurmHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobIDStr := r.PathValue("job_id")
	
	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(SlurmResponseHeader{
			Errors: []SlurmError{{ErrorNo: 400, ErrorMsg: "Invalid job ID format"}},
		})
		return
	}

	job, found := h.store.GetJob(jobID)
	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(SlurmResponseHeader{
			Errors: []SlurmError{{ErrorNo: 404, ErrorMsg: fmt.Sprintf("Job %d not found", jobID)}},
		})
		return
	}

	json.NewEncoder(w).Encode(SlurmJobResponse{
		SlurmResponseHeader: SlurmResponseHeader{Errors: []SlurmError{}, Warnings: []string{}},
		Jobs:                []SlurmJob{job},
	})
}

// SubmitJob implements POST /slurm/v0.0.38/job/submit
func (h *SlurmHandler) SubmitJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var req SlurmSubmitJobReq
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(SlurmResponseHeader{
			Errors: []SlurmError{{ErrorNo: 400, ErrorMsg: "Invalid JSON request body"}},
		})
		return
	}

	// Validate inputs
	if req.Job.Partition == "" {
		req.Job.Partition = "h100-partition" // Default partition
	}
	if req.Job.Name == "" {
		req.Job.Name = "slurm_mock_job"
	}

	// Determine GPU allocation from tres_per_node (e.g. "gres/gpu:2" or "gpu:h100:2")
	gpus := 0
	gpuStr := req.Job.TresPerNode
	if gpuStr != "" {
		// Try to parse number of GPUs from e.g. "gpu:2", "gres/gpu:2", "gpu:h100:4"
		parts := strings.Split(gpuStr, ":")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			if val, err := strconv.Atoi(lastPart); err == nil {
				gpus = val
			}
		}
	}
	// Fallback to 1 GPU if script suggests it or if requested partition is h100/a100
	if gpus == 0 && (strings.Contains(req.Script, "nvidia-smi") || strings.Contains(req.Script, "gpu")) {
		gpus = 1
	}

	cpus := req.Job.Tasks * req.Job.CPUsPerTask
	if cpus == 0 {
		cpus = 1 // Default 1 CPU
	}

	duration := req.Job.TimeLimit
	if duration == 0 {
		duration = 15 // Default 15 seconds run-time for simulation speed
	}

	memory := req.Job.MemoryPerNode.Number
	if memory == 0 {
		memory = 4000 // Default 4GB
	}

	job := h.store.SubmitJob(req.Job.Name, req.Job.Partition, cpus, gpus, memory, duration)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SlurmSubmitJobResponse{
		SlurmResponseHeader: SlurmResponseHeader{Errors: []SlurmError{}, Warnings: []string{}},
		JobID:               job.JobID,
		StepID:              "batch",
		JobSubmitUserMsg:    fmt.Sprintf("Job %d submitted successfully to partition '%s'", job.JobID, job.Partition),
	})
}

// CancelJob implements DELETE /slurm/v0.0.38/job/{job_id}
func (h *SlurmHandler) CancelJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobIDStr := r.PathValue("job_id")
	
	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(SlurmResponseHeader{
			Errors: []SlurmError{{ErrorNo: 400, ErrorMsg: "Invalid job ID format"}},
		})
		return
	}

	_, found := h.store.CancelJob(jobID)
	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(SlurmResponseHeader{
			Errors: []SlurmError{{ErrorNo: 404, ErrorMsg: fmt.Sprintf("Job %d not found", jobID)}},
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SlurmResponseHeader{
		Errors:   []SlurmError{},
		Warnings: []string{},
	})
}
