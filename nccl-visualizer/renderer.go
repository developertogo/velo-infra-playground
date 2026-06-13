package main

import (
	"fmt"
	"strings"
)

// ANSI color escape codes
const (
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
	ColorUnder  = "\033[4m"
	
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorGray    = "\033[90m"

	BgBlue    = "\033[44m"
	BgCyan    = "\033[46m"
	BgGray    = "\033[100m"
)

// ClearScreen clears the terminal screen and resets cursor position.
func ClearScreen() {
	fmt.Print("\033[H\033[2J")
}

// RenderDashboard prints the complete CLI visualization panel.
func RenderDashboard(state SimulationState) {
	ClearScreen()
	DrawHeader(state)
	DrawTopology(state)
	DrawBufferStates(state)
	DrawLogs(state)
	DrawControls(state)
}

// DrawHeader displays the header banner with simulation parameters.
func DrawHeader(state SimulationState) {
	fmt.Printf("%s%s  NCCL COLLECTIVE COMMUNICATION SIMULATOR  %s\n", ColorBold, BgBlue+ColorWhite, ColorReset)
	
	// Create a progress bar
	progressWidth := 30
	var bar string
	if state.MaxSteps > 0 {
		filled := int(float64(state.Step) / float64(state.MaxSteps) * float64(progressWidth))
		bar = strings.Repeat("█", filled) + strings.Repeat("░", progressWidth-filled)
	} else {
		bar = strings.Repeat("░", progressWidth)
	}

	statusStr := ColorGreen + "RUNNING" + ColorReset
	if state.Finished {
		statusStr = ColorCyan + "COMPLETED" + ColorReset
	}

	fmt.Println(strings.Repeat("─", 70))
	fmt.Printf("%sTopology:%s %-12s | %sPhase:%s %-20s | %sStatus:%s %s\n", 
		ColorBold, ColorReset, state.Topology, 
		ColorBold, ColorReset, state.Phase, 
		ColorBold, ColorReset, statusStr)
	fmt.Printf("%sStep:%s %d/%d %s%s%s\n", 
		ColorBold, ColorReset, state.Step, state.MaxSteps, 
		ColorCyan, bar, ColorReset)
	fmt.Println(strings.Repeat("─", 70))
}

// DrawTopology draws the graphical representation of the GPU layout and communication flow.
func DrawTopology(state SimulationState) {
	fmt.Printf("%s[ TOPOLOGY DIAGRAM ]%s\n", ColorBold+ColorCyan, ColorReset)

	if state.Topology == TopologyRing {
		drawRingTopology(state)
	} else {
		drawTreeTopology(state)
	}
	fmt.Println()
}

func drawRingTopology(state SimulationState) {
	// Ring diagram for 4 GPUs
	// We want to highlight arrows when active
	arrowColor := ColorGreen
	if state.Finished {
		arrowColor = ColorGray
	}

	fmt.Printf("      ┌─────────┐                ┌─────────┐\n")
	fmt.Printf("      │  %sGPU 0%s  │  %s────────────>%s  │  %sGPU 1%s  │\n", 
		ColorBold+ColorMagenta, ColorReset, arrowColor, ColorReset, ColorBold+ColorMagenta, ColorReset)
	fmt.Printf("      └─────────┘                └─────────┘\n")
	fmt.Printf("           ▲                          │\n")
	fmt.Printf("           │ %s                        │ %s\n", arrowColor, ColorReset)
	fmt.Printf("           │ %s                        ▼ %s\n", arrowColor, ColorReset)
	fmt.Printf("      ┌─────────┐                ┌─────────┐\n")
	fmt.Printf("      │  %sGPU 3%s  │  %s<────────────%s  │  %sGPU 2%s  │\n", 
		ColorBold+ColorMagenta, ColorReset, arrowColor, ColorReset, ColorBold+ColorMagenta, ColorReset)
	fmt.Printf("      └─────────┘                └─────────┘\n")
}

func drawTreeTopology(state SimulationState) {
	// Tree diagram for 4 GPUs
	//          [GPU 0]
	//         /       \
	//     [GPU 1]   [GPU 2]
	//       /
	//   [GPU 3]
	
	// We color links based on whether they are active in the current step
	l01, l02, l13 := ColorGray, ColorGray, ColorGray
	dir01, dir02, dir13 := "──", "──", "──" // neutral

	if !state.Finished {
		maxD := state.MaxSteps / 2
		if state.Step < maxD {
			// Reduce Phase (Upward)
			activeDepth := maxD - state.Step
			if activeDepth == 2 {
				l13 = ColorYellow
				dir13 = "▲ " // Sending up
			} else if activeDepth == 1 {
				l01 = ColorYellow
				dir01 = "▲ "
				l02 = ColorYellow
				dir02 = " ▲"
			}
		} else {
			// Broadcast Phase (Downward)
			activeDepth := state.Step - maxD
			if activeDepth == 0 {
				l01 = ColorGreen
				dir01 = "▼ " // Broadcasting down
				l02 = ColorGreen
				dir02 = " ▼"
			} else if activeDepth == 1 {
				l13 = ColorGreen
				dir13 = "▼ "
			}
		}
	}

	fmt.Printf("                     ┌─────────┐\n")
	fmt.Printf("                     │  %sGPU 0%s  │ (Root)\n", ColorBold+ColorMagenta, ColorReset)
	fmt.Printf("                     └─────────┘\n")
	fmt.Printf("                      %s/       \\%s\n", l01, l02)
	fmt.Printf("                   %s%s         %s%s\n", l01, dir01, dir02, l02)
	fmt.Printf("                    %s/           \\%s\n", l01, l02)
	fmt.Printf("         ┌─────────┐             ┌─────────┐\n")
	fmt.Printf("         │  %sGPU 1%s  │             │  %sGPU 2%s  │ (Leaf)\n", 
		ColorBold+ColorMagenta, ColorReset, ColorBold+ColorMagenta, ColorReset)
	fmt.Printf("         └─────────┘             └─────────┘\n")
	fmt.Printf("          %s/%s\n", l13, ColorReset)
	fmt.Printf("       %s%s%s\n", l13, dir13, ColorReset)
	fmt.Printf("        %s/%s\n", l13, ColorReset)
	fmt.Printf("   ┌─────────┐\n")
	fmt.Printf("   │  %sGPU 3%s  │ (Leaf)\n", ColorBold+ColorMagenta, ColorReset)
	fmt.Printf("   └─────────┘\n")
}

