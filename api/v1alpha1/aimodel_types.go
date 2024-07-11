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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AIModelSpec defines the desired state of AIModel
type AIModelSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Type of the AIModel, could be "local" or "remote"
	Type string `json:"type"`

	// Model is the name of the model
	Model string `json:"model"`

	// Replicas is the number of replicas of the model
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// If the AIModel is of type "Remote", the following fields should be set
	// otherwise, they should be omitted

	// +optional
	BaseURL string `json:"baseURL,omitempty"`
	// +optional
	APIKey string `json:"apiKey,omitempty"`

	// Image is the docker image of the model
	Image string `json:"image"`

	// MaxProcessNum is the maximum number of threads to process the requests
	// +optional
	MaxProcessNum *int32 `json:"maxProcessNum,omitempty"`

	// MsgBacklogThreshold is the threshold of the lag for KEDA
	// +optional
	MsgBacklogThreshold *int32 `json:"msgBacklogThreshold,omitempty"`
}

// AIModelStatus defines the observed state of AIModel
type AIModelStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="Type of the AIModel"
// +kubebuilder:printcolumn:name="Model",type="string",JSONPath=".spec.model",description="Model of the AIModel"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas",description="Replicas of the AIModel"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="State of the AIModel"
// AIModel is the Schema for the aimodels API
type AIModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// +optional
	Spec AIModelSpec `json:"spec,omitempty"`

	// +optional
	Status AIModelStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AIModelList contains a list of AIModel
type AIModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIModel{}, &AIModelList{})
}
