package main

// TopologyType defines the type of communication topology.
type TopologyType string

const (
	TopologyRing TopologyType = "Ring"
	TopologyTree TopologyType = "Tree"
)

// GPUState represents the current state of a simulated GPU.
type GPUState struct {
	ID                 int
	Buffer             []float64
	InitialBuffer      []float64
	Status             string // e.g., "Idle", "Sending", "Receiving", "Reducing", "Broadcasting", "Done"
	LastSentTo         int    // ID of the GPU we last sent a message to (-1 if none)
	LastReceivedFrom   int    // ID of the GPU we last received a message from (-1 if none)
	ActiveBlockIdx     int    // Index of the block currently being processed (-1 if none)
}

// BlockMsg is the message payload sent through the channels between GPUs.
type BlockMsg struct {
	SenderID   int
	ReceiverID int
	BlockIdx   int
	Values     []float64 // Can be a single block value or the entire buffer
	Step       int
	Phase      string // "Reduce-Scatter", "All-Gather", "Reduce-Up", "Broadcast-Down"
}

// SimLog represents a historical log message for the visualizer.
type SimLog struct {
	Step      int
	Phase     string
	Message   string
	IsWarning bool
}

// SimulationState holds the overall status of the running simulation.
type SimulationState struct {
	Topology   TopologyType
	NumGPUs    int
	BlockSize  int
	Step       int
	MaxSteps   int
	Phase      string
	GPUs       []GPUState
	Logs       []SimLog
	Finished   bool
}
