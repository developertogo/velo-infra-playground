package main

import (
	"sync"
	"time"
)

// SlurmError represents a standard Slurm REST API error format.
type SlurmError struct {
	ErrorNo   int    `json:"error_number"`
	ErrorMsg  string `json:"error"`
	Source    string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
}

// SlurmResponseHeader contains standard errors/warnings arrays.
type SlurmResponseHeader struct {
	Errors   []SlurmError `json:"errors"`
	Warnings []string     `json:"warnings"`
}

// SlurmPingResponse matches the standard ping response.
type SlurmPingResponse struct {
	SlurmResponseHeader
	Ping string `json:"ping"`
}

// GRES represents Generic Resource configuration (e.g. GPUs).
type GRES struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Count int    `json:"count"`
	Used  int    `json:"used"`
}

// SlurmNode represents a mock compute node.
type SlurmNode struct {
	Name       string `json:"name"`
	State      string `json:"state"` // IDLE, ALLOCATED, MIXED, DOWN
	CPUs       int    `json:"cpus"`
	CPUsUsed   int    `json:"cpus_used"`
	Memory     int64  `json:"real_memory"` // in MB
	MemoryUsed int64  `json:"memory_used"`
	GRESList   []GRES `json:"gres"`
}

// SlurmNodesResponse matches the response for /slurm/v0.0.38/nodes.
type SlurmNodesResponse struct {
	SlurmResponseHeader
	Nodes []SlurmNode `json:"nodes"`
}

// SlurmPartition represents a mock partition.
type SlurmPartition struct {
	Name          string   `json:"name"`
	State         string   `json:"state"` // UP, DOWN
	Nodes         string   `json:"nodes"` // Node list format, e.g. "node-[1-2]"
	MaxTime       int64    `json:"max_time"` // in minutes
	DefaultMemory int64    `json:"default_memory"` // in MB
	GPUsPerNode   int      `json:"gpus_per_node"`
	GPUType       string   `json:"gpu_type"` // e.g. h100, a100
}

// SlurmPartitionsResponse matches the response for /slurm/v0.0.38/partitions.
type SlurmPartitionsResponse struct {
	SlurmResponseHeader
	Partitions []SlurmPartition `json:"partitions"`
}

// TimeObject matches the Slurm JSON format for timestamps.
type TimeObject struct {
	Number int64 `json:"number"`
	Set    bool  `json:"set"`
}

// Value64Object matches Slurm's numeric object format.
type Value64Object struct {
	Number int64 `json:"number"`
	Set    bool  `json:"set"`
}

// SlurmJob represents a mock Slurm job.
type SlurmJob struct {
	JobID        int64         `json:"job_id"`
	Name         string        `json:"name"`
	JobState     string        `json:"job_state"` // PENDING, RUNNING, COMPLETED, FAILED, CANCELLED
	Partition    string        `json:"partition"`
	NodeList     string        `json:"nodes,omitempty"`
	CPUs         int           `json:"cpus"`
	GPUs         int           `json:"gpus"`
	Duration     int64         `json:"duration"` // in seconds (simulated run time)
	StartTime    TimeObject    `json:"start_time"`
	EndTime      TimeObject    `json:"end_time"`
	SubmitTime   TimeObject    `json:"submit_time"`
	Memory       Value64Object `json:"memory_per_node"`
	GresAllocStr string        `json:"tres_alloc_str,omitempty"`
}

// SlurmJobsResponse matches the response for /slurm/v0.0.38/jobs.
type SlurmJobsResponse struct {
	SlurmResponseHeader
	Jobs []SlurmJob `json:"jobs"`
}

// SlurmJobResponse matches the response for /slurm/v0.0.38/job/{job_id}.
type SlurmJobResponse struct {
	SlurmResponseHeader
	Jobs []SlurmJob `json:"jobs"`
}

// SlurmSubmitJobReq matches the JSON body of a job submission.
type SlurmSubmitJobReq struct {
	Script string `json:"script"`
	Job    struct {
		Name          string `json:"name"`
		Partition     string `json:"partition"`
		Nodes         string `json:"nodes,omitempty"`
		Tasks         int    `json:"tasks,omitempty"`
		CPUsPerTask   int    `json:"cpus_per_task,omitempty"`
		TresPerNode   string `json:"tres_per_node,omitempty"` // e.g. "gres/gpu:h100:2" or "gpu:2"
		TimeLimit     int64  `json:"time_limit,omitempty"`    // duration in seconds for mock purposes
		MemoryPerNode struct {
			Number int64 `json:"number"`
			Set    bool  `json:"set"`
		} `json:"memory_per_node,omitempty"`
	} `json:"job"`
}

// SlurmSubmitJobResponse matches the response for a job submission.
type SlurmSubmitJobResponse struct {
	SlurmResponseHeader
	JobID             int64  `json:"job_id,omitempty"`
	StepID            string `json:"step_id,omitempty"`
	JobSubmitUserMsg  string `json:"job_submit_user_msg,omitempty"`
}

// ClusterStore is the thread-safe store for our cluster resources.
type ClusterStore struct {
	mu         sync.Mutex
	Nodes      map[string]*SlurmNode
	Partitions map[string]*SlurmPartition
	Jobs       map[int64]*SlurmJob
	nextJobID  int64
}

