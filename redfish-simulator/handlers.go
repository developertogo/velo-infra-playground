package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// RedfishErrorResponse conforms to DMTF Redfish error JSON structure.
type RedfishErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// RedfishHandler handles HTTP mapping.
type RedfishHandler struct {
	store *HardwareStore
}

// NewRedfishHandler initializes a handler with store.
func NewRedfishHandler(store *HardwareStore) *RedfishHandler {
	return &RedfishHandler{store: store}
}

// writeJSON writes JSON payload with Redfish content-type.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("OData-Version", "4.0")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a standard Redfish error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	var resp RedfishErrorResponse
	resp.Error.Code = code
	resp.Error.Message = message
	writeJSON(w, status, resp)
}

// RegisterRoutes sets up native Go mux path mappings.
func (h *RedfishHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /redfish/v1", h.GetServiceRoot)
	mux.HandleFunc("GET /redfish/v1/", h.GetServiceRoot)
	mux.HandleFunc("GET /redfish/v1/Systems", h.GetSystems)
	mux.HandleFunc("GET /redfish/v1/Systems/", h.GetSystems)
	mux.HandleFunc("GET /redfish/v1/Systems/{SystemId}", h.GetSystem)
	mux.HandleFunc("PATCH /redfish/v1/Systems/{SystemId}", h.PatchSystem)
	mux.HandleFunc("POST /redfish/v1/Systems/{SystemId}/Actions/System.Reset", h.ResetSystem)
	
	mux.HandleFunc("GET /redfish/v1/Chassis", h.GetChassisCollection)
	mux.HandleFunc("GET /redfish/v1/Chassis/{ChassisId}", h.GetChassis)
	mux.HandleFunc("GET /redfish/v1/Chassis/{ChassisId}/Thermal", h.GetThermal)
	mux.HandleFunc("GET /redfish/v1/Chassis/{ChassisId}/Power", h.GetPower)
}

