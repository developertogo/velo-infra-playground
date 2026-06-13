package main

import (
	"sync"
	"time"
)

// SystemBoot represents boot override parameters.
type SystemBoot struct {
	BootSourceOverrideTarget  string `json:"BootSourceOverrideTarget"`            // None, Pxe, Hdd, Cd, BiosSetup
	BootSourceOverrideEnabled string `json:"BootSourceOverrideEnabled"`           // Disabled, Once, Continuous
	BootSourceOverrideMode    string `json:"BootSourceOverrideMode,omitempty"`  // Legacy, UEFI
}

// RedfishStatus represents status field of resources.
type RedfishStatus struct {
	State  string `json:"State"`  // Enabled, Disabled, StandbyOffline
	Health string `json:"Health"` // OK, Warning, Critical
}

// RedfishSystem represents a mock bare-metal server.
type RedfishSystem struct {
	ID           string        `json:"Id"`
	Name         string        `json:"Name"`
	SystemType   string        `json:"SystemType"` // Physical
	Model        string        `json:"Model"`
	Manufacturer string        `json:"Manufacturer"`
	SerialNumber string        `json:"SerialNumber"`
	PowerState   string        `json:"PowerState"` // On, Off
	Status       RedfishStatus `json:"Status"`
	Boot         SystemBoot    `json:"Boot"`
}

// Fan represents a cooling fan.
type Fan struct {
	MemberID    string        `json:"MemberId"`
	Name        string        `json:"Name"`
	Reading     int           `json:"Reading"` // RPM
	ReadingUnits string       `json:"ReadingUnits"`
	Status      RedfishStatus `json:"Status"`
}

// Temperature represents a thermal sensor.
type Temperature struct {
	MemberID      string        `json:"MemberId"`
	Name          string        `json:"Name"`
	ReadingCelsius float64       `json:"ReadingCelsius"`
	UpperThresholdCritical float64 `json:"UpperThresholdCritical"`
	Status        RedfishStatus `json:"Status"`
}

// RedfishThermal represents the thermal resource of a chassis.
type RedfishThermal struct {
	ID           string        `json:"Id"`
	Name         string        `json:"Name"`
	Fans         []Fan         `json:"Fans"`
	Temperatures []Temperature `json:"Temperatures"`
}

// RedfishPower represents the power resource of a chassis.
type RedfishPower struct {
	ID           string        `json:"Id"`
	Name         string        `json:"Name"`
	PowerConsumedWatts int     `json:"PowerConsumedWatts"`
}

// HardwareStore tracks simulated systems.
type HardwareStore struct {
	mu      sync.Mutex
	Systems map[string]*RedfishSystem
}

// NewHardwareStore initializes server systems (e.g. DGX H100 node, DGX A100 node, generic node).
func NewHardwareStore() *HardwareStore {
	store := &HardwareStore{
		Systems: make(map[string]*RedfishSystem),
	}

	// 1. DGX H100 system
	store.Systems["system-1"] = &RedfishSystem{
		ID:           "system-1",
		Name:         "DGX-H100-Server-01",
		SystemType:   "Physical",
		Model:        "DGX H100",
		Manufacturer: "NVIDIA",
		SerialNumber: "NVSH100DGX0001",
		PowerState:   "On",
		Status: RedfishStatus{
			State:  "Enabled",
			Health: "OK",
		},
		Boot: SystemBoot{
			BootSourceOverrideTarget:  "None",
			BootSourceOverrideEnabled: "Disabled",
			BootSourceOverrideMode:    "UEFI",
		},
	}

	// 2. DGX A100 system
	store.Systems["system-2"] = &RedfishSystem{
		ID:           "system-2",
		Name:         "DGX-A100-Server-02",
		SystemType:   "Physical",
		Model:        "DGX A100",
		Manufacturer: "NVIDIA",
		SerialNumber: "NVSHA100DGX0002",
		PowerState:   "Off",
		Status: RedfishStatus{
			State:  "Enabled",
			Health: "OK",
		},
		Boot: SystemBoot{
			BootSourceOverrideTarget:  "None",
			BootSourceOverrideEnabled: "Disabled",
			BootSourceOverrideMode:    "UEFI",
		},
	}

	return store
}

// GetSystem returns a copy of a system.
func (s *HardwareStore) GetSystem(id string) (RedfishSystem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sys, ok := s.Systems[id]
	if !ok {
		return RedfishSystem{}, false
	}
	return *sys, true
}

// UpdateSystemBoot PATCHes system boot configuration.
func (s *HardwareStore) UpdateSystemBoot(id string, boot SystemBoot) (RedfishSystem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sys, ok := s.Systems[id]
	if !ok {
		return RedfishSystem{}, false
	}

	if boot.BootSourceOverrideTarget != "" {
		sys.Boot.BootSourceOverrideTarget = boot.BootSourceOverrideTarget
	}
	if boot.BootSourceOverrideEnabled != "" {
		sys.Boot.BootSourceOverrideEnabled = boot.BootSourceOverrideEnabled
	}
	if boot.BootSourceOverrideMode != "" {
		sys.Boot.BootSourceOverrideMode = boot.BootSourceOverrideMode
	}

	return *sys, true
}

