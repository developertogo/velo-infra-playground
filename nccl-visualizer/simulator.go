package main

import (
	"fmt"
	"sync"
)

// posMod implements a mathematically correct modulo for negative numbers.
func posMod(a, b int) int {
	return (a%b + b) % b
}

// GPU represents the virtual GPU executing its part of the collective.
type GPU struct {
	id             int
	numGPUs        int
	topology       TopologyType
	buffer         []float64
	initialBuffer  []float64
	rxChan         chan BlockMsg
	startStepChan  chan int
	doneStepChan   chan int
	sim            *Simulator
	parent         int   // for Tree topology (-1 if root)
	children       []int // for Tree topology
	depth          int   // for Tree topology
}

// Simulator manages the execution of the GPU goroutines.
type Simulator struct {
	mu           sync.RWMutex
	state        SimulationState
	gpus         []*GPU
	gpuChans     []chan BlockMsg
	startChans   []chan int
	doneChan     chan int
	maxDepth     int
	nodesByDepth map[int][]int
}

// NewSimulator initializes a new Simulator with the specified configuration.
func NewSimulator(topology TopologyType, numGPUs int) *Simulator {
	sim := &Simulator{
		doneChan:     make(chan int, numGPUs),
		gpuChans:     make([]chan BlockMsg, numGPUs),
		startChans:   make([]chan int, numGPUs),
		nodesByDepth: make(map[int][]int),
	}

	sim.state.Topology = topology
	sim.state.NumGPUs = numGPUs
	sim.state.BlockSize = numGPUs // Size of buffer is equal to number of GPUs
	sim.state.GPUs = make([]GPUState, numGPUs)
	sim.state.Logs = make([]SimLog, 0)

	// Initialize buffers and channels
	for i := 0; i < numGPUs; i++ {
		sim.gpuChans[i] = make(chan BlockMsg, numGPUs*2) // Buffered channel to prevent deadlocks
		sim.startChans[i] = make(chan int, 1)

		// Set initial buffers
		// GPU i has initial buffer where block j is (i+1)*10 + j+1
		buf := make([]float64, numGPUs)
		initBuf := make([]float64, numGPUs)
		for j := 0; j < numGPUs; j++ {
			val := float64((i+1)*10 + (j + 1))
			buf[j] = val
			initBuf[j] = val
		}

		gpu := &GPU{
			id:            i,
			numGPUs:       numGPUs,
			topology:      topology,
			buffer:        buf,
			initialBuffer: initBuf,
			rxChan:        sim.gpuChans[i],
			startStepChan: sim.startChans[i],
			doneStepChan:  sim.doneChan,
			sim:           sim,
			parent:        -1,
			children:      make([]int, 0),
		}
		sim.gpus = append(sim.gpus, gpu)
	}

	// Build Tree structure if needed
	if topology == TopologyTree {
		// Parent of i is (i-1)/2
		for i := 0; i < numGPUs; i++ {
			if i > 0 {
				p := (i - 1) / 2
				sim.gpus[i].parent = p
				sim.gpus[p].children = append(sim.gpus[p].children, i)
			}
		}

		// Compute node depths
		var computeDepth func(id int, d int)
		computeDepth = func(id int, d int) {
			sim.gpus[id].depth = d
			sim.nodesByDepth[d] = append(sim.nodesByDepth[d], id)
			if d > sim.maxDepth {
				sim.maxDepth = d
			}
			for _, child := range sim.gpus[id].children {
				computeDepth(child, d+1)
			}
		}
		computeDepth(0, 0)
		sim.state.MaxSteps = 2 * sim.maxDepth
	} else {
		// Ring steps = 2 * (N - 1)
		sim.state.MaxSteps = 2 * (numGPUs - 1)
	}

	sim.updateState()
	return sim
}

// Start spawns all GPU goroutines.
func (s *Simulator) Start() {
	for i := 0; i < s.state.NumGPUs; i++ {
		go s.gpus[i].loop()
	}
}

// Stop terminates all GPU goroutines.
func (s *Simulator) Stop() {
	for i := 0; i < s.state.NumGPUs; i++ {
		close(s.startChans[i])
	}
}

