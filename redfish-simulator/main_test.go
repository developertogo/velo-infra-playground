package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceRootAndCollections(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Get Service Root
	req, _ := http.NewRequest("GET", "/redfish/v1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 for root, got %d", rr.Code)
	}

	var rootMap map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &rootMap)
	if rootMap["RedfishVersion"] != "1.11.1" {
		t.Errorf("Expected version 1.11.1, got %v", rootMap["RedfishVersion"])
	}

	// 2. Get Systems Collection
	reqSys, _ := http.NewRequest("GET", "/redfish/v1/Systems", nil)
	rrSys := httptest.NewRecorder()
	mux.ServeHTTP(rrSys, reqSys)

	if rrSys.Code != http.StatusOK {
		t.Errorf("Expected status 200 for systems, got %d", rrSys.Code)
	}

	var sysColl map[string]interface{}
	json.Unmarshal(rrSys.Body.Bytes(), &sysColl)
	if sysColl["Members@odata.count"].(float64) != 2 {
		t.Errorf("Expected 2 members, got %v", sysColl["Members@odata.count"])
	}

	// 3. Get Chassis Collection
	reqChas, _ := http.NewRequest("GET", "/redfish/v1/Chassis", nil)
	rrChas := httptest.NewRecorder()
	mux.ServeHTTP(rrChas, reqChas)

	if rrChas.Code != http.StatusOK {
		t.Errorf("Expected status 200 for chassis, got %d", rrChas.Code)
	}
}

func TestGetSystemInvalid(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req, _ := http.NewRequest("GET", "/redfish/v1/Systems/system-invalid", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rr.Code)
	}

	var errResp RedfishErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &errResp)
	if errResp.Error.Code != "Base.1.0.ResourceNotFound" {
		t.Errorf("Expected ResourceNotFound error code, got %s", errResp.Error.Code)
	}
}

func TestBootOverridePatching(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// PATCH boot target to PXE once UEFI
	patchData := []byte(`{"Boot": {"BootSourceOverrideTarget": "Pxe", "BootSourceOverrideEnabled": "Once", "BootSourceOverrideMode": "UEFI"}}`)
	req, _ := http.NewRequest("PATCH", "/redfish/v1/Systems/system-1", bytes.NewReader(patchData))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for PATCH, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// Verify update in store
	sys, _ := store.GetSystem("system-1")
	if sys.Boot.BootSourceOverrideTarget != "Pxe" || sys.Boot.BootSourceOverrideEnabled != "Once" {
		t.Errorf("Expected BootOverride to be Pxe/Once, got Target=%s, Enabled=%s", 
			sys.Boot.BootSourceOverrideTarget, sys.Boot.BootSourceOverrideEnabled)
	}
}

