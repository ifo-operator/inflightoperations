/*
Copyright 2026.

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
	"time"

	libcnd "github.com/mansam/inflightoperations/lib/condition"
	"github.com/mansam/inflightoperations/settings"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var Settings = &settings.Settings

type Subject = unstructured.Unstructured

// OperationPhase represents the lifecycle phase of an operation
type OperationPhase string

const (
	OperationPhaseActive    OperationPhase = "Active"
	OperationPhaseCompleted OperationPhase = "Completed"
)

// SubjectReference identifies a Kubernetes resource
type SubjectReference struct {
	// APIVersion is the API version of the resource (e.g., "v1", "kubevirt.io/v1")
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind is the kind of the resource (e.g., "Pod", "VirtualMachine")
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name is the name of the resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the resource
	// Empty for cluster-scoped resources (which we don't track)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// UID is the UID of the resource for strong reference
	// +optional
	UID string `json:"uid,omitempty"`
}

// InFlightOperationSpec defines the desired state of InFlightOperation
type InFlightOperationSpec struct {
	// Operation is the name of the operation being performed (e.g., "Migrating", "Starting")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Operation string `json:"operation"`

	// Subject references the resource performing the operation.
	// +kubebuilder:validation:Required
	Subject SubjectReference `json:"subject"`
}

// InFlightOperationStatus defines the observed state of InFlightOperation.
type InFlightOperationStatus struct {
	// Phase represents the current lifecycle phase of the operation
	// +kubebuilder:validation:Enum=Active;Completed
	// +kubebuilder:default=Active
	// +optional
	Phase OperationPhase `json:"phase,omitempty"`

	// Completed is when the operation completed (phase transitioned to Completed)
	// +optional
	Completed *metav1.Time `json:"completed,omitempty"`

	// LastDetected is the last time this operation was detected as active
	// This is updated on every evaluation where the operation is still active
	// +optional
	LastDetected *metav1.Time `json:"lastDetected,omitempty"`

	// DetectedBy lists the OperationRuleSet names currently detecting this operation
	// This is updated on each evaluation to show which rules are actively matching
	// +optional
	DetectedBy []string `json:"detectedBy,omitempty"`

	// SubjectGeneration is the metadata.generation of the subject resource
	// Helps track which version of the resource is being evaluated
	// +optional
	SubjectGeneration int64 `json:"subjectGeneration,omitempty"`

	libcnd.Conditions `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ifo;ifos
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.subject.kind`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.subject.namespace`
// +kubebuilder:printcolumn:name="Subject",type=string,JSONPath=`.spec.subject.name`
// +kubebuilder:printcolumn:name="Operation",type=string,JSONPath=`.spec.operation`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Started",type=string,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Completed",type=string,JSONPath=`.status.completed`

// InFlightOperation is the Schema for the operations API
type InFlightOperation struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of InFlightOperation
	// +required
	Spec InFlightOperationSpec `json:"spec"`

	// status defines the observed state of InFlightOperation
	// +optional
	Status InFlightOperationStatus `json:"status,omitzero"`
}

func (r *InFlightOperation) MarkCompleted(subject *Subject) {
	now := metav1.Now()
	r.Status.Phase = OperationPhaseCompleted
	r.Status.Completed = &now
	r.Status.SubjectGeneration = subject.GetGeneration()
}

func (r *InFlightOperation) MarkDetection(subject *Subject, detectedBy []string) {
	now := metav1.Now()
	r.Status.Phase = OperationPhaseActive
	r.Status.LastDetected = &now
	r.Status.SubjectGeneration = subject.GetGeneration()
	r.Status.DetectedBy = detectedBy
	r.Status.Completed = nil
}

func (r *InFlightOperation) Complete() bool {
	return !r.Status.Completed.IsZero()
}

func (r *InFlightOperation) PastDebounceThreshold() bool {
	return r.Complete() && !r.Status.Completed.Add(Settings.DebounceThreshold).After(time.Now())
}

// +kubebuilder:object:root=true

// InFlightOperationList contains a list of InFlightOperation
type InFlightOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []InFlightOperation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InFlightOperation{}, &InFlightOperationList{})
}
