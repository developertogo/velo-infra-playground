package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	store := NewClusterStore()
	handler := NewSlurmHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Request without token
	req, _ := http.NewRequest("GET", "/slurm/v0.0.38/ping", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}

	var resp SlurmResponseHeader
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if len(resp.Errors) == 0 || resp.Errors[0].ErrorNo != 401 {
		t.Errorf("Expected auth error in response, got %v", resp.Errors)
	}

	// 2. Request with token
	reqWithToken, _ := http.NewRequest("GET", "/slurm/v0.0.38/ping", nil)
	reqWithToken.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	rrWithToken := httptest.NewRecorder()
	mux.ServeHTTP(rrWithToken, reqWithToken)

	if rrWithToken.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rrWithToken.Code)
	}
}

func TestGetNodesAndPartitions(t *testing.T) {
	store := NewClusterStore()
	handler := NewSlurmHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Get Nodes
	req, _ := http.NewRequest("GET", "/slurm/v0.0.38/nodes", nil)
	req.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var nodeResp SlurmNodesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &nodeResp); err != nil {
		t.Fatalf("Failed to unmarshal nodes: %v", err)
	}

	if len(nodeResp.Nodes) != 4 { // node-1, node-2, node-3, node-4
		t.Errorf("Expected 4 nodes, got %d", len(nodeResp.Nodes))
	}

	// 2. Get Partitions
	reqPart, _ := http.NewRequest("GET", "/slurm/v0.0.38/partitions", nil)
	reqPart.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	rrPart := httptest.NewRecorder()
	mux.ServeHTTP(rrPart, reqPart)

	if rrPart.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rrPart.Code)
	}

	var partResp SlurmPartitionsResponse
	if err := json.Unmarshal(rrPart.Body.Bytes(), &partResp); err != nil {
		t.Fatalf("Failed to unmarshal partitions: %v", err)
	}

	if len(partResp.Partitions) != 2 { // h100-partition, a100-partition
		t.Errorf("Expected 2 partitions, got %d", len(partResp.Partitions))
	}
}

func TestJobLifecycle(t *testing.T) {
	store := NewClusterStore()
	handler := NewSlurmHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Submit Job
	jobReq := SlurmSubmitJobReq{}
	jobReq.Job.Name = "test_run"
	jobReq.Job.Partition = "h100-partition"
	jobReq.Job.Tasks = 1
	jobReq.Job.CPUsPerTask = 2
	jobReq.Job.TresPerNode = "gpu:2" // 2 GPUs
	jobReq.Job.TimeLimit = 10

	body, _ := json.Marshal(jobReq)
	req, _ := http.NewRequest("POST", "/slurm/v0.0.38/job/submit", bytes.NewReader(body))
	req.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Failed to submit job, status: %d, body: %s", rr.Code, rr.Body.String())
	}

	var submitResp SlurmSubmitJobResponse
	json.Unmarshal(rr.Body.Bytes(), &submitResp)

	if submitResp.JobID == 0 {
		t.Errorf("Expected non-zero job ID, got %d", submitResp.JobID)
	}

	// 2. Query Job Info
	reqQuery, _ := http.NewRequest("GET", "/slurm/v0.0.38/job/"+strconvFormat(submitResp.JobID), nil)
	reqQuery.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	rrQuery := httptest.NewRecorder()
	mux.ServeHTTP(rrQuery, reqQuery)

	if rrQuery.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rrQuery.Code)
	}

	var jobQueryResp SlurmJobResponse
	json.Unmarshal(rrQuery.Body.Bytes(), &jobQueryResp)
	if len(jobQueryResp.Jobs) != 1 || jobQueryResp.Jobs[0].JobID != submitResp.JobID {
		t.Errorf("Expected job ID %d, got %v", submitResp.JobID, jobQueryResp.Jobs)
	}
	if jobQueryResp.Jobs[0].JobState != "PENDING" {
		t.Errorf("Expected job to start in PENDING, got %s", jobQueryResp.Jobs[0].JobState)
	}

	// 3. Cancel Job
	reqCancel, _ := http.NewRequest("DELETE", "/slurm/v0.0.38/job/"+strconvFormat(submitResp.JobID), nil)
	reqCancel.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	rrCancel := httptest.NewRecorder()
	mux.ServeHTTP(rrCancel, reqCancel)

	if rrCancel.Code != http.StatusOK {
		t.Errorf("Expected status 200 from delete, got %d", rrCancel.Code)
	}

	// Check it is indeed cancelled
	jobCancel, _ := store.GetJob(submitResp.JobID)
	if jobCancel.JobState != "CANCELLED" {
		t.Errorf("Expected job to be CANCELLED, got %s", jobCancel.JobState)
	}
}

