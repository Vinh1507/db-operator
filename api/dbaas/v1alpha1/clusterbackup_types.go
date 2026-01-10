package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupState represents the state of a backup
type BackupState string

const (
	BackupStateNew       BackupState = ""
	BackupStateStarting  BackupState = "Starting"
	BackupStateRunning   BackupState = "Running"
	BackupStateFailed    BackupState = "Failed"
	BackupStateSucceeded BackupState = "Succeeded"
	BackupStateDeleting  BackupState = "Deleting"
)

// ClusterBackupSpec defines the desired state of ClusterBackup
type ClusterBackupSpec struct {
	// ClusterName is the name of the Cluster to backup
	ClusterName string `json:"clusterName"`

	// EngineName is the name of the engine instance to backup
	EngineName string `json:"engineName"`
}

// ClusterBackupStatus defines the observed state of ClusterBackup
type ClusterBackupStatus struct {
	// State is the current state of the backup
	State BackupState `json:"state,omitempty"`

	// CreatedAt is when the backup was created
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// CompletedAt is when the backup completed
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Destination is the backup location
	Destination string `json:"destination,omitempty"`

	// Message provides additional information
	Message string `json:"message,omitempty"`

	// InUse indicates if backup is being used for restore
	// +kubebuilder:default=false
	InUse bool `json:"inUse,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cbk
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="Engine",type="string",JSONPath=".spec.engineName"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Destination",type="string",JSONPath=".status.destination"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterBackup is the Schema for the clusterbackups API
type ClusterBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterBackupSpec   `json:"spec,omitempty"`
	Status ClusterBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterBackupList contains a list of ClusterBackup
type ClusterBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterBackup{}, &ClusterBackupList{})
}

// IsCompleted returns true if backup has finished
func (b *ClusterBackup) IsCompleted() bool {
	return b.Status.State == BackupStateSucceeded ||
		b.Status.State == BackupStateFailed
}

// IsSucceeded returns true if backup succeeded
func (b *ClusterBackup) IsSucceeded() bool {
	return b.Status.State == BackupStateSucceeded
}

// IsFailed returns true if backup failed
func (b *ClusterBackup) IsFailed() bool {
	return b.Status.State == BackupStateFailed
}

// IsInProgress returns true if backup is running
func (b *ClusterBackup) IsInProgress() bool {
	return b.Status.State == BackupStateNew ||
		b.Status.State == BackupStateStarting ||
		b.Status.State == BackupStateRunning
}
