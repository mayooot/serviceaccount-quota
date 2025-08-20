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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceAccountQuotaSpec defines the desired state of ServiceAccountQuota.
type ServiceAccountQuotaSpec struct {
	// ServiceAccountName specifies the ServiceAccount name this quota applies to
	// +kubebuilder:validation:Required
	ServiceAccountName string `json:"serviceAccountName"`

	// Hard is the set of desired hard limits for each named resource
	// +optional
	Hard corev1.ResourceList `json:"hard,omitempty"`
}

// ServiceAccountQuotaStatus defines the observed state of ServiceAccountQuota.
type ServiceAccountQuotaStatus struct {
	// Hard is the set of enforced hard limits for each named resource.
	// +optional
	Hard corev1.ResourceList `json:"hard,omitempty"`

	// Used is the current observed resource usage.
	// +optional
	Used corev1.ResourceList `json:"used,omitempty"`

	// QuotaSummary provides a human-readable summary of used and hard limits in the format "resource: used/hard".
	// +optional
	QuotaSummary string `json:"quotaSummary,omitempty"`

	// LastReconciled indicates the last time the quota was reconciled.
	// +optional
	LastReconciled *metav1.Time `json:"lastReconciled,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=aq
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ServiceAccount",type="string",JSONPath=".spec.serviceAccountName",description="Target ServiceAccount"
// +kubebuilder:printcolumn:name="Quota",type="string",JSONPath=".status.quotaSummary",description="Resource usage and limits"
// +kubebuilder:printcolumn:name="LastReconciled",type="date",JSONPath=".status.lastReconciled",description="Last reconciliation time"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"

// ServiceAccountQuota is the Schema for the serviceaccountquotas API.
type ServiceAccountQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceAccountQuotaSpec   `json:"spec,omitempty"`
	Status ServiceAccountQuotaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceAccountQuotaList contains a list of ServiceAccountQuota.
type ServiceAccountQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceAccountQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceAccountQuota{}, &ServiceAccountQuotaList{})
}
