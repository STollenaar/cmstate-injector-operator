/*
Copyright 2023.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CMAudience struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Important: Run "make" to regenerate code after modifying this file
// CMStateSpec defines the desired state of CMState
type CMStateSpec struct {
	Audience []CMAudience `json:"audience"`
	Target   string       `json:"target,omitempty"`
}

// CMStateStatus defines the observed state of CMState
type CMStateStatus struct {
	// Represents the observations of a Memcached's current state.
	// condition types may define expected values and meanings for this field, and whether the values
	// are considered a guaranteed API.
	// For further information see: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// Conditions store the status conditions of the Memcached instances
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced

// CMState is the Schema for the cmstates API
type CMState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CMStateSpec   `json:"spec,omitempty"`
	Status CMStateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CMStateList contains a list of CMState
type CMStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CMState `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CMState{}, &CMStateList{})
}
