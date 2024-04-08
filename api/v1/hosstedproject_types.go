/*
Copyright 2024.

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

// HosstedprojectSpec defines the desired state of Hosstedproject
type HosstedprojectSpec struct {
	Stop           bool       `json:"stop,omitempty"`
	Monitoring     Monitoring `json:"monitoring,omitempty"`
	CVE     CVE `json:"cve,omitempty"`
	DenyNamespaces []string   `json:"denyNamespaces,omitempty"`
}

type Monitoring struct {
	// +kubebuilder:default:=false
	Enable bool `json:"enable,omitempty"`
}

type CVE struct {
	// +kubebuilder:default:=false
	Enable bool `json:"enable,omitempty"`
}

// HosstedprojectStatus defines the observed state of Hosstedproject
type HosstedprojectStatus struct {
	ClusterUUID             string            `json:"clusterUUID,omitempty"`
	EmailID                 string            `json:"email_id,omitempty"`
	LastReconciledTimestamp string            `json:"lastReconcileTimestamp,omitempty"`
	ReconciledHelmReleases  map[string]string `json:"reconcileHelmReleases,omitempty"`
	HelmStatus              []HelmInfo        `json:"helmStatus,omitempty"`
	Revision                []int             `json:"revision,omitempty"`
}

// Define HelmInfo struct
type HelmInfo struct {
	Name        string `json:"name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	AppUUID     string `json:"appUUID,omitempty"`
	Revision    int    `json:"revision,omitempty"`
	Updated     string `json:"updated,omitempty"`
	Status      string `json:"status,omitempty"`
	Chart       string `json:"chart,omitempty"`
	AppVersion  string `json:"appVersion,omitempty"`
	HosstedHelm bool   `json:"hossted_helm,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=hp,scope=Cluster
// Hosstedproject is the Schema for the hosstedprojects API
type Hosstedproject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HosstedprojectSpec   `json:"spec,omitempty"`
	Status HosstedprojectStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HosstedprojectList contains a list of Hosstedproject
type HosstedprojectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Hosstedproject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Hosstedproject{}, &HosstedprojectList{})
}
