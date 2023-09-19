// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SupervisorLocationReference struct {
	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +optional
	Name string `json:"name,omitempty"`
}

// SupervisorLocationSpec defines the desired state of SupervisorLocation
type SupervisorLocationSpec struct {
	// The secret containing the authentication info for the API server.
	Identity corev1.SecretReference `json:"identity"`

	// The hostname on which the API server is serving.
	Host string `json:"host"`

	// The port on which the API server is serving.
	Port int32 `json:"port"`

	// The namespace in which the API server is serving.
	Namespace string `json:"namespace"`
}

// SupervisorLocationStatus defines the observed state of SupervisorLocation
type SupervisorLocationStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SupervisorLocation is the Schema for the SupervisorLocations API
type SupervisorLocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SupervisorLocationSpec   `json:"spec,omitempty"`
	Status SupervisorLocationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SupervisorLocationList contains a list of SupervisorLocation
type SupervisorLocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SupervisorLocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SupervisorLocation{}, &SupervisorLocationList{})
}
