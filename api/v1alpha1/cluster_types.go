package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EngineCategory string
type EngineType string

const (
	CategoryDatabase EngineCategory = "database"
	CategoryProxy    EngineCategory = "proxy"

	EnginePXC      EngineType = "pxc"
	EngineCNPG     EngineType = "cnpg"
	EngineProxySQL EngineType = "proxysql"
)

// ClusterPhase represents the phase of the cluster
type ClusterPhase string

const (
	PhaseInitializing ClusterPhase = "Initializing"
	PhaseReady        ClusterPhase = "Ready"
	PhaseUpgrading    ClusterPhase = "Upgrading"
	PhaseFailed       ClusterPhase = "Failed"
	PhaseDeleting     ClusterPhase = "Deleting"
	PhasePaused       ClusterPhase = "Paused"
)

// EnginePhase represents the phase of an engine
type EnginePhase string

const (
	EnginePhaseInitializing EnginePhase = "Initializing"
	EnginePhaseReady        EnginePhase = "Ready"
	EnginePhaseUpgrading    EnginePhase = "Upgrading"
	EnginePhaseRestoring    EnginePhase = "Restoring"
	EnginePhaseFailed       EnginePhase = "Failed"
	EnginePhasePaused       EnginePhase = "Paused"
)

type StorageSpec struct {
	Enabled bool   `json:"enabled"`
	Class   string `json:"class,omitempty"`
	Size    string `json:"size,omitempty"`
}

type ResourceSpec struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type EngineMonitoring struct {
	Enabled              bool   `json:"enabled"`
	MonitoringConfigName string `json:"monitoringConfigName,omitempty"`
	ExporterPort         int32  `json:"exporterPort,omitempty"`
}

type EngineBackupSpec struct {
	Enabled           bool                 `json:"enabled"`
	BackupStorageName string               `json:"backupStorageName,omitempty"`
	Schedules         []BackupScheduleSpec `json:"schedules,omitempty"`
	BackupStorages    []BackupStorage      `json:"backupStorages,omitempty"`
}

// BackupStorage references and configures a backup storage location
type BackupStorage struct {
	// Name of the backup storage (reference to BackupStorage CR or logical name)
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// secret ref
	// +optional
	SecretRef string `json:"secretRef,omitempty"`
}
type BackupScheduleSpec struct {
	Name              string `json:"name"`
	Schedule          string `json:"schedule"` // cron
	Enabled           bool   `json:"enabled"`
	RetentionCopies   int32  `json:"retentionCopies,omitempty"`
	BackupType        string `json:"backupType,omitempty"`
	BackupStorageName string `json:"backupStorageName,omitempty"`
}

type EngineExposeSpec struct {
	Enabled bool       `json:"enabled"`
	Type    ExposeType `json:"type,omitempty"`
	Port    int32      `json:"port,omitempty"`
}

type ExposeType string

const (
	ExposeTypeHeadless     ExposeType = "Headless"
	ExposeTypeClusterIP    ExposeType = "ClusterIP"
	ExposeTypeNodePort     ExposeType = "NodePort"
	ExposeTypeLoadBalancer ExposeType = "LoadBalancer"
)

type EngineSpec struct {
	Name       string         `json:"name"`
	Category   EngineCategory `json:"category"`
	EngineType EngineType     `json:"engineType"`
	Version    string         `json:"version"`
	Replicas   int32          `json:"replicas"`
	Priority   int32          `json:"priority"`

	Storage   StorageSpec  `json:"storage,omitempty"`
	Resources ResourceSpec `json:"resources,omitempty"`

	Config ConfigSpec `json:"config,omitempty"`

	CredentialsSecretNames []string         `json:"credentialsSecretNames,omitempty"`
	Monitoring             EngineMonitoring `json:"monitoring,omitempty"`

	Backup EngineBackupSpec `json:"backup,omitempty"`
	Expose EngineExposeSpec `json:"expose,omitempty"`
}

type ConfigSpec struct {
	Raw string            `json:"raw,omitempty"`
	KV  map[string]string `json:"kv,omitempty"`
}

// ClusterSpec defines the desired state
type ClusterSpec struct {
	Engines []EngineSpec `json:"engines"`
	Paused  bool         `json:"paused,omitempty"`
}

type EngineEndpoints struct {
	// Internal service endpoints (in-cluster)
	// +optional
	Internal []string `json:"internal,omitempty"`

	// External endpoints (LB / public)
	// +optional
	External []string `json:"external,omitempty"`

	// Admin / management endpoints
	// +optional
	Admin []string `json:"admin,omitempty"`
}

// EngineStatus represents the status of a single engine
type EngineStatus struct {
	// Name of the engine (matches EngineSpec.Name)
	Name string `json:"name"`

	// Type of the engine (cnpg, pxc, proxysql, etc.)
	Type string `json:"type"`

	// Phase of the engine (Ready, Initializing, Failed, etc.)
	Phase EnginePhase `json:"phase"`

	// Number of ready instances
	Ready int32 `json:"ready"`

	// Total number of desired instances
	Total int32 `json:"total"`

	// Current version running
	Version string `json:"version,omitempty"`

	// Hostname or endpoint for connection
	Hostname string `json:"hostname,omitempty"`

	// Port for connection
	Port int32 `json:"port,omitempty"`

	// Human-readable message about current state
	Message string `json:"message,omitempty"`

	// Last time this status was updated
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// ClusterStatus defines the observed state
type ClusterStatus struct {
	// Phase represents the overall cluster state
	// +kubebuilder:validation:Enum=Initializing;Ready;Upgrading;Failed;Deleting;Paused
	Phase ClusterPhase `json:"phase,omitempty"`

	// Reason provides more context about the current phase
	Reason string `json:"reason,omitempty"`

	// Message provides human-readable information
	Message string `json:"message,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed Cluster
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Engines contains status for each engine in the cluster
	Engines []EngineStatus `json:"engines,omitempty"`

	// Conditions represent the latest available observations of the cluster's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Connection endpoints for client
	// +optional
	Endpoints *EngineEndpoints `json:"endpoints,omitempty"`

	// LastUpdateTime is the last time the status was updated
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Engines",type=integer,JSONPath=`.status.engines[*].name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