// Log adds a message to the simulation log.
func (s *Simulator) Log(msg string, isWarning bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Logs = append(s.state.Logs, SimLog{
		Step:      s.state.Step,
		Phase:     s.state.Phase,
		Message:   msg,
		IsWarning: isWarning,
	})
}

// GetState returns a copy of the current simulation state.
func (s *Simulator) GetState() SimulationState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Deep copy GPUs
	gpusCopy := make([]GPUState, len(s.state.GPUs))
	copy(gpusCopy, s.state.GPUs)
	
	logsCopy := make([]SimLog, len(s.state.Logs))
	copy(logsCopy, s.state.Logs)

	return SimulationState{
		Topology:   s.state.Topology,
		NumGPUs:    s.state.NumGPUs,
		BlockSize:  s.state.BlockSize,
		Step:       s.state.Step,
		MaxSteps:   s.state.MaxSteps,
		Phase:      s.state.Phase,
		GPUs:       gpusCopy,
		Logs:       logsCopy,
		Finished:   s.state.Finished,
	}
}

// updateState updates the SimulationState struct using the GPU states.
func (s *Simulator) updateState() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 0; i < s.state.NumGPUs; i++ {
		gpu := s.gpus[i]
		s.state.GPUs[i] = GPUState{
			ID:               gpu.id,
			Buffer:           append([]float64(nil), gpu.buffer...),
			InitialBuffer:    append([]float64(nil), gpu.initialBuffer...),
			Status:           gpu.Status(),
			LastSentTo:       gpu.LastSentTo(),
			LastReceivedFrom: gpu.LastReceivedFrom(),
			ActiveBlockIdx:   gpu.ActiveBlockIdx(),
		}
	}

	if s.state.Topology == TopologyRing {
		n := s.state.NumGPUs
		if s.state.Step < n-1 {
			s.state.Phase = "Reduce-Scatter"
		} else if s.state.Step < 2*(n-1) {
			s.state.Phase = "All-Gather"
		} else {
			s.state.Phase = "Completed"
			s.state.Finished = true
		}
	} else {
		if s.state.Step < s.maxDepth {
			s.state.Phase = "Upward-Reduce"
		} else if s.state.Step < 2*s.maxDepth {
			s.state.Phase = "Downward-Broadcast"
		} else {
			s.state.Phase = "Completed"
			s.state.Finished = true
		}
	}
}

// Step advances the simulation by one step.
func (s *Simulator) Step() bool {
	if s.state.Finished {
		return false
	}

	// Trigger all GPUs to start the step
	for i := 0; i < s.state.NumGPUs; i++ {
		s.startChans[i] <- s.state.Step
	}

	// Wait for all GPUs to complete the step
	for i := 0; i < s.state.NumGPUs; i++ {
		<-s.doneChan
	}

	// Advance step count
	s.state.Step++
	s.updateState()
	return !s.state.Finished
}

// GPU Goroutine Loop
func (g *GPU) loop() {
	for {
		step, ok := <-g.startStepChan
		if !ok {
			return // Simulator stopped
		}

		if g.topology == TopologyRing {
			g.runRingStep(step)
		} else {
			g.runTreeStep(step)
		}

		g.doneStepChan <- g.id
	}
}

// Status returns the current status text of the GPU.
func (g *GPU) Status() string {
	if g.sim.state.Finished {
		return "Done"
	}
	step := g.sim.state.Step
	if g.topology == TopologyRing {
		n := g.numGPUs
		if step < n-1 {
			return "Reduce-Scatter"
		} else if step < 2*(n-1) {
			return "All-Gather"
		}
		return "Idle"
	} else {
		maxD := g.sim.maxDepth
		if step < maxD {
			// Reduce Phase: check if this GPU sends/receives
			currDepth := maxD - step
			if g.depth == currDepth {
				return "Sending"
			} else if g.depth == currDepth-1 && len(g.children) > 0 {
				return "Receiving"
			}
			return "Idle"
		} else if step < 2*maxD {
			// Broadcast Phase
			currDepth := step - maxD
			if g.depth == currDepth {
				return "Broadcasting"
			} else if g.depth == currDepth+1 {
				return "Receiving"
			}
			return "Idle"
		}
		return "Idle"
	}
}

