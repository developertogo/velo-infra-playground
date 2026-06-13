package main

import (
	"testing"
)

// TestNewSimulator verifies that the simulator correctly initializes state,
// buffers, and topology details for both Ring and Tree modes.
func TestNewSimulator(t *testing.T) {
	// 1. Ring Init
	simRing := NewSimulator(TopologyRing, 4)
	if simRing.state.Topology != TopologyRing {
		t.Errorf("Expected Ring topology, got %s", simRing.state.Topology)
	}
	if len(simRing.gpus) != 4 {
		t.Errorf("Expected 4 GPUs initialized, got %d", len(simRing.gpus))
	}
	if simRing.state.MaxSteps != 6 { // 2 * (N - 1) = 6
		t.Errorf("Expected MaxSteps to be 6 for Ring N=4, got %d", simRing.state.MaxSteps)
	}

	// Verify buffer values
	// GPU i block j is (i+1)*10 + j+1
	// GPU 0 should have [11, 12, 13, 14]
	expectedGPU0 := []float64{11, 12, 13, 14}
	for j, v := range simRing.gpus[0].buffer {
		if v != expectedGPU0[j] {
			t.Errorf("GPU 0 block %d got %f, expected %f", j, v, expectedGPU0[j])
		}
	}

	// 2. Tree Init
	simTree := NewSimulator(TopologyTree, 4)
	if simTree.state.Topology != TopologyTree {
		t.Errorf("Expected Tree topology, got %s", simTree.state.Topology)
	}
	// Verify tree structure: parent of i is (i-1)/2
	// Node 0: Root, parent=-1, children=[1, 2]
	if simTree.gpus[0].parent != -1 {
		t.Errorf("Expected Node 0 parent to be -1, got %d", simTree.gpus[0].parent)
	}
	if len(simTree.gpus[0].children) != 2 || simTree.gpus[0].children[0] != 1 || simTree.gpus[0].children[1] != 2 {
		t.Errorf("Expected Node 0 children to be [1, 2], got %v", simTree.gpus[0].children)
	}
	// Node 3: Child of 1, depth=2
	if simTree.gpus[3].parent != 1 {
		t.Errorf("Expected Node 3 parent to be 1, got %d", simTree.gpus[3].parent)
	}
	if simTree.gpus[3].depth != 2 {
		t.Errorf("Expected Node 3 depth to be 2, got %d", simTree.gpus[3].depth)
	}
	if simTree.maxDepth != 2 {
		t.Errorf("Expected maxDepth to be 2 for Tree N=4, got %d", simTree.maxDepth)
	}
	if simTree.state.MaxSteps != 4 { // 2 * maxDepth = 4
		t.Errorf("Expected MaxSteps to be 4 for Tree N=4, got %d", simTree.state.MaxSteps)
	}
}

// TestRingAllReduceCorrectness tests Ring AllReduce correctness with N=4 nodes.
func TestRingAllReduceCorrectness(t *testing.T) {
	numGPUs := 4
	sim := NewSimulator(TopologyRing, numGPUs)
	sim.Start()
	defer sim.Stop()

	for !sim.state.Finished {
		sim.Step()
	}

	state := sim.GetState()

	// Expected sum for each block:
	// Block 0: 11 + 21 + 31 + 41 = 104
	// Block 1: 12 + 22 + 32 + 42 = 108
	// Block 2: 13 + 23 + 33 + 43 = 112
	// Block 3: 14 + 24 + 34 + 44 = 116
	expected := []float64{104, 108, 112, 116}

	for i := 0; i < numGPUs; i++ {
		gpu := state.GPUs[i]
		for j := 0; j < numGPUs; j++ {
			if gpu.Buffer[j] != expected[j] {
				t.Errorf("GPU %d Block %d got %f, expected %f", i, j, gpu.Buffer[j], expected[j])
			}
		}
	}
}

// TestTreeAllReduceCorrectness tests Tree AllReduce correctness with N=4 nodes.
func TestTreeAllReduceCorrectness(t *testing.T) {
	numGPUs := 4
	sim := NewSimulator(TopologyTree, numGPUs)
	sim.Start()
	defer sim.Stop()

	for !sim.state.Finished {
		sim.Step()
	}

	state := sim.GetState()

	expected := []float64{104, 108, 112, 116}

	for i := 0; i < numGPUs; i++ {
		gpu := state.GPUs[i]
		for j := 0; j < numGPUs; j++ {
			if gpu.Buffer[j] != expected[j] {
				t.Errorf("GPU %d Block %d got %f, expected %f", i, j, gpu.Buffer[j], expected[j])
			}
		}
	}
}

