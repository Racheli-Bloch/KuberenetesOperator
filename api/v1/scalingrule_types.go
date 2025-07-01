/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScalingRuleSpec defines the desired state of ScalingRule.
type ScalingRuleSpec struct {
	// DeploymentName is the name of the Deployment to scale
	// +kubebuilder:validation:Required
	DeploymentName string `json:"deploymentName"`

	// Namespace is the namespace where the Deployment is located
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// MinReplicas is the minimum number of replicas for the Deployment
	// +kubebuilder:validation:Minimum=1
	MinReplicas int32 `json:"minReplicas"`

	// MaxReplicas is the maximum number of replicas for the Deployment
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas"`

	// NatsMonitoringURL is the URL for NATS monitoring endpoint (e.g., http://nats:8222)
	// +kubebuilder:validation:Required
	NatsMonitoringURL string `json:"natsMonitoringURL"`

	// Subject is the NATS queue subject to monitor
	// +kubebuilder:validation:Required
	Subject string `json:"subject"`

	// ScaleUpThreshold is the number of pending messages that triggers scaling up
	// +kubebuilder:validation:Minimum=1
	ScaleUpThreshold int32 `json:"scaleUpThreshold"`

	// ScaleDownThreshold is the number of pending messages that triggers scaling down
	// +kubebuilder:validation:Minimum=0
	ScaleDownThreshold int32 `json:"scaleDownThreshold"`

	// PollIntervalSeconds is the interval in seconds between NATS monitoring checks
	// +kubebuilder:validation:Minimum=1
	PollIntervalSeconds int32 `json:"pollIntervalSeconds"`
}

// ScalingRuleStatus defines the observed state of ScalingRule.
type ScalingRuleStatus struct {
	// CurrentReplicas is the current number of replicas of the target Deployment
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// PendingMessages is the current number of pending messages on the monitored subject
	PendingMessages int32 `json:"pendingMessages,omitempty"`

	// LastScalingAction is the timestamp of the last scaling action
	LastScalingAction *metav1.Time `json:"lastScalingAction,omitempty"`

	// LastScalingReason is the reason for the last scaling action
	LastScalingReason string `json:"lastScalingReason,omitempty"`

	// Conditions represent the latest available observations of a ScalingRule's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Deployment",type="string",JSONPath=".spec.deploymentName"
// +kubebuilder:printcolumn:name="Namespace",type="string",JSONPath=".spec.namespace"
// +kubebuilder:printcolumn:name="Current Replicas",type="integer",JSONPath=".status.currentReplicas"
// +kubebuilder:printcolumn:name="Pending Messages",type="integer",JSONPath=".status.pendingMessages"
// +kubebuilder:printcolumn:name="Last Action",type="string",JSONPath=".status.lastScalingAction"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ScalingRule is the Schema for the scalingrules API.
type ScalingRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScalingRuleSpec   `json:"spec,omitempty"`
	Status ScalingRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScalingRuleList contains a list of ScalingRule.
type ScalingRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScalingRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScalingRule{}, &ScalingRuleList{})
}