// GetServiceRoot implements GET /redfish/v1
func (h *RedfishHandler) GetServiceRoot(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"@odata.context": "/redfish/v1/$metadata#ServiceRoot.ServiceRoot",
		"@odata.id":      "/redfish/v1",
		"@odata.type":    "#ServiceRoot.v1_11_1.ServiceRoot",
		"Id":             "RootService",
		"Name":           "Redfish Service Root",
		"RedfishVersion": "1.11.1",
		"UUID":           "92aa4e8e-d9ad-4cf5-992a-b089cc5b1d47",
		"Systems": map[string]string{
			"@odata.id": "/redfish/v1/Systems",
		},
		"Chassis": map[string]string{
			"@odata.id": "/redfish/v1/Chassis",
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetSystems implements GET /redfish/v1/Systems
func (h *RedfishHandler) GetSystems(w http.ResponseWriter, r *http.Request) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	members := make([]map[string]string, 0, len(h.store.Systems))
	for id := range h.store.Systems {
		members = append(members, map[string]string{
			"@odata.id": fmt.Sprintf("/redfish/v1/Systems/%s", id),
		})
	}

	resp := map[string]interface{}{
		"@odata.context":      "/redfish/v1/$metadata#ComputerSystemCollection.ComputerSystemCollection",
		"@odata.id":           "/redfish/v1/Systems",
		"@odata.type":         "#ComputerSystemCollection.ComputerSystemCollection",
		"Name":                "Computer System Collection",
		"Members@odata.count": len(members),
		"Members":             members,
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetSystem implements GET /redfish/v1/Systems/{SystemId}
func (h *RedfishHandler) GetSystem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("SystemId")
	sys, ok := h.store.GetSystem(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound", fmt.Sprintf("System %s not found", id))
		return
	}

	resp := map[string]interface{}{
		"@odata.context": fmt.Sprintf("/redfish/v1/$metadata#ComputerSystem.ComputerSystem"),
		"@odata.id":      fmt.Sprintf("/redfish/v1/Systems/%s", id),
		"@odata.type":    "#ComputerSystem.v1_15_0.ComputerSystem",
		"Id":             sys.ID,
		"Name":           sys.Name,
		"SystemType":     sys.SystemType,
		"Model":          sys.Model,
		"Manufacturer":   sys.Manufacturer,
		"SerialNumber":   sys.SerialNumber,
		"PowerState":     sys.PowerState,
		"Status":         sys.Status,
		"Boot":           sys.Boot,
		"Actions": map[string]interface{}{
			"#System.Reset": map[string]interface{}{
				"target": fmt.Sprintf("/redfish/v1/Systems/%s/Actions/System.Reset", id),
				"ResetType@Redfish.AllowableValues": []string{
					"On",
					"ForceOff",
					"GracefulShutdown",
					"GracefulRestart",
					"ForceRestart",
				},
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// PatchSystem implements PATCH /redfish/v1/Systems/{SystemId}
func (h *RedfishHandler) PatchSystem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("SystemId")
	
	// Read input body
	var req struct {
		Boot *SystemBoot `json:"Boot"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.MalformedJSON", "Malformed request body")
		return
	}

	if req.Boot == nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.PropertyMissing", "Patch request must specify fields, e.g. 'Boot'")
		return
	}

	sys, ok := h.store.UpdateSystemBoot(id, *req.Boot)
	if !ok {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound", fmt.Sprintf("System %s not found", id))
		return
	}

	writeJSON(w, http.StatusOK, sys)
}

// ResetSystem implements POST /redfish/v1/Systems/{SystemId}/Actions/System.Reset
func (h *RedfishHandler) ResetSystem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("SystemId")
	
	var req struct {
		ResetType string `json:"ResetType"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.MalformedJSON", "Malformed request body")
		return
	}

	validTypes := map[string]bool{
		"On":               true,
		"ForceOff":         true,
		"GracefulShutdown": true,
		"GracefulRestart":  true,
		"ForceRestart":     true,
	}

	if !validTypes[req.ResetType] {
		writeError(w, http.StatusBadRequest, "Base.1.0.PropertyValueNotInList", 
			fmt.Sprintf("ResetType '%s' not supported. Use: On, ForceOff, GracefulShutdown, GracefulRestart, ForceRestart", req.ResetType))
		return
	}

	sys, ok := h.store.ResetSystem(id, req.ResetType)
	if !ok {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound", fmt.Sprintf("System %s not found", id))
		return
	}

	fmt.Printf("[Redfish Simulator] System %s reset action triggered: %s. PowerState is now %s\n", 
		id, req.ResetType, sys.PowerState)
	
	// Redfish Reset Action returns 204 No Content on success
	w.WriteHeader(http.StatusNoContent)
}

// GetChassisCollection implements GET /redfish/v1/Chassis
func (h *RedfishHandler) GetChassisCollection(w http.ResponseWriter, r *http.Request) {
	// Map each system to a chassis
	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	members := make([]map[string]string, 0, len(h.store.Systems))
	for id := range h.store.Systems {
		chassisID := strings.Replace(id, "system", "chassis", 1)
		members = append(members, map[string]string{
			"@odata.id": fmt.Sprintf("/redfish/v1/Chassis/%s", chassisID),
		})
	}

	resp := map[string]interface{}{
		"@odata.context":      "/redfish/v1/$metadata#ChassisCollection.ChassisCollection",
		"@odata.id":           "/redfish/v1/Chassis",
		"@odata.type":         "#ChassisCollection.ChassisCollection",
		"Name":                "Chassis Collection",
		"Members@odata.count": len(members),
		"Members":             members,
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetChassis implements GET /redfish/v1/Chassis/{ChassisId}
func (h *RedfishHandler) GetChassis(w http.ResponseWriter, r *http.Request) {
	chassisID := r.PathValue("ChassisId")
	systemID := strings.Replace(chassisID, "chassis", "system", 1)

	sys, ok := h.store.GetSystem(systemID)
	if !ok {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound", fmt.Sprintf("Chassis %s not found", chassisID))
		return
	}

	resp := map[string]interface{}{
		"@odata.context": "/redfish/v1/$metadata#Chassis.Chassis",
		"@odata.id":      fmt.Sprintf("/redfish/v1/Chassis/%s", chassisID),
		"@odata.type":    "#Chassis.v1_14_0.Chassis",
		"Id":             chassisID,
		"Name":           "Main Chassis Layout",
		"ChassisType":    "RackMount",
		"Model":          sys.Model,
		"Manufacturer":   sys.Manufacturer,
		"SerialNumber":   sys.SerialNumber,
		"Status":         sys.Status,
		"Thermal": map[string]string{
			"@odata.id": fmt.Sprintf("/redfish/v1/Chassis/%s/Thermal", chassisID),
		},
		"Power": map[string]string{
			"@odata.id": fmt.Sprintf("/redfish/v1/Chassis/%s/Power", chassisID),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetThermal implements GET /redfish/v1/Chassis/{ChassisId}/Thermal
func (h *RedfishHandler) GetThermal(w http.ResponseWriter, r *http.Request) {
	chassisID := r.PathValue("ChassisId")
	systemID := strings.Replace(chassisID, "chassis", "system", 1)

	thermal, ok := h.store.GetThermal(systemID)
	if !ok {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound", fmt.Sprintf("Chassis thermal sensors for %s not found", chassisID))
		return
	}

	resp := map[string]interface{}{
		"@odata.context": fmt.Sprintf("/redfish/v1/$metadata#Thermal.Thermal"),
		"@odata.id":      fmt.Sprintf("/redfish/v1/Chassis/%s/Thermal", chassisID),
		"@odata.type":    "#Thermal.v1_6_0.Thermal",
		"Id":             thermal.ID,
		"Name":           thermal.Name,
		"Fans":           thermal.Fans,
		"Temperatures":   thermal.Temperatures,
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetPower implements GET /redfish/v1/Chassis/{ChassisId}/Power
func (h *RedfishHandler) GetPower(w http.ResponseWriter, r *http.Request) {
	chassisID := r.PathValue("ChassisId")
	systemID := strings.Replace(chassisID, "chassis", "system", 1)

	power, ok := h.store.GetPower(systemID)
	if !ok {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceNotFound", fmt.Sprintf("Chassis power control for %s not found", chassisID))
		return
	}

	resp := map[string]interface{}{
		"@odata.context": fmt.Sprintf("/redfish/v1/$metadata#Power.Power"),
		"@odata.id":      fmt.Sprintf("/redfish/v1/Chassis/%s/Power", chassisID),
		"@odata.type":    "#Power.v1_6_0.Power",
		"Id":             power.ID,
		"Name":           power.Name,
		"PowerControl": []map[string]interface{}{
			{
				"MemberId":           "power-ctrl-1",
				"Name":               "Main Control Loop",
				"PowerConsumedWatts": power.PowerConsumedWatts,
				"Status": map[string]string{
					"State":  "Enabled",
					"Health": "OK",
				},
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}
