# Bare Metal (BMaaS) Redfish Simulator (`redfish-simulator`)

This is a mock implementation of the industry-standard DMTF Redfish REST API, representing out-of-band bare-metal hardware management. It enables testing bare-metal provisioning orchestrators (such as OpenStack Ironic or custom cluster managers) on Apple Silicon macOS.

---

## Features

- **Standard REST Endpoints**: Implements DMTF Redfish conformant schemas for Systems, Chassis, Power, and Thermal resources.
- **Boot Override Management**: Supports `PATCH` requests to override boot targets (e.g. setting boot target to `Pxe` for network booting), a critical component for bare-metal OS provisioning flows.
- **Power Actions**: Supports reset type overrides via `POST /redfish/v1/Systems/{SystemId}/Actions/System.Reset` (types like `On`, `ForceOff`, `GracefulShutdown`, `ForceRestart`, `GracefulRestart`).
- **Reactive Physics Engine**: 
  - When a server is **On**: Power consumption spikes to active levels (e.g. 700+ Watts for DGX H100), fan speeds spin at operational RPMs, and CPU/GPU temperatures settle at normal active levels.
  - When a server is **Off**: Power consumption drops to BMC standby levels (15 Watts), fan speeds drop to 0 RPM, and temperatures settle at room temperature (22°C).
- **Conformant Errors**: Returns structured JSON errors in standard Redfish base registry format.

---

## File Structure

- [`store.go`](store.go): Database tracking mock bare-metal server states (`system-1`/`system-2`) and modeling reactive physics for power, thermal sensors (temperatures), and cooling fan RPMs.
- [`handlers.go`](handlers.go): Conforms REST responses to OData specifications and implements route handlers for service root, systems collection, patching, resetting, and sensors.
- [`main.go`](main.go): Application entrypoint initiating data stores and serving HTTP connections.
- [`main_test.go`](main_test.go): Automated unit tests checking Service Root structure, boot overrides, power actions, and physics simulation.

---

## REST API Specification

| Method | Route | Description |
| :--- | :--- | :--- |
| **GET** | `/redfish/v1` | Redfish service root schema info |
| **GET** | `/redfish/v1/Systems` | List all available bare-metal systems |
| **GET** | `/redfish/v1/Systems/{SystemId}` | Query specific server properties (Model, Serial, Boot Override, PowerState) |
| **PATCH**| `/redfish/v1/Systems/{SystemId}` | Modify boot override target, mode, or frequency |
| **POST** | `/redfish/v1/Systems/{SystemId}/Actions/System.Reset` | Trigger power control actions (shutdown, boot, reboot) |
| **GET** | `/redfish/v1/Chassis` | List all available chasses |
| **GET** | `/redfish/v1/Chassis/{ChassisId}` | Query specific chassis link parameters |
| **GET** | `/redfish/v1/Chassis/{ChassisId}/Thermal` | Query current fan speeds (RPM) and temperatures (°C) |
| **GET** | `/redfish/v1/Chassis/{ChassisId}/Power` | Query current power draw (Watts) |

---

## Quick Start

### Build and Run

To run the simulator natively:

```bash
# Start the daemon on the default port 8000
go run ./redfish-simulator

# Configure a custom port
go run ./redfish-simulator -port 9000
```

### Running Tests

To run the automated tests:

```bash
go test -v ./...
```

---

## Usage Examples

Below are standard examples of querying the simulator using `curl`.

### 1. View System Details
```bash
curl http://localhost:8000/redfish/v1/Systems/system-1 | jq .
```

### 2. Override Boot Target to PXE
Set boot override to boot via PXE network boot once:

```bash
curl -X PATCH http://localhost:8000/redfish/v1/Systems/system-1 \
  -H "Content-Type: application/json" \
  -d '{
    "Boot": {
      "BootSourceOverrideTarget": "Pxe",
      "BootSourceOverrideEnabled": "Once",
      "BootSourceOverrideMode": "UEFI"
    }
  }'
```

### 3. Query Chassis Thermal Readings (ON State)
```bash
curl http://localhost:8000/redfish/v1/Chassis/chassis-1/Thermal | jq .
```

### 4. Power Off the Server
```bash
curl -i -X POST http://localhost:8000/redfish/v1/Systems/system-1/Actions/System.Reset \
  -H "Content-Type: application/json" \
  -d '{"ResetType": "ForceOff"}'
```
*(Returns `204 No Content` on success)*

### 5. Query Thermal Readings Again (OFF State)
Note that temperatures have dropped to room temperature (22°C) and fans are at 0 RPM:

```bash
curl http://localhost:8000/redfish/v1/Chassis/chassis-1/Thermal | jq .
```

### 6. Query Power Usage (OFF State)
Note power has dropped to 15 Watts (BMC standby mode):

```bash
curl http://localhost:8000/redfish/v1/Chassis/chassis-1/Power | jq .
```
