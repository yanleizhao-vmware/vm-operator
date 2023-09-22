// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VsphereVMSpec defines the desired state of VsphereVM
type VsphereVMSpec struct {
	// Name of the vSphere VM
	// +optional
	Name string `json:"name,omitempty"`

	// MoID of the vSphere VM
	// +optional
	ManagedObjectID string `json:"moid,omitempty"`
}

// SupervisorLocationStatus defines the observed state of SupervisorLocation
type VsphereVMStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VsphereVM is the Schema for the VsphereVMs API
type VsphereVM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VsphereVMSpec   `json:"spec,omitempty"`
	Status VsphereVMStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VsphereVMList contains a list of VsphereVM
type VsphereVMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VsphereVM `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VsphereVM{}, &VsphereVMList{})
}
