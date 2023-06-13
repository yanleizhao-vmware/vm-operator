// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PlanFinalizer = "Plan.mobilityservice.vmware.com"
)

// PlanSpec defines the desired state of Plan
type PlanSpec struct {
	Operations []OperationReference `json:"operations"`
}

// PlanStatus defines the observed state of Plan
type PlanStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Plan is the Schema for the Plans API
type Plan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlanSpec   `json:"spec,omitempty"`
	Status PlanStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PlanList contains a list of Plan
type PlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Plan `json:"items"`
}

func (p *Plan) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *Plan) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Plan{}, &PlanList{})
}
