package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SentinelDeploymentSpec defines the desired state of SentinelDeployment
type SentinelDeploymentSpec struct {
	// GatewayAddress is the endpoint address of the Sentinel gateway server (e.g. velo-sentinel:9000)
	GatewayAddress string `json:"gatewayAddress"`

	// MinReplicas is the minimum number of worker replicas (default 1)
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum number of worker replicas (default 10)
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// ScaleThreshold is the queue depth per replica at which scaling starts (default 5 requests)
	ScaleThreshold int32 `json:"scaleThreshold,omitempty"`

	// TargetDeployment is the name of the Kubernetes Deployment to scale
	TargetDeployment string `json:"targetDeployment"`
}

// SentinelDeploymentStatus defines the observed state of SentinelDeployment
type SentinelDeploymentStatus struct {
	// Replicas is the current replica count of the worker deployment
	Replicas int32 `json:"replicas,omitempty"`

	// ActiveQueueDepth is the last observed request queue depth of the gateway
	ActiveQueueDepth int32 `json:"activeQueueDepth,omitempty"`

	// LastScaleTime represents the last time a scaling operation was triggered
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SentinelDeployment is the Schema for the sentineldeployments API
type SentinelDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SentinelDeploymentSpec   `json:"spec,omitempty"`
	Status SentinelDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SentinelDeploymentList contains a list of SentinelDeployment
type SentinelDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SentinelDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SentinelDeployment{}, &SentinelDeploymentList{})
}