// TestVaryingNodeCounts tests Ring and Tree topologies across different cluster sizes.
func TestVaryingNodeCounts(t *testing.T) {
	testSizes := []int{2, 3, 5, 8}

	for _, size := range testSizes {
		t.Run(string(TopologyRing)+"_Size_"+tName(size), func(t *testing.T) {
			sim := NewSimulator(TopologyRing, size)
			sim.Start()
			defer sim.Stop()

			for !sim.state.Finished {
				sim.Step()
			}

			// Compute expected sum manually for each block
			expected := make([]float64, size)
			for j := 0; j < size; j++ {
				sum := 0.0
				for i := 0; i < size; i++ {
					sum += float64((i+1)*10 + j + 1)
				}
				expected[j] = sum
			}

			state := sim.GetState()
			for i := 0; i < size; i++ {
				for j := 0; j < size; j++ {
					if state.GPUs[i].Buffer[j] != expected[j] {
						t.Errorf("Size %d GPU %d Block %d got %f, expected %f", size, i, j, state.GPUs[i].Buffer[j], expected[j])
					}
				}
			}
		})

		t.Run(string(TopologyTree)+"_Size_"+tName(size), func(t *testing.T) {
			sim := NewSimulator(TopologyTree, size)
			sim.Start()
			defer sim.Stop()

			for !sim.state.Finished {
				sim.Step()
			}

			// Compute expected sum manually for each block
			expected := make([]float64, size)
			for j := 0; j < size; j++ {
				sum := 0.0
				for i := 0; i < size; i++ {
					sum += float64((i+1)*10 + j + 1)
				}
				expected[j] = sum
			}

			state := sim.GetState()
			for i := 0; i < size; i++ {
				for j := 0; j < size; j++ {
					if state.GPUs[i].Buffer[j] != expected[j] {
						t.Errorf("Size %d GPU %d Block %d got %f, expected %f", size, i, j, state.GPUs[i].Buffer[j], expected[j])
					}
				}
			}
		})
	}
}

// TestStateTransitions verifies that phase transitions and finish signals are exact.
func TestStateTransitions(t *testing.T) {
	numGPUs := 4
	sim := NewSimulator(TopologyRing, numGPUs)
	sim.Start()
	defer sim.Stop()

	// Initial checks
	state := sim.GetState()
	if state.Step != 0 {
		t.Errorf("Expected initial step 0, got %d", state.Step)
	}
	if state.Phase != "Reduce-Scatter" {
		t.Errorf("Expected initial phase 'Reduce-Scatter', got %s", state.Phase)
	}
	if state.Finished {
		t.Error("Expected finished to be false initially")
	}

	// Advance steps of Reduce-Scatter (N-1 steps, steps 0 to 2)
	for i := 0; i < 3; i++ {
		sim.Step()
		state = sim.GetState()
		if state.Step != i+1 {
			t.Errorf("Expected step %d, got %d", i+1, state.Step)
		}
		if i < 2 {
			if state.Phase != "Reduce-Scatter" {
				t.Errorf("Expected phase 'Reduce-Scatter' at step %d, got %s", state.Step, state.Phase)
			}
		} else {
			// At Step 3, phase should transition to All-Gather
			if state.Phase != "All-Gather" {
				t.Errorf("Expected phase transition to 'All-Gather' at step 3, got %s", state.Phase)
			}
		}
	}

	// Advance remaining steps of All-Gather (N-1 steps, steps 3 to 5)
	for i := 3; i < 6; i++ {
		sim.Step()
		state = sim.GetState()
		if i < 5 {
			if state.Phase != "All-Gather" {
				t.Errorf("Expected phase 'All-Gather' at step %d, got %s", state.Step, state.Phase)
			}
			if state.Finished {
				t.Errorf("Expected Finished to be false at step %d", state.Step)
			}
		} else {
			// At Step 6, it should be Completed
			if state.Phase != "Completed" {
				t.Errorf("Expected phase 'Completed' at final step, got %s", state.Phase)
			}
			if !state.Finished {
				t.Error("Expected Finished to be true at final step")
			}
		}
	}
}

// Helper to convert size to name
func tName(size int) string {
	switch size {
	case 2:
		return "2"
	case 3:
		return "3"
	case 5:
		return "5"
	case 8:
		return "8"
	default:
		return "unknown"
	}
}
