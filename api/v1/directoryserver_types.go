/*
Copyright 2020 Red Hat, Inc.

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

// DirectoryServerSpec defines the desired state of DirectoryServer
type DirectoryServerSpec struct {
	// +kubebuilder:validation:Minimum=0
	// Size is the size of the directoryserver deployment
	Size int32 `json:"size"`
}

// DirectoryServerStatus defines the observed state of DirectoryServer
type DirectoryServerStatus struct {
	// Nodes are the names of the directory server pods
	Nodes []string `json:"nodes"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DirectoryServer is the Schema for the directoryservers API
type DirectoryServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DirectoryServerSpec   `json:"spec,omitempty"`
	Status DirectoryServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DirectoryServerList contains a list of DirectoryServer
type DirectoryServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DirectoryServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DirectoryServer{}, &DirectoryServerList{})
}