func TestSchedulerResourceManagement(t *testing.T) {
	store := NewClusterStore()
	scheduler := NewScheduler(store)

	// Submit job requiring 4 H100 GPUs and 8 CPUs
	job := store.SubmitJob("gpu_intensive", "h100-partition", 8, 4, 16000, 30)

	// Manually trigger scheduling reconciliation
	scheduler.reconcileJobs()

	// Verify job transitioned to RUNNING
	updatedJob, found := store.GetJob(job.JobID)
	if !found {
		t.Fatalf("Job %d not found in store", job.JobID)
	}
	if updatedJob.JobState != "RUNNING" {
		t.Errorf("Expected job to transition to RUNNING, got %s", updatedJob.JobState)
	}
	if updatedJob.NodeList != "node-1" {
		t.Errorf("Expected job scheduled on node-1, got %s", updatedJob.NodeList)
	}

	// Verify node resources are allocated
	node1 := store.Nodes["node-1"]
	if node1.CPUsUsed != 8 {
		t.Errorf("Expected 8 CPUs used on node-1, got %d", node1.CPUsUsed)
	}
	if node1.State != "MIXED" {
		t.Errorf("Expected node-1 state to be MIXED, got %s", node1.State)
	}
	gpuUsed := 0
	for _, g := range node1.GRESList {
		if g.Name == "gpu" {
			gpuUsed = g.Used
		}
	}
	if gpuUsed != 4 {
		t.Errorf("Expected 4 GPUs used on node-1, got %d", gpuUsed)
	}

	// Try to submit another job that exceeds available GPUs on partition
	// Partitions has node-1 (4 GPUs remaining) and node-2 (8 GPUs remaining)
	// We submit a job requesting 6 GPUs. It should fit on node-2!
	job2 := store.SubmitJob("gpu_intensive_2", "h100-partition", 8, 6, 16000, 30)
	scheduler.reconcileJobs()

	updatedJob2, _ := store.GetJob(job2.JobID)
	if updatedJob2.JobState != "RUNNING" || updatedJob2.NodeList != "node-2" {
		t.Errorf("Expected job2 scheduled on node-2, got %s (state %s)", updatedJob2.NodeList, updatedJob2.JobState)
	}

	// Now submit a job requiring 6 GPUs. It shouldn't fit anywhere since node-1 has 4 free, node-2 has 2 free.
	job3 := store.SubmitJob("unschedulable", "h100-partition", 8, 6, 16000, 30)
	scheduler.reconcileJobs()

	updatedJob3, _ := store.GetJob(job3.JobID)
	if updatedJob3.JobState != "PENDING" {
		t.Errorf("Expected job3 to remain PENDING due to capacity, got %s", updatedJob3.JobState)
	}

	// Cancel job1, which should free node-1 resources
	store.CancelJob(job.JobID)
	scheduler.reconcileJobs()

	// Now job3 should be schedulable on node-1!
	scheduler.reconcileJobs()
	updatedJob3, _ = store.GetJob(job3.JobID)
	if updatedJob3.JobState != "RUNNING" || updatedJob3.NodeList != "node-1" {
		t.Errorf("Expected job3 to run on node-1 after cancellation, got state=%s on %s", updatedJob3.JobState, updatedJob3.NodeList)
	}
}