// ResetSystem performs a chassis power action.
func (s *HardwareStore) ResetSystem(id string, resetType string) (RedfishSystem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sys, ok := s.Systems[id]
	if !ok {
		return RedfishSystem{}, false
	}

	switch resetType {
	case "On":
		sys.PowerState = "On"
		sys.Status.State = "Enabled"
	case "ForceOff", "GracefulShutdown":
		sys.PowerState = "Off"
		sys.Status.State = "StandbyOffline"
	case "ForceRestart", "GracefulRestart":
		sys.PowerState = "On"
		sys.Status.State = "Enabled"
		// If boot override was set to "Once", consume it back to Disabled on reset/reboot
		if sys.Boot.BootSourceOverrideEnabled == "Once" {
			sys.Boot.BootSourceOverrideEnabled = "Disabled"
		}
	}

	return *sys, true
}

// GetThermal returns dynamic thermal status (fans and temps depend on power state).
func (s *HardwareStore) GetThermal(systemID string) (RedfishThermal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sys, ok := s.Systems[systemID]
	if !ok {
		return RedfishThermal{}, false
	}

	thermal := RedfishThermal{
		ID:   "thermal-1",
		Name: "Chassis Thermal Status",
	}

	// Physics engine simulation
	if sys.PowerState == "On" {
		// Operational status
		thermal.Fans = []Fan{
			{MemberID: "fan-1", Name: "Front Fan 1", Reading: 5400, ReadingUnits: "RPM", Status: RedfishStatus{State: "Enabled", Health: "OK"}},
			{MemberID: "fan-2", Name: "Front Fan 2", Reading: 5350, ReadingUnits: "RPM", Status: RedfishStatus{State: "Enabled", Health: "OK"}},
			{MemberID: "fan-3", Name: "Rear Fan 1", Reading: 6100, ReadingUnits: "RPM", Status: RedfishStatus{State: "Enabled", Health: "OK"}},
		}

		cpuTemp := 42.5
		gpuTemp := 55.0
		if sys.Model == "DGX H100" {
			gpuTemp = 58.2
		}

		thermal.Temperatures = []Temperature{
			{MemberID: "temp-1", Name: "CPU 1 Temp", ReadingCelsius: cpuTemp, UpperThresholdCritical: 90.0, Status: RedfishStatus{State: "Enabled", Health: "OK"}},
			{MemberID: "temp-2", Name: "GPU Board Temp", ReadingCelsius: gpuTemp, UpperThresholdCritical: 85.0, Status: RedfishStatus{State: "Enabled", Health: "OK"}},
		}
	} else {
		// Powered off: fans stop and temperatures drop to room temperature
		thermal.Fans = []Fan{
			{MemberID: "fan-1", Name: "Front Fan 1", Reading: 0, ReadingUnits: "RPM", Status: RedfishStatus{State: "StandbyOffline", Health: "OK"}},
			{MemberID: "fan-2", Name: "Front Fan 2", Reading: 0, ReadingUnits: "RPM", Status: RedfishStatus{State: "StandbyOffline", Health: "OK"}},
			{MemberID: "fan-3", Name: "Rear Fan 1", Reading: 0, ReadingUnits: "RPM", Status: RedfishStatus{State: "StandbyOffline", Health: "OK"}},
		}
		thermal.Temperatures = []Temperature{
			{MemberID: "temp-1", Name: "CPU 1 Temp", ReadingCelsius: 22.0, UpperThresholdCritical: 90.0, Status: RedfishStatus{State: "StandbyOffline", Health: "OK"}},
			{MemberID: "temp-2", Name: "GPU Board Temp", ReadingCelsius: 21.5, UpperThresholdCritical: 85.0, Status: RedfishStatus{State: "StandbyOffline", Health: "OK"}},
		}
	}

	return thermal, true
}

// GetPower returns dynamic power consumption.
func (s *HardwareStore) GetPower(systemID string) (RedfishPower, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sys, ok := s.Systems[systemID]
	if !ok {
		return RedfishPower{}, false
	}

	watts := 0
	if sys.PowerState == "On" {
		watts = 350 // Idle base
		if sys.Model == "DGX H100" {
			watts = 700 // DGX H100 base
		}
		// Add some slight dynamic fluctuation based on current time
		watts += int(time.Now().Unix() % 15)
	} else {
		watts = 15 // Standby BMC power
	}

	return RedfishPower{
		ID:                 "power-1",
		Name:               "Chassis Power Control",
		PowerConsumedWatts: watts,
	}, true
}
