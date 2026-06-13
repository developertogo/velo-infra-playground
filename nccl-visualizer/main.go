package main

import (
	"flag"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Message represents the data block transmitted between GPUs in the ring.
type Message struct {
	Step       int
	ChunkIndex int
	Value      float64
}

// GPUNode represents a virtual GPU executing its slice of the Ring-AllReduce.
type GPUNode struct {
	ID        int
	Data      []float64
	SendChan  chan Message
	RecvChan  chan Message
	StepMutex sync.Mutex
}

func main() {
	numGPUs := flag.Int("nodes", 4, "Number of GPU nodes in the ring (minimum 2)")
	delayMs := flag.Int("delay", 800, "Latency delay in milliseconds per transfer step")
	flag.Parse()

	if *numGPUs < 2 {
		fmt.Println("Error: Ring topology requires at least 2 nodes.")
		return
	}

	fmt.Printf("\033[1;36m====================================================\033[0m\n")
	fmt.Printf("\033[1;36m   NVIDIA NCCL Ring-AllReduce Simulator (macOS M3)   \033[0m\n")
	fmt.Printf("\033[1;36m====================================================\033[0m\n")
	fmt.Printf("Nodes (GPUs): %d\n", *numGPUs)
	fmt.Printf("Simulated Interconnect Latency: %dms\n\n", *delayMs)

	// 1. Initialize data on each GPU.
	// For visualization, we seed them with distinct scales (e.g. Node 0 has units, Node 1 has tens, etc.)
	gpuData := make([][]float64, *numGPUs)
	for i := 0; i < *numGPUs; i++ {
		gpuData[i] = make([]float64, *numGPUs)
		scale := 1.0
		for k := 0; k < i; k++ {
			scale *= 10.0
		}
		for j := 0; j < *numGPUs; j++ {
			// e.g. Node 0: [1, 2, 3, 4], Node 1: [10, 20, 30, 40], Node 2: [100, 200, 300, 400]
			gpuData[i][j] = float64(j+1) * scale
		}
	}

	// Calculate the expected final averaged array
	expectedSum := make([]float64, *numGPUs)
	for j := 0; j < *numGPUs; j++ {
		sum := 0.0
		for i := 0; i < *numGPUs; i++ {
			sum += gpuData[i][j]
		}
		expectedSum[j] = sum
	}

	fmt.Println("\033[1;33mInitial GPU States:\033[0m")
	for i := 0; i < *numGPUs; i++ {
		fmt.Printf("  GPU %d: %v\n", i, gpuData[i])
	}
	fmt.Printf("  Expected Target Sum Array: %v\n\n", expectedSum)

	// Print Ring Topology
	printRingTopology(*numGPUs)

	// 2. Set up communication ring of channels.
	// channels[i] represents the input channel to GPU i.
	// GPU i receives from channels[i], and sends to channels[(i+1)%N].
	channels := make([]chan Message, *numGPUs)
	for i := 0; i < *numGPUs; i++ {
		channels[i] = make(chan Message, 1) // buffered size 1 to prevent deadlock
	}

	// 3. Create GPU Nodes
	gpus := make([]*GPUNode, *numGPUs)
	for i := 0; i < *numGPUs; i++ {
		gpus[i] = &GPUNode{
			ID:       i,
			Data:     make([]float64, *numGPUs),
			SendChan: channels[(i+1)%*numGPUs], // Sends to next node in ring
			RecvChan: channels[i],               // Receives from its own input channel
		}
		copy(gpus[i].Data, gpuData[i])
	}

	// Synchronization barriers
	var wg sync.WaitGroup
	wg.Add(*numGPUs)

	// Lock for synchronized print statements to keep output readable
	var printMutex sync.Mutex

	// Start GPU worker goroutines
	start := time.Now()
	for i := 0; i < *numGPUs; i++ {
		go func(gpu *GPUNode) {
			defer wg.Done()

			delay := time.Duration(*delayMs) * time.Millisecond

			// ----------------------------------------------------
			// PHASE 1: Reduce-Scatter
			// Each node sends parts of its data to its neighbor.
			// After N-1 steps, each node i has the fully accumulated sum
			// for exactly one chunk index: (i+1)%N.
			// ----------------------------------------------------
			for step := 0; step < *numGPUs-1; step++ {
				// Index of chunk this GPU sends
				cSend := (gpu.ID - step + *numGPUs) % *numGPUs
				// Index of chunk this GPU receives and accumulates
				cRecv := (gpu.ID - step - 1 + *numGPUs) % *numGPUs

				valToSend := gpu.Data[cSend]

				printMutex.Lock()
				fmt.Printf("\033[34m[Reduce-Scatter Step %d]\033[0m GPU %d sending chunk %d (val: %.2f) to GPU %d\n",
					step, gpu.ID, cSend, valToSend, (gpu.ID+1)%*numGPUs)
				printMutex.Unlock()

				// Wire transmission latency simulation
				time.Sleep(delay)

				// Non-blocking write to next node
				gpu.SendChan <- Message{
					Step:       step,
					ChunkIndex: cSend,
					Value:      valToSend,
				}

				// Read from previous node
				msg := <-gpu.RecvChan

				// Verify step synchronization
				if msg.Step != step {
					fmt.Printf("Error: Sync issues. GPU %d received message from step %d but is in step %d\n", gpu.ID, msg.Step, step)
				}

				// Accumulate value
				gpu.Data[cRecv] += msg.Value

				printMutex.Lock()
				fmt.Printf("                  GPU %d received chunk %d from GPU %d. New State: %v\n",
					gpu.ID, cRecv, (gpu.ID-1+*numGPUs)%*numGPUs, formatData(gpu.Data, cRecv))
				printMutex.Unlock()

				// Brief pause to synchronize steps visually
				time.Sleep(100 * time.Millisecond)
			}

			// ----------------------------------------------------
			// PHASE 2: All-Gather
			// Each node broadcasts its fully accumulated chunk.
			// After N-1 steps, every node has the fully reduced array.
			// ----------------------------------------------------
			for step := 0; step < *numGPUs-1; step++ {
				// Index of chunk this GPU sends (starts with the one it fully reduced: (ID+1)%N)
				cSend := (gpu.ID + 1 - step + *numGPUs) % *numGPUs
				// Index of chunk this GPU receives and overwrites
				cRecv := (gpu.ID - step + *numGPUs) % *numGPUs

				valToSend := gpu.Data[cSend]

				printMutex.Lock()
				fmt.Printf("\033[32m[All-Gather Step %d]\033[0m     GPU %d sending final chunk %d (val: %.2f) to GPU %d\n",
					step, gpu.ID, cSend, valToSend, (gpu.ID+1)%*numGPUs)
				printMutex.Unlock()

				time.Sleep(delay)

				gpu.SendChan <- Message{
					Step:       step,
					ChunkIndex: cSend,
					Value:      valToSend,
				}

				msg := <-gpu.RecvChan

				// Overwrite local value with the fully reduced value
				gpu.Data[cRecv] = msg.Value

				printMutex.Lock()
				fmt.Printf("                  GPU %d received final chunk %d from GPU %d. New State: %v\n",
					gpu.ID, cRecv, (gpu.ID-1+*numGPUs)%*numGPUs, formatData(gpu.Data, cRecv))
				printMutex.Unlock()

				time.Sleep(100 * time.Millisecond)
			}
		}(gpus[i])
	}

	wg.Wait()
	duration := time.Since(start)

	fmt.Printf("\n\033[1;36m====================================================\033[0m\n")
	fmt.Printf("\033[1;36m                Simulation Completed                \033[0m\n")
	fmt.Printf("\033[1;36m====================================================\033[0m\n")
	fmt.Printf("Total execution time: %v\n\n", duration)

	fmt.Println("\033[1;33mFinal GPU States:\033[0m")
	allSucceeded := true
	for i := 0; i < *numGPUs; i++ {
		match := true
		for j := 0; j < *numGPUs; j++ {
			if gpus[i].Data[j] != expectedSum[j] {
				match = false
				allSucceeded = false
			}
		}
		statusColor := "\033[32mSUCCESS\033[0m"
		if !match {
			statusColor = "\033[31mFAILED\033[0m"
		}
		fmt.Printf("  GPU %d: %v [%s]\n", i, gpus[i].Data, statusColor)
	}

	if allSucceeded {
		fmt.Println("\n\033[1;32mRing-AllReduce finished successfully! All nodes synchronized perfectly.\033[0m")
	} else {
		fmt.Println("\n\033[1;31mRing-AllReduce verification failed. Data mismatch found.\033[0m")
	}
}

// formatData underlines or highlights the modified index for clarity in logs.
func formatData(data []float64, highlightIdx int) string {
	res := "["
	for i, val := range data {
		if i == highlightIdx {
			res += fmt.Sprintf("\033[4;33m%.1f\033[0m", val) // underlined yellow
		} else {
			res += fmt.Sprintf("%.1f", val)
		}
		if i < len(data)-1 {
			res += ", "
		}
	}
	res += "]"
	return res
}

func printRingTopology(nodes int) {
	fmt.Println("\033[1;35mLogical Communication Ring Topology:\033[0m")
	if nodes == 4 {
		fmt.Println("       [GPU 0] ==========(NVLink)==========> [GPU 1]")
		fmt.Println("          ^                                     |")
		fmt.Println("          |                                     |")
		fmt.Println("       (NVLink)                              (NVLink)")
		fmt.Println("          |                                     |")
		fmt.Println("          |                                     v")
		fmt.Println("       [GPU 3] <==========(NVLink)========== [GPU 2]")
	} else {
		for i := 0; i < nodes; i++ {
			fmt.Printf("   [GPU %d] --(NVLink)--> ", i)
		}
		fmt.Printf("[GPU 0] (Loop)\n")
	}
	fmt.Println()
}

// Seed random source for potential dynamic network jitter simulations
func init() {
	rand.Seed(time.Now().UnixNano())
}
