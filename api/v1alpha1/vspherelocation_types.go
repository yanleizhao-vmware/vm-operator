// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VsphereLocationReference struct {
	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +optional
	Name string `json:"name,omitempty"`
}

// VsphereLocationSpec defines the desired state of vSphere location
type VsphereLocationSpec struct {
	// ResourcePool is the name or inventory path of the resource pool in which
	// the virtual machine is or should be located
	// +optional
	ResourcePool string `json:"resourcePool,omitempty"`
}

// SupervisorLocationStatus defines the observed state of SupervisorLocation
type VsphereLocationStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VsphereLocation is the Schema for the VsphereLocations API
type VsphereLocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VsphereLocationSpec   `json:"spec,omitempty"`
	Status VsphereLocationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VsphereLocationList contains a list of VsphereLocation
type VsphereLocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VsphereLocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VsphereLocation{}, &VsphereLocationList{})
}
