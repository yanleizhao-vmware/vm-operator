// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

var (
	VSphereVMEntityKind = "VsphereVMEntity"
)

type EntityReference struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// Required
	Kind string `json:"kind"`

	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// Required
	Namespace string `json:"namespace"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// Required
	Name string `json:"name"`
}

type EntitySelector struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// Required
	Kind string `json:"kind"`

	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// Required
	Namespace string `json:"namespace"`

	// Selector is the same as the label selector but in the string format to avoid introspection
	// by clients. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	Selector *v1.LabelSelector `json:"selector,omitempty"`

	// NameRegexPattern is a regular expression to match the name of the entities.
	NameRegexPattern string `json:"nameRegexPattern,omitempty"`

	// Parent resource pool of the entities.
	ResourcePool string `json:"resourcePool,omitempty"`
}

type EntitiesReference struct {
	// +optional
	EntityRefs []EntityReference `json:"entityRefs,omitempty"`

	// +optional
	EntitySelector *EntitySelector `json:"entitySelector,omitempty"`

	// vSphere Tag?
}

/*
// EntitySpec defines the desired state of Entity
type EntitySpec struct {
	EntityRef EntityReference `json:"entityRef"`
}

// EntityStatus defines the observed state of Entity
type EntityStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Entity is the Schema for the entities API
type Entity struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EntitySpec   `json:"spec,omitempty"`
	Status EntityStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EntityList contains a list of Entity
type EntityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Entity `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Entity{}, &EntityList{})
}

*/
