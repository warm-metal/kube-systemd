/*
Copyright 2021.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// UnitSpec defines the desired state of Unit
type UnitSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Specify an existed job which will restart once node boots up.
	// +optional
	Job corev1.ObjectReference `json:"job,omitempty"`

	// Defines a systemd unit.
	// +optional
	HostUnit HostSystemdUnit `json:"unit,omitempty"`
}

type HostSystemdUnit struct {
	// Path defines the absolute path on the host of the unit.
	Path string `json:"path,omitempty"`

	// Definition specifies the unit definition. If set, it is written to the unit configuration which Path defines.
	// Or, the original unit on the host will be used.
	// +optional
	Definition string `json:"definition,omitempty"`

	// Config specifies config files and contents on the host with respect to the systemd unit.
	// The key is the absolute path of the configuration file. And, the value is the file content.
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// UnitStatus defines the observed state of Unit
type UnitStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Timestamp of the last execution
	// +optional
	ExecTimestamp metav1.Time `json:"execTimestamp,omitempty"`

	// Specify Errors on reconcile
	// +optional
	Error string `json:"error,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.spec.job`
//+kubebuilder:printcolumn:name="HostUnit",type=string,JSONPath=`.spec.unit.path`
//+kubebuilder:printcolumn:name="ExecAGE",type=date,JSONPath=`.status.execTimestamp`
//+kubebuilder:printcolumn:name="Error",type=string,JSONPath=`.status.error`

// Unit is the Schema for the units API
type Unit struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UnitSpec   `json:"spec,omitempty"`
	Status UnitStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// UnitList contains a list of Unit
type UnitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Unit `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Unit{}, &UnitList{})
}
