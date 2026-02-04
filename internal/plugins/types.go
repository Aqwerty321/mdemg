package plugins

import (
	"time"

	pb "mdemg/api/modulepb"
)

// ModuleState represents the current state of a module
type ModuleState string

const (
	StateUnknown     ModuleState = "unknown"
	StateStarting    ModuleState = "starting"
	StateReady       ModuleState = "ready"
	StateUnhealthy   ModuleState = "unhealthy"
	StateStopping    ModuleState = "stopping"
	StateStopped     ModuleState = "stopped"
	StateCrashed     ModuleState = "crashed"
)

// Manifest represents the module manifest (manifest.json)
type Manifest struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Version               string            `json:"version"`
	Type                  string            `json:"type"` // INGESTION, REASONING, APE
	Binary                string            `json:"binary"`
	Capabilities          Capabilities      `json:"capabilities"`
	HealthCheckIntervalMs int               `json:"health_check_interval_ms"`
	StartupTimeoutMs      int               `json:"startup_timeout_ms"`
	Config                map[string]string `json:"config,omitempty"`
	AdditionalServices    []string          `json:"additional_services,omitempty"` // e.g., ["CRUD"]
}

// Capabilities defines what a module can do
type Capabilities struct {
	IngestionSources []string `json:"ingestion_sources,omitempty"` // For INGESTION modules
	ContentTypes     []string `json:"content_types,omitempty"`     // File types this module can parse
	PatternDetectors []string `json:"pattern_detectors,omitempty"` // For REASONING modules
	EventTriggers    []string `json:"event_triggers,omitempty"`    // For APE modules
	CRUDEntityTypes  []string `json:"crud_entity_types,omitempty"` // For CRUD-capable modules
}

// ModuleInfo represents runtime information about a loaded module
type ModuleInfo struct {
	Manifest    Manifest
	State       ModuleState
	SocketPath  string
	PID         int
	StartedAt   time.Time
	LastHealthy time.Time
	LastError   string
	Metrics     map[string]string

	// gRPC clients (populated after handshake)
	LifecycleClient  pb.ModuleLifecycleClient
	IngestionClient  pb.IngestionModuleClient
	ReasoningClient  pb.ReasoningModuleClient
	APEClient        pb.APEModuleClient
	CRUDClient       pb.CRUDModuleClient
}

// ModuleStatus is the API response type for module status
type ModuleStatus struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Type         string            `json:"type"`
	State        string            `json:"state"`
	SocketPath   string            `json:"socket_path,omitempty"`
	PID          int               `json:"pid,omitempty"`
	StartedAt    string            `json:"started_at,omitempty"`
	LastHealthy  string            `json:"last_healthy,omitempty"`
	LastError    string            `json:"last_error,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Metrics      map[string]string `json:"metrics,omitempty"`
}

// ToModuleType converts string type to pb.ModuleType
func ToModuleType(t string) pb.ModuleType {
	switch t {
	case "INGESTION":
		return pb.ModuleType_MODULE_TYPE_INGESTION
	case "REASONING":
		return pb.ModuleType_MODULE_TYPE_REASONING
	case "APE":
		return pb.ModuleType_MODULE_TYPE_APE
	case "CRUD":
		return pb.ModuleType_MODULE_TYPE_CRUD
	default:
		return pb.ModuleType_MODULE_TYPE_UNSPECIFIED
	}
}

// FromModuleType converts pb.ModuleType to string
func FromModuleType(t pb.ModuleType) string {
	switch t {
	case pb.ModuleType_MODULE_TYPE_INGESTION:
		return "INGESTION"
	case pb.ModuleType_MODULE_TYPE_REASONING:
		return "REASONING"
	case pb.ModuleType_MODULE_TYPE_APE:
		return "APE"
	case pb.ModuleType_MODULE_TYPE_CRUD:
		return "CRUD"
	default:
		return "UNSPECIFIED"
	}
}
