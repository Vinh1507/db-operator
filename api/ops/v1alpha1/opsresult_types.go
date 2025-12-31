package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpsResultSpec defines the desired state of OpsResult
type OpsResultSpec struct {
	// ClusterRef references the DatabaseCluster
	ClusterRef ObjectReference `json:"clusterRef"`

	// EngineRef references the underlying engine (e.g., CNPG Cluster)
	EngineRef ObjectReference `json:"engineRef"`

	// Logs contains collected information from cluster pods
	// Named "Logs" for extensibility to include logs, events, metrics, etc.
	Logs map[string]string `json:"logs,omitempty"`
}

// ObjectReference contains enough information to identify a resource
type ObjectReference struct {
	// Name of the referenced object
	Name string `json:"name"`

	// Namespace of the referenced object
	Namespace string `json:"namespace,omitempty"`

	// Kind of the referenced object
	Kind string `json:"kind,omitempty"`

	// APIVersion of the referenced object
	APIVersion string `json:"apiVersion,omitempty"`
}

// OpsResultStatus defines the observed state of OpsResult
type OpsResultStatus struct {
	// Phase represents the current phase of the OpsResult
	// +optional
	Phase string `json:"phase,omitempty"`

	// LastUpdateTime is the last time the logs were updated
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// TotalPods is the total number of pods logs collected from
	// +optional
	TotalPods int `json:"totalPods,omitempty"`

	// Conditions represent the latest available observations of the OpsResult's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=opsr
//+kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef.name`
//+kubebuilder:printcolumn:name="Pods",type=integer,JSONPath=`.status.totalPods`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OpsResult is the Schema for the opsresults API
type OpsResult struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpsResultSpec   `json:"spec,omitempty"`
	Status OpsResultStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OpsResultList contains a list of OpsResult
type OpsResultList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpsResult `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpsResult{}, &OpsResultList{})
}
