/*
Copyright 2026 Red Hat, Inc.

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DirectoryServiceSpec defines the desired state of a 389 Directory Server instance.
type DirectoryServiceSpec struct {
	// Image is the 389DS container image to deploy.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Version is the target 389DS semantic version. Existing resources may omit
	// this field; the operator adopts their image version on first observation.
	// +kubebuilder:validation:Pattern=`^v?[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$`
	// +optional
	Version string `json:"version,omitempty"`

	// Replicas is the number of DS pods in the StatefulSet.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Suffixes defines the database backends to create on first boot.
	// +optional
	Suffixes []SuffixSpec `json:"suffixes,omitempty"`

	// Storage configures persistent volume claims for the /data volume.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// DMPasswordSecretRef references a Secret containing the Directory Manager password.
	// The Secret must have a key named "dm-password".
	// If omitted, the operator generates a random password and creates a Secret.
	// +optional
	DMPasswordSecretRef *corev1.LocalObjectReference `json:"dmPasswordSecretRef,omitempty"`

	// DMPasswordMode controls how the Directory Manager password is injected into the container.
	//   - "env": injects via DS_DM_PASSWORD environment variable (supported by all 389DS container versions).
	//   - "file": mounts the Secret as a file and sets DS_DM_PASSWORD_FILE.
	//     Requires 389DS container image with DS_DM_PASSWORD_FILE support.
	// +kubebuilder:default=env
	// +kubebuilder:validation:Enum=env;file
	// +optional
	DMPasswordMode string `json:"dmPasswordMode,omitempty"`

	// Ports configures LDAP and LDAPS listener ports.
	// +optional
	Ports *PortSpec `json:"ports,omitempty"`

	// Resources defines CPU/memory requests and limits for the DS container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SuffixSpec defines a database backend (suffix) to create.
type SuffixSpec struct {
	// Name is the backend database name (e.g. "userroot").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z][a-zA-Z0-9_-]*$`
	Name string `json:"name"`

	// DN is the suffix distinguished name (e.g. "dc=example,dc=com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DN string `json:"dn"`

	// CreateEntries populates the suffix with sample organizational entries on creation.
	// +kubebuilder:default=false
	// +optional
	CreateEntries bool `json:"createEntries,omitempty"`
}

// StorageSpec defines persistent storage configuration.
type StorageSpec struct {
	// Size is the PVC storage request (e.g. "10Gi").
	// +kubebuilder:default="1Gi"
	// +optional
	Size resource.Quantity `json:"size,omitempty"`

	// StorageClassName is the name of the StorageClass to use.
	// If omitted, the cluster default StorageClass is used.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// PortSpec defines LDAP listener ports.
type PortSpec struct {
	// LDAP is the non-TLS listener port.
	// +kubebuilder:default=3389
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	LDAP int32 `json:"ldap,omitempty"`

	// LDAPS is the TLS listener port.
	// +kubebuilder:default=3636
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	LDAPS int32 `json:"ldaps,omitempty"`
}

// DirectoryServiceStatus defines the observed state of DirectoryService.
type DirectoryServiceStatus struct {
	// Phase represents the current lifecycle phase.
	// +kubebuilder:validation:Enum=Initializing;Running;Degraded;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// Replicas is the total number of pods targeted by the StatefulSet.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of pods that are ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// SuffixesReady indicates whether all configured suffixes have been created.
	// +optional
	SuffixesReady bool `json:"suffixesReady,omitempty"`

	// Conditions represent the latest available observations of the instance's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CurrentVersion is the last version confirmed healthy by the operator.
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// TargetVersion is the version requested by the latest upgrade.
	// +optional
	TargetVersion string `json:"targetVersion,omitempty"`

	// Upgrade contains the active upgrade progress.
	// +optional
	Upgrade *UpgradeStatus `json:"upgrade,omitempty"`

	// History contains the latest completed upgrade attempts.
	// +optional
	History []UpgradeRecord `json:"history,omitempty"`
}

// UpgradeStatus records the state of a version transition.
type UpgradeStatus struct {
	// FromVersion is the version before the upgrade.
	// +optional
	FromVersion string `json:"fromVersion,omitempty"`
	// Phase is the current upgrade phase.
	// +kubebuilder:validation:Enum=Validating;Upgrading;Succeeded;Failed;RollingBack;RolledBack
	Phase string `json:"phase,omitempty"`
	// UpdatedReplicas is the number of replicas using the target template.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`
	// ReadyReplicas is the number of ready replicas using the target template.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// Message describes the current upgrade step.
	// +optional
	Message string `json:"message,omitempty"`
	// FailureReason describes why the upgrade stopped.
	// +optional
	FailureReason string `json:"failureReason,omitempty"`
	// StartedAt records when the upgrade started.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// CompletedAt records when the upgrade ended.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// UpgradeRecord is a compact record of one completed upgrade attempt.
type UpgradeRecord struct {
	FromVersion   string      `json:"fromVersion,omitempty"`
	ToVersion     string      `json:"toVersion,omitempty"`
	Result        string      `json:"result,omitempty"`
	FailureReason string      `json:"failureReason,omitempty"`
	StartedAt     metav1.Time `json:"startedAt,omitempty"`
	CompletedAt   metav1.Time `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dirsrv
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DirectoryService is the Schema for the directoryservices API.
// It represents a 389 Directory Server deployment managed by the operator.
type DirectoryService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DirectoryServiceSpec   `json:"spec,omitempty"`
	Status DirectoryServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DirectoryServiceList contains a list of DirectoryService.
type DirectoryServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DirectoryService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DirectoryService{}, &DirectoryServiceList{})
}