// LastSentTo returns which GPU was targeted in the current/last action.
func (g *GPU) LastSentTo() int {
	step := g.sim.state.Step
	if g.topology == TopologyRing {
		if step < 2*(g.numGPUs-1) {
			return (g.id + 1) % g.numGPUs
		}
	} else {
		maxD := g.sim.maxDepth
		if step < maxD {
			// Reduce phase: node sends to parent
			currDepth := maxD - step
			if g.depth == currDepth && g.parent != -1 {
				return g.parent
			}
		} else if step < 2*maxD {
			// Broadcast phase: node sends to children
			currDepth := step - maxD
			if g.depth == currDepth && len(g.children) > 0 {
				// We target all children, return first one or just indicate there is action
				if len(g.children) > 0 {
					return g.children[0] // Simplify for visualization
				}
			}
		}
	}
	return -1
}

// LastReceivedFrom returns which GPU sent data in the current/last action.
func (g *GPU) LastReceivedFrom() int {
	step := g.sim.state.Step
	if g.topology == TopologyRing {
		if step < 2*(g.numGPUs-1) {
			return posMod(g.id-1, g.numGPUs)
		}
	} else {
		maxD := g.sim.maxDepth
		if step < maxD {
			// Reduce: parent receives from child
			currDepth := maxD - step
			if g.depth == currDepth-1 {
				// Parent receives from children at depth currDepth
				for _, c := range g.children {
					if g.sim.gpus[c].depth == currDepth {
						return c
					}
				}
			}
		} else if step < 2*maxD {
			// Broadcast: child receives from parent
			currDepth := step - maxD
			if g.depth == currDepth+1 {
				return g.parent
			}
		}
	}
	return -1
}

// ActiveBlockIdx returns the index of the block active in this step.
func (g *GPU) ActiveBlockIdx() int {
	step := g.sim.state.Step
	n := g.numGPUs
	if g.topology == TopologyRing {
		if step < n-1 {
			// Reduce-Scatter step
			return posMod(g.id-step, n)
		} else if step < 2*(n-1) {
			// All-Gather step
			stepAg := step - (n - 1)
			return posMod(g.id+1-stepAg, n)
		}
	}
	return -1
}

// runRingStep executes a single step of the Ring AllReduce algorithm.
func (g *GPU) runRingStep(step int) {
	n := g.numGPUs
	if step < n-1 {
		// --- Reduce-Scatter Phase ---
		// GPU i sends block (i - step) % n to GPU i+1
		sendBlockIdx := posMod(g.id-step, n)
		sendVal := g.buffer[sendBlockIdx]
		nextGPUID := (g.id + 1) % n

		// Send block
		g.sim.gpuChans[nextGPUID] <- BlockMsg{
			SenderID:   g.id,
			ReceiverID: nextGPUID,
			BlockIdx:   sendBlockIdx,
			Values:     []float64{sendVal},
			Step:       step,
			Phase:      "Reduce-Scatter",
		}
		g.sim.Log(fmt.Sprintf("GPU %d sent Block %d (val %.1f) to GPU %d", g.id, sendBlockIdx, sendVal, nextGPUID), false)

		// Receive block (comes from GPU i-1)
		msg := <-g.rxChan
		// Reduce received block into our own buffer
		oldVal := g.buffer[msg.BlockIdx]
		g.buffer[msg.BlockIdx] += msg.Values[0]

		g.sim.Log(fmt.Sprintf("GPU %d received Block %d from GPU %d: %.1f + %.1f = %.1f", 
			g.id, msg.BlockIdx, msg.SenderID, oldVal, msg.Values[0], g.buffer[msg.BlockIdx]), false)

	} else if step < 2*(n-1) {
		// --- All-Gather Phase ---
		stepAg := step - (n - 1)
		// GPU i sends block (i + 1 - stepAg) % n to GPU i+1
		sendBlockIdx := posMod(g.id+1-stepAg, n)
		sendVal := g.buffer[sendBlockIdx]
		nextGPUID := (g.id + 1) % n

		// Send block
		g.sim.gpuChans[nextGPUID] <- BlockMsg{
			SenderID:   g.id,
			ReceiverID: nextGPUID,
			BlockIdx:   sendBlockIdx,
			Values:     []float64{sendVal},
			Step:       step,
			Phase:      "All-Gather",
		}
		g.sim.Log(fmt.Sprintf("GPU %d gathered Block %d (val %.1f) to GPU %d", g.id, sendBlockIdx, sendVal, nextGPUID), false)

		// Receive block
		msg := <-g.rxChan
		// Overwrite local block with fully reduced block value
		g.buffer[msg.BlockIdx] = msg.Values[0]

		g.sim.Log(fmt.Sprintf("GPU %d synchronized Block %d from GPU %d to final value %.1f", 
			g.id, msg.BlockIdx, msg.SenderID, msg.Values[0]), false)
	}
}

