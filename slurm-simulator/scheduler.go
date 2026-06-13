package main

import (
	"fmt"
	"strings"
	"time"
)

// Scheduler coordinates the lifecycle transitions of mock jobs in the ClusterStore.
type Scheduler struct {
	store  *ClusterStore
	ticker *time.Ticker
	stop   chan struct{}
}

// NewScheduler creates a scheduler associated with the store.
func NewScheduler(store *ClusterStore) *Scheduler {
	return &Scheduler{
		store: store,
		stop:  make(chan struct{}),
	}
}

// Start runs the scheduling loop in the background.
func (s *Scheduler) Start() {
	s.ticker = time.NewTicker(1 * time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.reconcileJobs()
			case <-s.stop:
				s.ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops the background scheduler.
func (s *Scheduler) Stop() {
	close(s.stop)
}

// reconcileJobs scans the database to start pending jobs and complete running ones.
func (s *Scheduler) reconcileJobs() {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	now := time.Now().Unix()

	// 1. Complete running jobs whose end times have passed
	for _, job := range s.store.Jobs {
		if job.JobState == "RUNNING" && now >= job.EndTime.Number {
			job.JobState = "COMPLETED"
			s.store.releaseResources(job)
			fmt.Printf("[Scheduler] Job %d (%s) COMPLETED on node %s\n", job.JobID, job.Name, job.NodeList)
		}
	}

	// 2. Schedule pending jobs if resources are available
	for _, job := range s.store.Jobs {
		if job.JobState == "PENDING" {
			s.tryScheduleJob(job, now)
		}
	}
}

// tryScheduleJob looks for a node in the partition with enough resources to run the job.
func (s *Scheduler) tryScheduleJob(job *SlurmJob, now int64) {
	part, ok := s.store.Partitions[job.Partition]
	if !ok {
		// Partition not found, fail the job
		job.JobState = "FAILED"
		fmt.Printf("[Scheduler] Job %d FAILED: Partition '%s' not found\n", job.JobID, job.Partition)
		return
	}

	// Get list of nodes in this partition
	nodesInPart := strings.Split(part.Nodes, ",")

	// Simple placement strategy: First-fit node allocation
	for _, nodeName := range nodesInPart {
		node, exists := s.store.Nodes[nodeName]
		if !exists {
			continue
		}

		// Check CPUs
		if node.CPUs - node.CPUsUsed < job.CPUs {
			continue
		}

		// Check memory (if specified)
		if job.Memory.Set && (node.Memory - node.MemoryUsed < job.Memory.Number) {
			continue
		}

		// Check GPUs
		gpuAvailable := 0
		gpuIdx := -1
		for idx, g := range node.GRESList {
			if g.Name == "gpu" {
				gpuAvailable = g.Count - g.Used
				gpuIdx = idx
				break
			}
		}

		if gpuIdx == -1 || gpuAvailable < job.GPUs {
			continue
		}

		// Allocate resources on this node!
		node.CPUsUsed += job.CPUs
		if job.Memory.Set {
			node.MemoryUsed += job.Memory.Number
		}
		node.GRESList[gpuIdx].Used += job.GPUs
		s.store.recalculateNodeState(node)

		// Set job runtime parameters
		job.JobState = "RUNNING"
		job.NodeList = node.Name
		job.StartTime = TimeObject{
			Number: now,
			Set:    true,
		}
		job.EndTime = TimeObject{
			Number: now + job.Duration,
			Set:    true,
		}
		job.GresAllocStr = fmt.Sprintf("cpu=%d,node=1,gres/gpu=%d", job.CPUs, job.GPUs)

		fmt.Printf("[Scheduler] Scheduled Job %d (%s) on %s (allocated %d CPUs, %d GPUs)\n", 
			job.JobID, job.Name, node.Name, job.CPUs, job.GPUs)
		return
	}
}