func TestPowerStatePhysicsSimulation(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// --- 1. System-1 starts ON ---
	sys, _ := store.GetSystem("system-1")
	if sys.PowerState != "On" {
		t.Fatalf("Expected initial PowerState to be On, got %s", sys.PowerState)
	}

	// Verify thermal readings for ON state (Fans should be spinning, CPU temp > room temp)
	reqTherm, _ := http.NewRequest("GET", "/redfish/v1/Chassis/chassis-1/Thermal", nil)
	rrTherm := httptest.NewRecorder()
	mux.ServeHTTP(rrTherm, reqTherm)
	if rrTherm.Code != http.StatusOK {
		t.Fatalf("Failed to query thermal: %d", rrTherm.Code)
	}
	var thermalOn RedfishThermal
	json.Unmarshal(rrTherm.Body.Bytes(), &thermalOn)
	if len(thermalOn.Fans) == 0 || thermalOn.Fans[0].Reading <= 0 {
		t.Errorf("Expected active fans (>0 RPM) when powered on, got fan readings: %v", thermalOn.Fans)
	}
	if thermalOn.Temperatures[0].ReadingCelsius <= 25.0 {
		t.Errorf("Expected warm CPU (>25 C) when powered on, got: %f", thermalOn.Temperatures[0].ReadingCelsius)
	}

	// Verify power consumption is high (e.g. 700+ Watts for DGX H100)
	reqPower, _ := http.NewRequest("GET", "/redfish/v1/Chassis/chassis-1/Power", nil)
	rrPower := httptest.NewRecorder()
	mux.ServeHTTP(rrPower, reqPower)
	// Helper decoding since API wraps it in PowerControl list
	var powerCtrlResp struct {
		PowerControl []struct {
			PowerConsumedWatts int `json:"PowerConsumedWatts"`
		} `json:"PowerControl"`
	}
	json.Unmarshal(rrPower.Body.Bytes(), &powerCtrlResp)
	if powerCtrlResp.PowerControl[0].PowerConsumedWatts < 700 {
		t.Errorf("Expected high power consumption for active DGX H100, got: %d Watts", 
			powerCtrlResp.PowerControl[0].PowerConsumedWatts)
	}

	// --- 2. Trigger Shutdown (ForceOff) ---
	resetData := []byte(`{"ResetType": "ForceOff"}`)
	reqReset, _ := http.NewRequest("POST", "/redfish/v1/Systems/system-1/Actions/System.Reset", bytes.NewReader(resetData))
	rrReset := httptest.NewRecorder()
	mux.ServeHTTP(rrReset, reqReset)

	if rrReset.Code != http.StatusNoContent {
		t.Fatalf("Expected 204 No Content for reset action, got %d", rrReset.Code)
	}

	// Check PowerState changed to Off
	sysOff, _ := store.GetSystem("system-1")
	if sysOff.PowerState != "Off" {
		t.Errorf("Expected PowerState to transition to Off, got %s", sysOff.PowerState)
	}

	// --- 3. Verify physics in OFF state ---
	// Fans must be 0 RPM and temperatures must settle back to room temperature (e.g. 22 C)
	rrThermOff := httptest.NewRecorder()
	mux.ServeHTTP(rrThermOff, reqTherm) // reuse GET request
	var thermalOff RedfishThermal
	json.Unmarshal(rrThermOff.Body.Bytes(), &thermalOff)
	for _, fan := range thermalOff.Fans {
		if fan.Reading != 0 {
			t.Errorf("Expected fan speed to be 0 RPM when powered off, got %d for %s", fan.Reading, fan.Name)
		}
	}
	if thermalOff.Temperatures[0].ReadingCelsius != 22.0 {
		t.Errorf("Expected CPU temp to drop to room temp (22 C) when powered off, got %f", 
			thermalOff.Temperatures[0].ReadingCelsius)
	}

	// Power consumption should drop to standby levels (e.g. 15 Watts)
	rrPowerOff := httptest.NewRecorder()
	mux.ServeHTTP(rrPowerOff, reqPower) // reuse GET request
	var powerCtrlRespOff struct {
		PowerControl []struct {
			PowerConsumedWatts int `json:"PowerConsumedWatts"`
		} `json:"PowerControl"`
	}
	json.Unmarshal(rrPowerOff.Body.Bytes(), &powerCtrlRespOff)
	if powerCtrlRespOff.PowerControl[0].PowerConsumedWatts != 15 {
		t.Errorf("Expected low standby power consumption (15 W) when powered off, got: %d Watts", 
			powerCtrlRespOff.PowerControl[0].PowerConsumedWatts)
	}
}

func TestResetInvalidResetType(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	resetData := []byte(`{"ResetType": "InvalidAction"}`)
	req, _ := http.NewRequest("POST", "/redfish/v1/Systems/system-1/Actions/System.Reset", bytes.NewReader(resetData))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rr.Code)
	}

	var errResp RedfishErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &errResp)
	if errResp.Error.Code != "Base.1.0.PropertyValueNotInList" {
		t.Errorf("Expected PropertyValueNotInList error code, got %s", errResp.Error.Code)
	}
}