// runTreeStep executes a single step of the Tree AllReduce algorithm.
func (g *GPU) runTreeStep(step int) {
	maxD := g.sim.maxDepth

	if step < maxD {
		// --- Upward Reduce Phase ---
		// Current active depth that sends is (maxD - step)
		activeDepth := maxD - step

		if g.depth == activeDepth {
			// This GPU is sending its buffer to its parent
			if g.parent != -1 {
				// Send entire accumulated buffer
				bufCopy := append([]float64(nil), g.buffer...)
				g.sim.gpuChans[g.parent] <- BlockMsg{
					SenderID:   g.id,
					ReceiverID: g.parent,
					Values:     bufCopy,
					Step:       step,
					Phase:      "Upward-Reduce",
				}
				g.sim.Log(fmt.Sprintf("GPU %d (leaf/child at depth %d) sent buffer to parent GPU %d", g.id, g.depth, g.parent), false)
			}
		} else if g.depth == activeDepth-1 {
			// This GPU is a parent expecting data from children at activeDepth
			// We only proceed after receiving from all children that are at activeDepth
			expectedCount := 0
			for _, child := range g.children {
				if g.sim.gpus[child].depth == activeDepth {
					expectedCount++
				}
			}

			for i := 0; i < expectedCount; i++ {
				msg := <-g.rxChan
				// Reduce the entire buffer block by block
				for idx := 0; idx < len(g.buffer); idx++ {
					g.buffer[idx] += msg.Values[idx]
				}
				g.sim.Log(fmt.Sprintf("GPU %d (parent) reduced buffer received from child GPU %d", g.id, msg.SenderID), false)
			}
		}
	} else if step < 2*maxD {
		// --- Downward Broadcast Phase ---
		// Current active depth that broadcasts is (step - maxD)
		activeDepth := step - maxD

		if g.depth == activeDepth {
			// This GPU is broadcasting the fully reduced buffer to children
			for _, child := range g.children {
				bufCopy := append([]float64(nil), g.buffer...)
				g.sim.gpuChans[child] <- BlockMsg{
					SenderID:   g.id,
					ReceiverID: child,
					Values:     bufCopy,
					Step:       step,
					Phase:      "Downward-Broadcast",
				}
				g.sim.Log(fmt.Sprintf("GPU %d (parent at depth %d) broadcasted reduced buffer to child GPU %d", g.id, g.depth, child), false)
			}
		} else if g.depth == activeDepth+1 {
			// This GPU is receiving the broadcast from parent
			msg := <-g.rxChan
			// Overwrite entire buffer with fully reduced parent buffer
			copy(g.buffer, msg.Values)
			g.sim.Log(fmt.Sprintf("GPU %d (child) updated buffer from parent GPU %d broadcast", g.id, msg.SenderID), false)
		}
	}
}

// FormatBuffer formats the slice nicely for printing.
func FormatBuffer(buf []float64) string {
	res := "["
	for i, v := range buf {
		if i > 0 {
			res += ", "
		}
		res += fmt.Sprintf("%5.1f", v)
	}
	res += "]"
	return res
}
