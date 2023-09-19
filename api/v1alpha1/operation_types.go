// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OperationFinalizer = "Operation.mobilityservice.vmware.com"
)

type OperationType string

const (
	Import        OperationType = "Import"
	Export        OperationType = "Export"
	ColdMigration OperationType = "ColdMigration"
	LiveMigration OperationType = "LiveMigration"
)

type OperationReference struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	Kind string `json:"kind,omitempty"`

	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +optional
	Name string `json:"name,omitempty"`
}

type RelocateSpec struct {
	HostIp           string `json:"hostIp"`
	ResourcePoolName string `json:"resourcePoolName"`
	DatastoreName    string `json:"datastoreName"`
	VmNetworkName    string `json:"vmNetworkName"`
	FolderName       string `json:"folderName"`
}

// OperationSpec defines the desired state of Operation
type OperationSpec struct {
	OperationType OperationType               `json:"operationType"`
	EntityName    string                      `json:"entityName,omitempty"`
	Entities      EntitiesReference           `json:"entities,omitempty"`
	VmSpec        VirtualMachineSpec          `json:"vmSpec,omitempty"`
	Source        VsphereLocationReference    `json:"source,omitempty"`
	Destination   SupervisorLocationReference `json:"destination,omitempty"`
	RelocateSpec  RelocateSpec                `json:"relocateSpec,omitempty"`
}

// OperationStatus defines the observed state of Operation
type OperationStatus struct {

	// Phase represents the current phase of operation actuation.
	// E.g. Pending, Running, Terminating, Failed etc.
	// +optional
	Phase string `json:"phase,omitempty"`

	// TaskRef is a managed object reference to a Task related to this operation.
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	// +optional
	TaskRef string `json:"taskRef,omitempty"`

	// Conditions describes the current condition information of the Operation.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Operation is the Schema for the Plans API
type Operation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperationSpec   `json:"spec,omitempty"`
	Status OperationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OperationList contains a list of Operation
type OperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Operation `json:"items"`
}

func (o *Operation) GetConditions() []metav1.Condition {
	return o.Status.Conditions
}

func (o *Operation) SetConditions(conditions []metav1.Condition) {
	o.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Operation{}, &OperationList{})
}