func TestPatchInvalidRequests(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. PATCH with malformed JSON
	reqBadJSON, _ := http.NewRequest("PATCH", "/redfish/v1/Systems/system-1", bytes.NewReader([]byte("{bad json")))
	rrBadJSON := httptest.NewRecorder()
	mux.ServeHTTP(rrBadJSON, reqBadJSON)
	if rrBadJSON.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for bad JSON, got %d", rrBadJSON.Code)
	}

	// 2. PATCH with missing Boot object
	reqMissingProp, _ := http.NewRequest("PATCH", "/redfish/v1/Systems/system-1", bytes.NewReader([]byte("{}")))
	rrMissingProp := httptest.NewRecorder()
	mux.ServeHTTP(rrMissingProp, reqMissingProp)
	if rrMissingProp.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty PATCH, got %d", rrMissingProp.Code)
	}
}

func TestResetConsumesOnceBootTarget(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Setup Boot Override to PXE Once
	patchData := []byte(`{"Boot": {"BootSourceOverrideTarget": "Pxe", "BootSourceOverrideEnabled": "Once", "BootSourceOverrideMode": "UEFI"}}`)
	reqPatch, _ := http.NewRequest("PATCH", "/redfish/v1/Systems/system-1", bytes.NewReader(patchData))
	rrPatch := httptest.NewRecorder()
	mux.ServeHTTP(rrPatch, reqPatch)
	if rrPatch.Code != http.StatusOK {
		t.Fatalf("Failed to PATCH boot override")
	}

	// 2. Perform a GracefulRestart reset action
	resetData := []byte(`{"ResetType": "GracefulRestart"}`)
	reqReset, _ := http.NewRequest("POST", "/redfish/v1/Systems/system-1/Actions/System.Reset", bytes.NewReader(resetData))
	rrReset := httptest.NewRecorder()
	mux.ServeHTTP(rrReset, reqReset)
	if rrReset.Code != http.StatusNoContent {
		t.Fatalf("Failed to execute Reset action: %d", rrReset.Code)
	}

	// 3. Query system and verify BootSourceOverrideEnabled has reverted to "Disabled"
	reqGet, _ := http.NewRequest("GET", "/redfish/v1/Systems/system-1", nil)
	rrGet := httptest.NewRecorder()
	mux.ServeHTTP(rrGet, reqGet)
	var sys RedfishSystem
	json.Unmarshal(rrGet.Body.Bytes(), &sys)
	if sys.Boot.BootSourceOverrideEnabled != "Disabled" {
		t.Errorf("Expected boot override enabled to be 'Disabled' after reboot, got %s", 
			sys.Boot.BootSourceOverrideEnabled)
	}
}

func TestChassisInvalidEndpoints(t *testing.T) {
	store := NewHardwareStore()
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. GET non-existent Chassis
	reqChassis, _ := http.NewRequest("GET", "/redfish/v1/Chassis/chassis-invalid", nil)
	rrChassis := httptest.NewRecorder()
	mux.ServeHTTP(rrChassis, reqChassis)
	if rrChassis.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for invalid chassis, got %d", rrChassis.Code)
	}

	// 2. GET thermal for non-existent Chassis
	reqThermal, _ := http.NewRequest("GET", "/redfish/v1/Chassis/chassis-invalid/Thermal", nil)
	rrThermal := httptest.NewRecorder()
	mux.ServeHTTP(rrThermal, reqThermal)
	if rrThermal.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for invalid chassis thermal, got %d", rrThermal.Code)
	}

	// 3. GET power for non-existent Chassis
	reqPower, _ := http.NewRequest("GET", "/redfish/v1/Chassis/chassis-invalid/Power", nil)
	rrPower := httptest.NewRecorder()
	mux.ServeHTTP(rrPower, reqPower)
	if rrPower.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for invalid chassis power, got %d", rrPower.Code)
	}
}