// NewClusterStore initializes a default cluster store.
func NewClusterStore() *ClusterStore {
	store := &ClusterStore{
		Nodes:      make(map[string]*SlurmNode),
		Partitions: make(map[string]*SlurmPartition),
		Jobs:       make(map[int64]*SlurmJob),
		nextJobID:  10001,
	}

	// Initialize default partition 'h100-partition' (2 nodes, 8 GPUs each)
	store.Partitions["h100-partition"] = &SlurmPartition{
		Name:          "h100-partition",
		State:         "UP",
		Nodes:         "node-1,node-2",
		MaxTime:       120,
		DefaultMemory: 128000,
		GPUsPerNode:   8,
		GPUType:       "h100",
	}
	store.Nodes["node-1"] = &SlurmNode{
		Name:     "node-1",
		State:    "IDLE",
		CPUs:     64,
		Memory:   256000,
		GRESList: []GRES{{Name: "gpu", Type: "h100", Count: 8, Used: 0}},
	}
	store.Nodes["node-2"] = &SlurmNode{
		Name:     "node-2",
		State:    "IDLE",
		CPUs:     64,
		Memory:   256000,
		GRESList: []GRES{{Name: "gpu", Type: "h100", Count: 8, Used: 0}},
	}

	// Initialize default partition 'a100-partition' (2 nodes, 4 GPUs each)
	store.Partitions["a100-partition"] = &SlurmPartition{
		Name:          "a100-partition",
		State:         "UP",
		Nodes:         "node-3,node-4",
		MaxTime:       240,
		DefaultMemory: 64000,
		GPUsPerNode:   4,
		GPUType:       "a100",
	}
	store.Nodes["node-3"] = &SlurmNode{
		Name:     "node-3",
		State:    "IDLE",
		CPUs:     32,
		Memory:   128000,
		GRESList: []GRES{{Name: "gpu", Type: "a100", Count: 4, Used: 0}},
	}
	store.Nodes["node-4"] = &SlurmNode{
		Name:     "node-4",
		State:    "IDLE",
		CPUs:     32,
		Memory:   128000,
		GRESList: []GRES{{Name: "gpu", Type: "a100", Count: 4, Used: 0}},
	}

	return store
}

// GetNodes returns all nodes in the cluster.
func (s *ClusterStore) GetNodes() []SlurmNode {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodes := make([]SlurmNode, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		nodes = append(nodes, *n)
	}
	return nodes
}

// GetPartitions returns all partitions in the cluster.
func (s *ClusterStore) GetPartitions() []SlurmPartition {
	s.mu.Lock()
	defer s.mu.Unlock()

	partitions := make([]SlurmPartition, 0, len(s.Partitions))
	for _, p := range s.Partitions {
		partitions = append(partitions, *p)
	}
	return partitions
}

// GetJobs returns all jobs.
func (s *ClusterStore) GetJobs() []SlurmJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs := make([]SlurmJob, 0, len(s.Jobs))
	for _, j := range s.Jobs {
		jobs = append(jobs, *j)
	}
	return jobs
}

// GetJob returns a specific job by ID.
func (s *ClusterStore) GetJob(id int64) (SlurmJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.Jobs[id]
	if !ok {
		return SlurmJob{}, false
	}
	return *j, true
}

// SubmitJob creates a new job.
func (s *ClusterStore) SubmitJob(name, partition string, cpus, gpus int, memory int64, duration int64) SlurmJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	job := &SlurmJob{
		JobID:     s.nextJobID,
		Name:      name,
		JobState:  "PENDING",
		Partition: partition,
		CPUs:      cpus,
		GPUs:      gpus,
		Duration:  duration,
		SubmitTime: TimeObject{
			Number: time.Now().Unix(),
			Set:    true,
		},
		Memory: Value64Object{
			Number: memory,
			Set:    memory > 0,
		},
	}

	s.Jobs[s.nextJobID] = job
	s.nextJobID++
	return *job
}

// CancelJob transitions a job to CANCELLED and frees resources if it was running.
func (s *ClusterStore) CancelJob(id int64) (SlurmJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.Jobs[id]
	if !ok {
		return SlurmJob{}, false
	}

	if job.JobState == "RUNNING" {
		// Release node allocations
		s.releaseResources(job)
	}

	job.JobState = "CANCELLED"
	job.EndTime = TimeObject{
		Number: time.Now().Unix(),
		Set:    true,
	}

	return *job, true
}

// releaseResources returns CPU/GPU allocations back to nodes.
func (s *ClusterStore) releaseResources(job *SlurmJob) {
	// Simple node matching from job.NodeList
	// In our simple case, NodeList will be the node name (e.g. "node-1")
	if node, ok := s.Nodes[job.NodeList]; ok {
		node.CPUsUsed -= job.CPUs
		if node.CPUsUsed < 0 {
			node.CPUsUsed = 0
		}
		
		node.MemoryUsed -= job.Memory.Number
		if node.MemoryUsed < 0 {
			node.MemoryUsed = 0
		}

		for idx, g := range node.GRESList {
			if g.Name == "gpu" {
				node.GRESList[idx].Used -= job.GPUs
				if node.GRESList[idx].Used < 0 {
					node.GRESList[idx].Used = 0
				}
			}
		}

		// Update state
		s.recalculateNodeState(node)
	}
}

// recalculateNodeState evaluates node occupancy to set IDLE, MIXED, ALLOCATED, or DOWN.
func (s *ClusterStore) recalculateNodeState(node *SlurmNode) {
	gpuTotal := 0
	gpuUsed := 0
	for _, g := range node.GRESList {
		if g.Name == "gpu" {
			gpuTotal += g.Count
			gpuUsed += g.Used
		}
	}

	if gpuUsed == 0 && node.CPUsUsed == 0 {
		node.State = "IDLE"
	} else if gpuUsed == gpuTotal && node.CPUsUsed == node.CPUs {
		node.State = "ALLOCATED"
	} else {
		node.State = "MIXED"
	}
}