// DrawBufferStates prints a table of current GPU buffer values.
func DrawBufferStates(state SimulationState) {
	fmt.Printf("%s[ BUFFER STATE TABLE ]%s\n", ColorBold+ColorCyan, ColorReset)
	
	// Print Header
	fmt.Printf("%s%-8s | %-12s | %-30s%s\n", ColorBold, "GPU ID", "STATUS", "BUFFER (BLOCKS 0-3)", ColorReset)
	fmt.Println(strings.Repeat("─", 70))

	for _, gpu := range state.GPUs {
		// Colorize status
		statusColor := ColorReset
		switch gpu.Status {
		case "Reduce-Scatter", "Sending", "Broadcasting":
			statusColor = ColorYellow
		case "All-Gather", "Receiving", "Reducing":
			statusColor = ColorGreen
		case "Done":
			statusColor = ColorCyan
		case "Idle":
			statusColor = ColorDim
		}

		// Print buffer with active/modified block highlighted
		var bufParts []string
		for idx, val := range gpu.Buffer {
			valStr := fmt.Sprintf("%5.1f", val)
			
			// Ring highlighting logic
			if state.Topology == TopologyRing && !state.Finished {
				activeIdx := gpu.ActiveBlockIdx
				if idx == activeIdx {
					if state.Step < state.NumGPUs-1 {
						// Reduce-Scatter: highlight the block we are sending (Yellow)
						bufParts = append(bufParts, ColorYellow+valStr+ColorReset)
					} else {
						// All-Gather: highlight the block we are updating/gathering (Green)
						bufParts = append(bufParts, ColorGreen+valStr+ColorReset)
					}
					continue
				}
			}

			// Tree highlighting logic: highlight blocks that changed from initial
			initVal := gpu.InitialBuffer[idx]
			if val != initVal {
				if state.Finished {
					bufParts = append(bufParts, ColorCyan+valStr+ColorReset)
				} else if state.Step >= state.MaxSteps/2 {
					// Broadcast Phase: fully reduced values
					bufParts = append(bufParts, ColorGreen+valStr+ColorReset)
				} else {
					// Reduce Phase: partially reduced values
					bufParts = append(bufParts, ColorYellow+valStr+ColorReset)
				}
			} else {
				bufParts = append(bufParts, valStr)
			}
		}
		
		bufStr := "[" + strings.Join(bufParts, ", ") + "]"

		fmt.Printf("GPU %-4d | %s%-12s%s | %s\n", 
			gpu.ID, statusColor, gpu.Status, ColorReset, bufStr)
	}
	fmt.Println(strings.Repeat("─", 70))
}

// DrawLogs prints the visualizer event console log.
func DrawLogs(state SimulationState) {
	fmt.Printf("%s[ EVENT CONSOLE LOG ]%s\n", ColorBold+ColorCyan, ColorReset)
	
	// Show last 5 logs
	logLen := len(state.Logs)
	startIdx := logLen - 5
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < logLen; i++ {
		log := state.Logs[i]
		color := ColorReset
		if log.IsWarning {
			color = ColorRed
		} else if strings.Contains(log.Message, "received") || strings.Contains(log.Message, "reduced") || strings.Contains(log.Message, "updated") {
			color = ColorGreen
		}
		
		fmt.Printf("%s[Step %d %s] %s%s\n", ColorGray, log.Step, log.Phase, color, log.Message)
	}
	
	// Fill blank lines to maintain stable UI height
	for i := logLen - startIdx; i < 5; i++ {
		fmt.Println()
	}
	fmt.Println(strings.Repeat("─", 70))
}

// DrawControls displays the available keyboard controls.
func DrawControls(state SimulationState) {
	if state.Finished {
		fmt.Printf("%sSimulation Finished Successfully!%s\n", ColorBold+ColorGreen, ColorReset)
		fmt.Printf("Press %s[r]%s to restart or %s[q]%s to exit.\n", ColorBold+ColorYellow, ColorReset, ColorBold+ColorRed, ColorReset)
	} else {
		fmt.Printf("Controls: %s[Space/Enter]%s Step | %s[a]%s Auto (%s) | %s[q]%s Quit\n", 
			ColorBold+ColorGreen, ColorReset, 
			ColorBold+ColorYellow, ColorReset, "500ms delay",
			ColorBold+ColorRed, ColorReset)
	}
}