func TestInvalidRequests(t *testing.T) {
	store := NewClusterStore()
	handler := NewSlurmHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Submit invalid JSON
	reqSubmitBad, _ := http.NewRequest("POST", "/slurm/v0.0.38/job/submit", bytes.NewReader([]byte("{invalid json")))
	reqSubmitBad.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	reqSubmitBad.Header.Set("Content-Type", "application/json")
	rrSubmitBad := httptest.NewRecorder()
	mux.ServeHTTP(rrSubmitBad, reqSubmitBad)
	if rrSubmitBad.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for bad JSON, got %d", rrSubmitBad.Code)
	}

	// 2. Query non-existent job
	reqGetBad, _ := http.NewRequest("GET", "/slurm/v0.0.38/job/99999", nil)
	reqGetBad.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	rrGetBad := httptest.NewRecorder()
	mux.ServeHTTP(rrGetBad, reqGetBad)
	if rrGetBad.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found for missing job, got %d", rrGetBad.Code)
	}

	// 3. Cancel non-existent job
	reqCancelBad, _ := http.NewRequest("DELETE", "/slurm/v0.0.38/job/99999", nil)
	reqCancelBad.Header.Set("X-SLURM-USER-TOKEN", "mock-jwt-token")
	rrCancelBad := httptest.NewRecorder()
	mux.ServeHTTP(rrCancelBad, reqCancelBad)
	if rrCancelBad.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found for cancelling missing job, got %d", rrCancelBad.Code)
	}
}

func TestMissingPartitionSubmit(t *testing.T) {
	store := NewClusterStore()
	scheduler := NewScheduler(store)

	// Submit job to a partition that does not exist
	job := store.SubmitJob("broken_job", "non-existent-partition", 1, 1, 1000, 10)
	
	// Scheduler should mark it as FAILED
	scheduler.reconcileJobs()
	
	updatedJob, _ := store.GetJob(job.JobID)
	if updatedJob.JobState != "FAILED" {
		t.Errorf("Expected job state to be FAILED for invalid partition, got %s", updatedJob.JobState)
	}
}

func TestJobCompletionLifecycle(t *testing.T) {
	store := NewClusterStore()
	scheduler := NewScheduler(store)

	// Submit a job with a 1 second duration
	job := store.SubmitJob("fast_job", "h100-partition", 1, 1, 1000, 1)

	// Schedule the job
	scheduler.reconcileJobs()
	
	runningJob, _ := store.GetJob(job.JobID)
	if runningJob.JobState != "RUNNING" {
		t.Fatalf("Expected job to start as RUNNING, got %s", runningJob.JobState)
	}

	// Verify resources allocated on node-1
	node := store.Nodes["node-1"]
	if node.CPUsUsed != 1 {
		t.Errorf("Expected 1 CPU allocated, got %d", node.CPUsUsed)
	}

	// Force-forward the job end time to simulate passing of time without sleeping
	store.mu.Lock()
	store.Jobs[job.JobID].EndTime.Number = 1 // Set to 1 (far in the past)
	store.mu.Unlock()

	// Reconcile again, it should transition the job to COMPLETED and free resources
	scheduler.reconcileJobs()

	completedJob, _ := store.GetJob(job.JobID)
	if completedJob.JobState != "COMPLETED" {
		t.Errorf("Expected job to transition to COMPLETED, got %s", completedJob.JobState)
	}

	// Verify resources freed
	if node.CPUsUsed != 0 {
		t.Errorf("Expected CPUs to be released, got %d used", node.CPUsUsed)
	}
}

func strconvFormat(v int64) string {
	return jsonNumberString(v)
}

func jsonNumberString(v int64) string {
	return bytesToString(v)
}

func bytesToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
